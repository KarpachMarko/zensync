package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type soulTestClient struct {
	a       *App
	token   string
	profile string
}

func (c soulTestClient) call(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	if c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	r.Header.Set("X-Profile-Id", c.profile)
	w := httptest.NewRecorder()
	c.a.Handler().ServeHTTP(w, r)
	return w
}
func soulKey(t *testing.T, c client, scopes string) soulTestClient {
	t.Helper()
	w := c.call("POST", "/api/v2/api-keys", `{"label":"Wavio","scopes":`+scopes+`}`, "")
	expect(t, w, 201)
	var key APIKey
	if e := json.Unmarshal(w.Body.Bytes(), &key); e != nil {
		t.Fatal(e)
	}
	return soulTestClient{c.a, key.Token, "1"}
}
func soulData[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	expect(t, w, 200)
	var envelope struct {
		Success    bool            `json:"success"`
		Data       T               `json:"data"`
		Error      json.RawMessage `json:"error"`
		Pagination json.RawMessage `json:"pagination"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &envelope); e != nil {
		t.Fatal(e)
	}
	if !envelope.Success || string(envelope.Error) != "null" || string(envelope.Pagination) != "null" {
		t.Fatal(w.Body.String())
	}
	return envelope.Data
}
func TestSoulSyncWavioConnectionAndErrors(t *testing.T) {
	f := newDeletionFixture(t)
	c := soulKey(t, f.member, `["search:read"]`)
	status := soulData[struct {
		Services map[string]bool `json:"services"`
		Uptime   int64           `json:"uptime_seconds"`
	}](t, c.call("GET", "/api/v1/system/status", ""))
	if status.Uptime < 0 || status.Services["soulseek"] {
		t.Fatal(status)
	}
	// Readiness reflects the configured acquisition backend rather than a fabricated Soulseek service.
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	f.a.dfiPath = binary
	if e = f.a.seal("deezer", "test-arl"); e != nil {
		t.Fatal(e)
	}
	ready := soulData[struct {
		Services map[string]bool `json:"services"`
	}](t, c.call("GET", "/api/v1/system/status", ""))
	if !ready.Services["soulseek"] || ready.Services["spotify"] {
		t.Fatal(ready)
	}
	// Profile 1 aliases only this key owner; it cannot select an administrator.
	c.profile = f.userID(t, f.admin)
	expect(t, c.call("GET", "/api/v1/system/status", ""), 403)
	c.profile = "1"
	expect(t, c.call("GET", "/api/v2/search", ""), 403)
	for _, token := range []string{"", "invalid"} {
		bad := soulTestClient{f.a, token, "1"}
		w := bad.call("GET", "/api/v1/system/status", "")
		expect(t, w, 401)
		if !strings.Contains(w.Body.String(), `"success":false`) || !strings.Contains(w.Body.String(), `"code":`) || w.Header().Get("X-Request-Id") == "" {
			t.Fatal(w.Body.String())
		}
	}
	expect(t, c.call("GET", "/api/v1/wishlist", ""), 403)
	expect(t, c.call("POST", "/api/v1/watchlist/scan", "{}"), 501)
	expect(t, c.call("GET", "/api/v1/unknown", ""), 404)
	if _, e := f.a.DB.Exec("UPDATE api_keys SET expires=? WHERE token_hash=?", time.Now().Unix()-1, digest(c.token)); e != nil {
		t.Fatal(e)
	}
	expect(t, c.call("GET", "/api/v1/system/status", ""), 401)
	if _, e := f.a.DB.Exec("DELETE FROM api_keys WHERE token_hash=?", digest(c.token)); e != nil {
		t.Fatal(e)
	}
	expect(t, c.call("GET", "/api/v1/system/status", ""), 401)
}
func TestSoulSyncWavioSearchAndAcquisitionLifecycle(t *testing.T) {
	f := newDeletionFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/track":
			send(w, 200, map[string]any{"data": []Track{makeTrack(123, "Song")}})
		case "/search/artist":
			send(w, 200, map[string]any{"data": []map[string]any{{"id": 55, "name": "Artist", "picture_medium": "https://example.com/cover.jpg", "nb_fan": 20}}})
		case "/track/123":
			track := makeTrack(123, "Song")
			track.Artist.Name = "Artist"
			send(w, 200, track)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	f.a.deezerURL = server.URL
	c := soulKey(t, f.member, `["search:read","requests:read","requests:create"]`)
	tracks := soulData[struct {
		Tracks []soulTrack `json:"tracks"`
		Source string      `json:"source"`
	}](t, c.call("POST", "/api/v1/search/tracks", `{"query":"Song","limit":20}`))
	if len(tracks.Tracks) != 1 || tracks.Tracks[0].ID != "deezer:track:123" || tracks.Source != "deezer" {
		t.Fatal(tracks)
	}
	artists := soulData[struct {
		Artists []struct {
			ID string `json:"id"`
		} `json:"artists"`
	}](t, c.call("POST", "/api/v1/search/artists", `{"query":"Artist"}`))
	if len(artists.Artists) != 1 || artists.Artists[0].ID != "55" {
		t.Fatal(artists)
	}
	payload := `{"track_data":{"id":"deezer:track:123","name":"Song","artists":["Artist"],"album":{"name":"Album","images":[{"url":"https://example.com/cover.jpg"}]},"duration_ms":180000,"popularity":0},"source_type":"api","failure_reason":"Requested from Wavio"}`
	expect(t, c.call("POST", "/api/v1/wishlist", payload), 200)
	expect(t, c.call("POST", "/api/v1/wishlist", payload), 200)
	wishlist := soulData[struct {
		Tracks []struct {
			ID     int64  `json:"id"`
			Target string `json:"spotify_track_id"`
		} `json:"tracks"`
	}](t, c.call("GET", "/api/v1/wishlist", ""))
	if len(wishlist.Tracks) != 1 || wishlist.Tracks[0].Target != tracks.Tracks[0].ID {
		t.Fatal(wishlist)
	}
	expect(t, c.call("POST", "/api/v1/wishlist/process", `{"track_ids":["deezer:track:123","deezer:track:999"]}`), 404)
	var count int
	f.a.DB.QueryRow("SELECT count(*) FROM compatibility_downloads").Scan(&count)
	if count != 0 {
		t.Fatal("Partial selection submitted", count)
	}
	other := soulKey(t, f.direct, `["requests:read","requests:create"]`)
	expect(t, other.call("POST", "/api/v1/wishlist/process", `{"track_ids":["deezer:track:123"]}`), 404)
	for i := 0; i < 2; i++ {
		expect(t, c.call("POST", "/api/v1/wishlist/process", `{"track_ids":["deezer:track:123"]}`), 200)
	}
	type task struct {
		ID       string  `json:"id"`
		Status   string  `json:"status"`
		Progress float64 `json:"progress"`
		Error    string  `json:"error"`
	}
	type queue struct {
		Downloads []task `json:"downloads"`
		Total     int    `json:"total"`
	}
	pending := soulData[queue](t, c.call("GET", "/api/v1/downloads?limit=200", ""))
	if pending.Total != 1 || len(pending.Downloads) != 1 || pending.Downloads[0].Status != "pending" {
		t.Fatal(pending)
	}
	rid := pending.Downloads[0].ID
	expect(t, f.admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`, ""), 200)
	for _, state := range []struct {
		native, status string
		progress       float64
	}{{"awaiting_scan", "post_processing", 0}, {"failed", "failed", 0}, {"available", "completed", 100}} {
		if _, e := f.a.DB.Exec("UPDATE jobs SET state=?,stage='Verification detail' WHERE request_id=?", state.native, rid); e != nil {
			t.Fatal(e)
		}
		result := soulData[queue](t, c.call("GET", "/api/v1/downloads", ""))
		if result.Downloads[0].Status != state.status || result.Downloads[0].Progress != state.progress {
			t.Fatal(result)
		}
		if state.native == "failed" && result.Downloads[0].Error != "Verification detail" {
			t.Fatal(result)
		}
		list := soulData[struct {
			Tracks []json.RawMessage `json:"tracks"`
		}](t, c.call("GET", "/api/v1/wishlist", ""))
		if (len(list.Tracks) == 0) != (state.native == "available") {
			t.Fatal(state, list)
		}
	}
	if result := soulData[queue](t, other.call("GET", "/api/v1/downloads", "")); result.Total != 0 {
		t.Fatal(result)
	}
	// Removing an intent must not erase a running task or fabricate completion.
	expect(t, c.call("DELETE", "/api/v1/wishlist/deezer:track:123", ""), 200)
	if result := soulData[queue](t, c.call("GET", "/api/v1/downloads", "")); result.Total != 1 {
		t.Fatal(result)
	}
	// Both intent and terminal queue history are persistent, rather than in-memory.
	f.a.DB.Close()
	reopened, e := Open(filepath.Join(f.a.dataDir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	f.a.DB = reopened.DB
	if result := soulData[queue](t, c.call("GET", "/api/v1/downloads", "")); result.Total != 1 || result.Downloads[0].Status != "completed" {
		t.Fatal(result)
	}
}
func TestSoulSyncRecentlyAddedUsesNavidromeArtwork(t *testing.T) {
	f := newDeletionFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path != "/rest/getAlbumList2.view" || r.Form.Get("type") != "newest" {
			t.Errorf("Unexpected Navidrome request %s", r.URL.Path)
		}
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "albumList2": map[string]any{"album": []map[string]any{{"id": "nav-album", "name": "Album", "year": 2026, "coverArt": "cover-id", "created": "2026-11-05"}}}}})
	}))
	defer server.Close()
	if e := f.a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "test"}); e != nil {
		t.Fatal(e)
	}
	c := soulKey(t, f.member, `["library:read"]`)
	data := soulData[struct {
		Items []struct {
			Thumb  string `json:"thumb_url"`
			Server string `json:"server_source"`
		} `json:"items"`
	}](t, c.call("GET", "/api/v1/library/recently-added?type=albums&limit=20&fields=id,title,year,thumb_url,server_source,created_at", ""))
	if len(data.Items) != 1 || data.Items[0].Thumb != "/rest/getCoverArt?id=cover-id" || data.Items[0].Server != "navidrome" {
		t.Fatal(data)
	}
}
func TestAPIDiagnosticsExcludeCredentials(t *testing.T) {
	f := newDeletionFixture(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	c := soulTestClient{f.a, "secret-bearer-token", "1"}
	w := c.call("GET", "/api/v1/system/status?api_key=secret-query-token", "")
	expect(t, w, 401)
	for _, secret := range []string{"secret-bearer-token", "secret-query-token"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("Credentials leaked")
		}
	}
	for _, value := range []string{`"status":401`, `"path":"/api/v1/system/status"`, w.Header().Get("X-Request-Id"), "Invalid or expired API key"} {
		if value == "" || !strings.Contains(logs.String(), value) {
			t.Fatal(logs.String())
		}
	}
}
