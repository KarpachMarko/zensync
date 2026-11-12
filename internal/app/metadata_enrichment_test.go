package app

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrichmentMatching(t *testing.T) {
	s := Song{Title: "Song", Artist: "Band", Album: "Album", Duration: 120}
	if !enrichmentTrackMatch(s, "Song", "Band", "Album", 121) {
		t.Fatal("exact match rejected")
	}
	for _, other := range []Song{{Title: "Song (Live)", Artist: "Band", Album: "Album", Duration: 120}, {Title: "Song", Artist: "Other", Album: "Album", Duration: 120}, {Title: "Song", Artist: "Band", Album: "Deluxe", Duration: 120}, {Title: "Song", Artist: "Band", Album: "Album", Duration: 150}, {Title: "Song", Artist: "Band", Album: "Album"}} {
		if enrichmentTrackMatch(s, other.Title, other.Artist, other.Album, other.Duration) {
			t.Fatalf("unsafe match: %+v", other)
		}
	}
}

func TestEnrichmentQueuesSelectionAndPermissions(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	expect(t, member.call("POST", "/api/v2/metadata/enrichment", `{"mode":"missing-covers"}`, ""), 403)
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"unknown"}`, ""), 400)
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"force-covers"}`, ""), 400)
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"force-covers","albumIds":["album-one"]}`, ""), 202)
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"force-covers","songIds":["two"]}`, ""), 202)
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"missing-lyrics"}`, ""), 409)
	var count int
	if err := a.DB.QueryRow("SELECT count(*) FROM metadata_jobs WHERE json_array_length(changes)=1").Scan(&count); err != nil || count != 2 {
		t.Fatalf("selection: %d %v", count, err)
	}
}

func TestEnrichmentLyricsJobPreservesAndSkipsExisting(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	failedOnce := false
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "lrclib.net" {
			if !failedOnce {
				failedOnce = true
				return &http.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"id":7,"trackName":"Song","artistName":"Band","albumName":"Album","duration":1,"plainLyrics":"Fetched words","syncedLyrics":"[00:01]Fetched words"}]`)), Header: make(http.Header)}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"missing-lyrics"}`, ""), 202)
	a.runMetadata(context.Background())
	var jid, failedState string
	if err := a.DB.QueryRow("SELECT id,state FROM metadata_jobs").Scan(&jid, &failedState); err != nil || failedState != "failed" {
		t.Fatal("provider error did not fail job", failedState, err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": jid}), ""), 200)
	a.runMetadata(context.Background())
	var state, message string
	if err := a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message); err != nil || state != "completed" {
		t.Fatalf("%s %s %v", state, message, err)
	}
	for _, s := range songs {
		p, err := a.probeUpload(filepath.Join(a.musicDir, s.Path), "flac")
		if err != nil {
			t.Fatal(err)
		}
		tags := uploadTags(p)
		if tags["lyrics"] != "[00:01]Fetched words" || tags["artist"] != s.Artist || tags["album"] != s.Album {
			t.Fatalf("tags lost: %+v", tags)
		}
	}
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "lrclib.net" {
			t.Error("existing lyrics triggered provider request")
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"missing-lyrics"}`, ""), 202)
	a.runMetadata(context.Background())
	if err := a.DB.QueryRow("SELECT message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&message); err != nil || !strings.Contains(message, "2 skipped") {
		t.Fatal(message, err)
	}
}

func TestEnrichmentDeezerTrackOrderAndCover(t *testing.T) {
	for _, ext := range []string{".flac", ".mp3", ".m4a"} {
		t.Run(ext, func(t *testing.T) {
			codec := "flac"
			if ext == ".mp3" {
				codec = "libmp3lame"
			}
			if ext == ".m4a" {
				codec = "aac"
			}
			a, _, songs := metadataLibraryFixtureFormat(t, ext, codec, "")
			var artwork bytes.Buffer
			if err := png.Encode(&artwork, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
				t.Fatal(err)
			}
			a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "cdn-images.dzcdn.net" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(artwork.Bytes())), Header: make(http.Header)}, nil
				}
				if strings.HasPrefix(r.URL.Path, "/search") {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":42,"title":"Song","artist":{"name":"Band"},"album":{"title":"Album"},"duration":1}]}`)), Header: make(http.Header)}, nil
				}
				if r.URL.Path == "/track/42" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":42,"title":"Song","track_position":7,"disk_number":2,"album":{"cover_xl":"https://cdn-images.dzcdn.net/image/cover/test.jpg"}}`)), Header: make(http.Header)}, nil
				}
				return http.DefaultTransport.RoundTrip(r)
			})
			c, err := a.shared()
			if err != nil {
				t.Fatal(err)
			}
			s := songs["one"]
			change := metadataChange{Song: s, Edit: MetadataEdit{Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumArtist: s.AlbumArtist}, Enrichment: "force-covers"}
			cover, err := a.resolveEnrichment(context.Background(), c, change)
			if err != nil || cover.Skip || !strings.Contains(cover.Edit.CoverURL, "dzcdn.net") {
				t.Fatalf("cover: %+v %v", cover, err)
			}
			change.Enrichment = "album-order"
			change, err = a.resolveEnrichment(context.Background(), c, change)
			if err != nil {
				t.Fatal(err)
			}
			var uid string
			a.DB.QueryRow("SELECT id FROM users WHERE role='admin'").Scan(&uid)
			change.Resolved = true
			jid, err := a.queueMetadata([]metadataChange{change}, User{ID: uid})
			if err != nil {
				t.Fatal(err)
			}
			before, err := metadataAudioHash(context.Background(), filepath.Join(a.musicDir, s.Path))
			if err != nil {
				t.Fatal(err)
			}
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", jid).Scan(&state, &message)
			if state != "completed" {
				t.Fatal(state, message)
			}
			p, err := a.probeUpload(filepath.Join(a.musicDir, s.Path), uploadFormat(ext))
			if err != nil {
				t.Fatal(err)
			}
			tags := uploadTags(p)
			if strings.Split(tags["track"], "/")[0] != "7" || strings.Split(tags["disc"], "/")[0] != "2" {
				t.Fatalf("order not updated: %+v", tags)
			}
			after, _ := metadataAudioHash(context.Background(), filepath.Join(a.musicDir, s.Path))
			if before != after {
				t.Fatal("audio changed")
			}
			// Publish a fetched cover, then prove missing-only mode leaves it untouched.
			cover.Resolved = true
			jid, err = a.queueMetadata([]metadataChange{cover}, User{ID: uid})
			if err != nil {
				t.Fatal(err)
			}
			a.runMetadata(context.Background())
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", jid).Scan(&state, &message)
			if state != "completed" {
				t.Fatal(state, message)
			}
			p, err = a.probeUpload(filepath.Join(a.musicDir, s.Path), uploadFormat(ext))
			if err != nil {
				t.Fatal(err)
			}
			hasCover := false
			for _, stream := range p.Streams {
				if stream.Disposition.AttachedPic == 1 {
					hasCover = true
				}
			}
			if !hasCover {
				t.Fatal("cover was not embedded")
			}
			cover.Enrichment = "missing-covers"
			cover.Resolved = false
			skipped, err := a.resolveEnrichment(context.Background(), c, cover)
			if err != nil || !skipped.Skip {
				t.Fatalf("existing artwork replaced: %+v %v", skipped, err)
			}
			after, _ = metadataAudioHash(context.Background(), filepath.Join(a.musicDir, s.Path))
			if before != after {
				t.Fatal("cover changed audio")
			}

		})
	}
}
