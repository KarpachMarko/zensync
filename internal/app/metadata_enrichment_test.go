package app

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
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
			// Existing numbers and aliases must actually be replaced, not appended.
			source := filepath.Join(a.musicDir, songs["one"].Path)
			tmp := source + ext
			if out, err := exec.Command("ffmpeg", "-v", "error", "-i", source, "-c", "copy", "-metadata", "track=20", "-metadata", "disc=3", tmp).CombinedOutput(); err != nil {
				t.Fatalf("seed order: %v %s", err, out)
			}
			if err := os.Rename(tmp, source); err != nil {
				t.Fatal(err)
			}
			var artwork bytes.Buffer
			if err := png.Encode(&artwork, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
				t.Fatal(err)
			}
			a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "cdn-images.dzcdn.net" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(artwork.Bytes())), Header: make(http.Header)}, nil
				}
				if body := albumReferenceFixture(r); body != "" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
				}
				if strings.HasPrefix(r.URL.Path, "/search") {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":42,"title":"Song","artist":{"name":"Band"},"album":{"id":88,"title":"Album"},"duration":1}]}`)), Header: make(http.Header)}, nil
				}
				if r.URL.Path == "/track/42" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":42,"title":"Song","track_position":7,"disk_number":2,"album":{"id":88,"cover_xl":"https://cdn-images.dzcdn.net/image/cover/test.jpg"}}`)), Header: make(http.Header)}, nil
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

func TestAlbumOrderResumePreservesProcessedPositionsAndSkips(t *testing.T) {
	a, _, _ := metadataLibraryFixture(t)
	c, err := a.shared()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"mixed-release", "duplicate-position", "partial"} {
		t.Run(kind, func(t *testing.T) {
			changes := []metadataChange{
				{Song: Song{ID: "a", AlbumID: "album"}, Enrichment: "album-order", Resolved: true, OrderVersion: 2, OrderAlbumID: 1, Edit: MetadataEdit{TrackNumber: 1, DiscNumber: 1}},
				{Song: Song{ID: "b", AlbumID: "album"}, Enrichment: "album-order", Resolved: true, OrderVersion: 2, OrderAlbumID: 1, Edit: MetadataEdit{TrackNumber: 2, DiscNumber: 1}},
			}
			switch kind {
			case "mixed-release":
				changes[1].OrderAlbumID = 2
			case "duplicate-position":
				changes[1].Edit.TrackNumber = 1
			case "partial":
				changes[1].Skip = true
			}
			if err := a.prepareAlbumOrder(context.Background(), c, "unused", changes); err != nil {
				t.Fatal(err)
			}
			if changes[0].Skip {
				t.Fatal("matched song incorrectly skipped")
			}
			if changes[1].Skip != (kind == "partial") {
				t.Fatalf("unexpected per-song skipping: %+v", changes)
			}
		})
	}
}

func TestAlbumOrderOnlyAcceptsAlbumSelections(t *testing.T) {
	_, admin, _ := metadataLibraryFixture(t)
	for _, body := range []string{`{"mode":"album-order","songIds":["one"]}`, `{"mode":"album-order","albumIds":[]}`, `{"mode":"album-order","albumIds":[""]}`} {
		expect(t, admin.call("POST", "/api/v2/metadata/enrichment", body, ""), 400)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/enrichment", `{"mode":"album-order","albumIds":["album-one"]}`, ""), 202)
}

func TestAlbumOrderLookupFailureDoesNotSkipOtherSongs(t *testing.T) {
	a, _, songs := metadataLibraryFixture(t)
	c, err := a.shared()
	if err != nil {
		t.Fatal(err)
	}
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/search") {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("down")), Header: make(http.Header)}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	changes := []metadataChange{
		{Song: songs["one"], Enrichment: "album-order", Resolved: true, OrderVersion: 2, OrderAlbumID: 1, Edit: MetadataEdit{TrackNumber: 7, DiscNumber: 1}},
		{Song: songs["two"], Enrichment: "album-order"},
	}
	if err := a.prepareAlbumOrder(context.Background(), c, "unused", changes); err != nil {
		t.Fatal(err)
	}
	if changes[0].Skip || !changes[1].Skip || !changes[1].Resolved {
		t.Fatalf("lookup error blocked sibling: %+v", changes)
	}
}

func TestAlbumOrderWritesDuplicatePositionsAndSkipsMissingSong(t *testing.T) {
	a, _, songs := metadataLibraryFixture(t)
	duplicate := songs["two"]
	duplicate.AlbumID = songs["one"].AlbumID
	songs["two"] = duplicate
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		if body := albumReferenceFixture(r); body != "" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}
		if strings.HasPrefix(r.URL.Path, "/search") {
			body = `{"data":[{"id":42,"title":"Song","artist":{"name":"Band"},"album":{"id":88,"title":"Album"},"duration":1}]}`
		}
		if r.URL.Path == "/track/42" {
			body = `{"id":42,"title":"Song","track_position":7,"disk_number":1,"album":{"id":88}}`
		}
		if body != "" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	changes := []metadataChange{}
	for _, id := range []string{"one", "two"} {
		s := songs[id]
		changes = append(changes, metadataChange{Song: s, Enrichment: "album-order", Edit: MetadataEdit{Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumArtist: s.AlbumArtist}})
	}
	changes = append(changes, metadataChange{Song: Song{ID: "missing", Title: "Removed since queueing"}, Enrichment: "album-order"})
	var uid string
	if err := a.DB.QueryRow("SELECT id FROM users WHERE role='admin'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	jid, err := a.queueMetadata(changes, User{ID: uid})
	if err != nil {
		t.Fatal(err)
	}
	a.runMetadata(context.Background())
	var state, message string
	if err = a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", jid).Scan(&state, &message); err != nil || state != "completed" || !strings.Contains(message, "2 updated, 1 skipped") {
		t.Fatal(state, message, err)
	}
	for _, id := range []string{"one", "two"} {
		p, err := a.probeUpload(filepath.Join(a.musicDir, songs[id].Path), "flac")
		if err != nil {
			t.Fatal(err)
		}
		if uploadTags(p)["track"] != "7" {
			t.Fatal("duplicate did not receive the matched position", uploadTags(p))
		}
	}
}

func TestAlbumOrderMatchesStructuredPrimaryArtist(t *testing.T) {
	a, _, songs := metadataLibraryFixture(t)
	s := songs["one"]
	s.Artist = "Band / Guest"
	s.Artists = []ArtistCredit{{Name: "Band"}, {Name: "Guest"}}
	songs["one"] = s
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		if body := albumReferenceFixture(r); body != "" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}
		if strings.HasPrefix(r.URL.Path, "/search") {
			body = `{"data":[{"id":42,"title":"Song","artist":{"name":"Band"},"album":{"id":88,"title":"Album"},"duration":1}]}`
		}
		if r.URL.Path == "/track/42" {
			body = `{"id":42,"track_position":7,"disk_number":1,"album":{"id":88}}`
		}
		if body != "" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	c, err := a.shared()
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.resolveEnrichment(context.Background(), c, metadataChange{Song: s, Enrichment: "album-order"})
	if err != nil || result.Skip || result.Edit.TrackNumber != 7 {
		t.Fatalf("additional credit hid primary recording: %+v %v", result, err)
	}
}

func albumReferenceFixture(r *http.Request) string {
	if r.URL.Path == "/search/album" {
		return `{"data":[{"id":88,"title":"Album","artist":{"name":"Band"}}]}`
	}
	if r.URL.Path == "/album/88/tracks" {
		return `{"data":[{"id":42,"title":"Song","artist":{"name":"Band"},"duration":1,"track_position":7,"disk_number":2}]}`
	}
	return ""
}
