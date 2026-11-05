package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetadataDuplicateCandidates(t *testing.T) {
	songs := []Song{
		{ID: "1", ArtistID: "a", AlbumID: "x", Artist: "The Band", Album: "The Album", Title: "Song!", Duration: 120},
		{ID: "2", ArtistID: "b", AlbumID: "y", Artist: "the band", Album: "The Album!", Title: "song", Duration: 121.5},
		{ID: "3", ArtistID: "a", AlbumID: "x", Artist: "The Band", Album: "The Album", Title: "Song (Live)", Duration: 121},
		{ID: "4", ArtistID: "a", AlbumID: "x", Artist: "The Band", Album: "The Album", Title: "Song", Duration: 180},
	}
	groups := metadataDuplicates(songs)
	if len(groups) != 3 {
		t.Fatalf("groups: %+v", groups)
	}
	for _, g := range groups {
		if len(g.Members) != 2 {
			t.Fatalf("members: %+v", g)
		}
	}
	if got := metadataDuplicates(append(songs[:1], songs[0])); len(got) != 0 {
		t.Fatalf("Repeated identity: %+v", got)
	}
	if got := metadataDuplicates([]Song{{ID: "a", Title: "Song", Artist: "Band"}, {ID: "b", Title: "Song", Artist: "Band"}}); len(got) != 0 {
		t.Fatal("Unknown durations must not match")
	}
}
func TestMetadataReviewPersistenceAndPermissions(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	admin := login(t, a, "admin")
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	for _, path := range []string{"/metadata/duplicates", "/metadata/lyrics?q=test", "/metadata/covers?q=test", "/metadata/songs/one"} {
		expect(t, member.call("GET", "/api/v2"+path, "", ""), 403)
	}
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "song": Song{ID: "one", Title: "Original", Artist: "Band", Album: "Album"}}})
	}))
	defer nav.Close()
	if e = a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "password"}); e != nil {
		t.Fatal(e)
	}
	path := "/api/v2/metadata/songs/one"
	expect(t, admin.call("PUT", path, `{"title":"","artist":"Band"}`, ""), 400)
	expect(t, admin.call("PUT", path, `{"title":"Edited","artist":"Band","coverUrl":"javascript:alert(1)"}`, ""), 400)
	body := `{"title":"Edited","artist":"Band","album":"Album","lyrics":"Words","lyricsSource":"LRCLIB:123"}`
	expect(t, member.call("PUT", path, body, ""), 403)
	longEdit := MetadataEdit{Title: "Edited", Artist: "Band", Album: "Album", Lyrics: strings.Repeat("words ", 4000)}
	expect(t, admin.call("PUT", path, mustJSON(longEdit), ""), 202)
	expect(t, admin.call("DELETE", path, "", ""), 409)
	expect(t, admin.call("PUT", path, body, ""), 409)
	if _, e = a.DB.Exec("UPDATE metadata_jobs SET state='failed'"); e != nil {
		t.Fatal(e)
	}
	expect(t, admin.call("PUT", path, body, ""), 202)
	if _, e = a.DB.Exec("UPDATE metadata_jobs SET state='failed'"); e != nil {
		t.Fatal(e)
	}
	got := admin.call("GET", path, "", "")
	expect(t, got, 200)
	if !strings.Contains(got.Body.String(), `"title":"Edited"`) || !strings.Contains(got.Body.String(), `"saved":true`) {
		t.Fatal(got.Body)
	}
	expect(t, admin.call("DELETE", path, "", ""), 200)
	got = admin.call("GET", path, "", "")
	if !strings.Contains(got.Body.String(), `"title":"Original"`) {
		t.Fatal(got.Body)
	}
	noCSRF := admin
	noCSRF.csrf = "wrong"
	expect(t, noCSRF.call("PUT", path, body, ""), 403)
}

type metadataTransport func(*http.Request) (*http.Response, error)

func (f metadataTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMetadataProviders(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	admin := login(t, a, "admin")
	id := "12345678-1234-1234-1234-123456789012"
	a.HTTP = &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.Header.Get("User-Agent"), "ZenSync/") {
			t.Error("Missing provider identification")
		}
		body := ""
		status := 200
		switch r.URL.Host {
		case "lrclib.net":
			if r.URL.Query().Get("q") != "Band & Song" {
				t.Error("Query encoding")
			}
			body = `[{"id":1,"trackName":"Song","artistName":"Band","plainLyrics":"Words","syncedLyrics":"[00:01]Words"}]`
		case "musicbrainz.org":
			body = `{"releases":[{"id":"` + id + `","title":"Album","artist-credit":[{"name":"Band"}]}]}`
		case "coverartarchive.org":
			body = `{"images":[{"front":true,"thumbnails":{"500":"https://archive.org/cover.jpg"}}]}`
		default:
			status = 500
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	lyrics := admin.call("GET", "/api/v2/metadata/lyrics?q=Band%20%26%20Song", "", "")
	expect(t, lyrics, 200)
	var rows []lyricCandidate
	if e = json.Unmarshal(lyrics.Body.Bytes(), &rows); e != nil || len(rows) != 1 || rows[0].SyncedLyrics == "" {
		t.Fatal(lyrics.Body)
	}
	covers := admin.call("GET", "/api/v2/metadata/covers?q=Band", "", "")
	expect(t, covers, 200)
	if !strings.Contains(covers.Body.String(), "https://archive.org/cover.jpg") {
		t.Fatal(covers.Body)
	}
}
func TestMetadataProviderCooldownAndMissingArt(t *testing.T) {
	a := &App{HTTP: &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader("busy"))}, nil
	})}}
	if e := a.metadataJSON(context.Background(), "https://lrclib.net/api/search?q=x", &struct{}{}); e == nil {
		t.Fatal("Expected rate limit error")
	}
	if time.Until(a.metadataNext) < 119*time.Second {
		t.Fatal("Retry-After was ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := a.metadataJSON(ctx, "https://lrclib.net", &struct{}{}); e != context.Canceled {
		t.Fatalf("Cancellation: %v", e)
	}
	a.metadataNext = time.Time{}
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("missing"))}, nil
	})
	if e := a.metadataJSON(context.Background(), "https://coverartarchive.org/release/missing", &struct{}{}); e != sql.ErrNoRows {
		t.Fatalf("404: %v", e)
	}
}
