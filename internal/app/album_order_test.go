package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestAlbumOrderUsesCurrentDeluxeAlbumForFilesWithoutProviderIDs(t *testing.T) {
	a, _, songs := metadataLibraryFixture(t)
	duplicate := songs["two"]
	duplicate.AlbumID = songs["one"].AlbumID
	songs["two"] = duplicate
	searches, tracklists := 0, 0
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/rest/getAlbum.view":
			_ = r.ParseForm()
			body = mustJSON(map[string]any{"subsonic-response": map[string]any{"status": "ok", "album": navAlbum{ID: r.Form.Get("id"), Name: "Album (Deluxe Edition)", Artist: "Band"}}})
		case "/search/album":
			searches++
			if !strings.Contains(r.URL.Query().Get("q"), "Deluxe Edition") {
				t.Error("lookup used original file album instead of current library album")
			}
			body = `{"data":[{"id":111,"title":"Album","artist":{"name":"Band"}},{"id":222,"title":"Album (Deluxe Edition)","artist":{"name":"Band"}}]}`
		case "/album/222/tracks":
			tracklists++
			tracks := []map[string]any{}
			for i := 0; i < 28; i++ {
				title := fmt.Sprintf("Intro %d", i)
				duration := 0.5
				if i == 19 {
					title = "Song"
					duration = 1
				}
				// No positional fields: numbering must count the complete reference list,
				// including intros missing locally. Even old nested provenance is irrelevant.
				tracks = append(tracks, map[string]any{"id": i + 1, "title": title, "artist": map[string]string{"name": "Band"}, "duration": duration, "album": map[string]any{"id": 111, "title": "Album"}})
			}
			body = mustJSON(map[string]any{"data": tracks})
		default:
			if !strings.HasPrefix(r.URL.Path, "/rest/") {
				t.Errorf("unexpected per-track/provider-origin request: %s", r.URL.Path)
			}
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
	changes = append(changes, metadataChange{Song: Song{ID: "missing", AlbumID: songs["one"].AlbumID, Title: "Missing"}, Enrichment: "album-order"})
	var uid string
	if err := a.DB.QueryRow("SELECT id FROM users WHERE role='admin'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	jid, err := a.queueMetadata(changes, User{ID: uid})
	if err != nil {
		t.Fatal(err)
	}
	a.runMetadata(context.Background())
	var state, message, raw string
	if err = a.DB.QueryRow("SELECT state,message,changes FROM metadata_jobs WHERE id=?", jid).Scan(&state, &message, &raw); err != nil || state != "completed" || !strings.Contains(message, "2 updated, 1 skipped") {
		t.Fatal(state, message, err)
	}
	if err = json.Unmarshal([]byte(raw), &changes); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		p, err := a.probeUpload(filepath.Join(a.musicDir, songs[id].Path), "flac")
		if err != nil {
			t.Fatal(err)
		}
		if uploadTags(p)["track"] != "20" {
			t.Fatalf("%s received original-release number instead of Deluxe: %+v", id, uploadTags(p))
		}
	}
	if searches != 1 || tracklists != 1 {
		t.Fatal("reference album was not shared across local files", searches, tracklists)
	}
	for _, change := range changes[:2] {
		if change.OrderAlbumID != 222 {
			t.Fatal("used original provider album identity", change.OrderAlbumID)
		}
	}
}

func TestAlbumOrderTrackMatchingIgnoresFileOriginAndAllowsDuplicatePositions(t *testing.T) {
	reference := albumOrderReference{ID: 222, Tracks: []albumOrderTrack{{Track: Track{Title: "Song", Duration: 120, Artist: struct {
		Name string `json:"name"`
	}{Name: "Band"}}, Disc: 2, Position: 5}}}
	for _, artist := range []string{"Band", "Band / Guest"} {
		change := metadataChange{Song: Song{ID: "local-no-deezer-id", Title: "Song", Artist: artist, Album: "Old release", Duration: 120, Artists: []ArtistCredit{{Name: "Band"}}}, OrderAlbumID: 111}
		result := matchAlbumOrder(change, reference)
		if result.Skip || result.Edit.TrackNumber != 5 || result.Edit.DiscNumber != 2 || result.OrderAlbumID != 222 {
			t.Fatalf("local file origin affected album order: %+v", result)
		}
	}
}
