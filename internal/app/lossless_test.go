package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type losslessTransport struct{ target *url.URL }

func (tr losslessTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = tr.target.Scheme
	u.Host = tr.target.Host
	clone.URL = &u
	return http.DefaultTransport.RoundTrip(clone)
}
func losslessFixture(t *testing.T, handler http.HandlerFunc) *App {
	t.Helper()
	a, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.DB.Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	a.HTTP = &http.Client{Transport: losslessTransport{u}}
	return a
}

const losslessSearchFixture = `{"tracks":[{"id":42,"title":"Song","duration":2,"source":"main","artists":[{"name":"Artist"}],"album":{"title":"Album","picture":{"medium":"https://example.com/cover"}},"isrc":null,"preview":"https://untrusted.test/audio","streaming_available":true}]}`

func TestLosslessSearchRequestIdentityAndSecrets(t *testing.T) {
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" || r.Method != "POST" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Cookie") != "session=private-value" {
			t.Error("missing session")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["query"] == "" {
			t.Error("missing search query")
		}
		io.WriteString(w, losslessSearchFixture)
	})
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	expect(t, admin.call("PUT", "/api/v2/connections", `{"kind":"lossless","arl":"session=private-value"}`, ""), 200)
	connections := admin.call("GET", "/api/v2/connections", "", "")
	if strings.Contains(connections.Body.String(), "private-value") {
		t.Fatal("session exposed")
	}
	var encrypted []byte
	a.DB.QueryRow("SELECT value FROM secrets WHERE name='lossless'").Scan(&encrypted)
	if strings.Contains(string(encrypted), "private-value") {
		t.Fatal("session stored in plaintext")
	}
	search := admin.call("GET", "/api/v2/search?provider=lossless&q=Artist&details=1", "", "")
	expect(t, search, 200)
	if !strings.Contains(search.Body.String(), `"provider":"lossless"`) || strings.Contains(search.Body.String(), "untrusted.test") {
		t.Fatal(search.Body.String())
	}
	request := admin.call("POST", "/api/v2/requests", `{"target":"lossless:track:main:42"}`, "lossless-request-1")
	expect(t, request, 201)
	expect(t, admin.call("POST", "/api/v2/requests", `{"target":"lossless:track:main:42"}`, "lossless-request-1"), 200)
	var target, metadata string
	a.DB.QueryRow("SELECT target,metadata FROM requests JOIN request_metadata ON requests.id=request_metadata.request_id").Scan(&target, &metadata)
	if target != "lossless:track:main:42" {
		t.Fatal(target)
	}
	var approved Track
	json.Unmarshal([]byte(metadata), &approved)
	other := approved
	other.Provider = ""
	if sameApprovedRecording(approved, other) {
		t.Fatal("cross-provider identity collision")
	}
	other = approved
	other.Source = "other"
	if sameApprovedRecording(approved, other) {
		t.Fatal("cross-source identity collision")
	}
	expect(t, admin.call("GET", "/api/v2/search?provider=unknown&q=x", "", ""), 400)
}
func TestLosslessDownloadsValidateAndPlace(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	audio := filepath.Join(t.TempDir(), "fixture.flac")
	out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "flac", "-metadata", "title=Song", "-metadata", "artist=Artist", "-metadata", "album=Album", audio).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v", out, err)
	}
	mode := "audio"
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search" {
			io.WriteString(w, losslessSearchFixture)
			return
		}
		if r.URL.Query().Get("id") != "42" || r.URL.Query().Get("source") != "main" {
			t.Error("incorrect download identity")
		}
		switch mode {
		case "redirect":
			http.Redirect(w, r, "https://untrusted.test/file", 302)
		case "json":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"error":"login required"}`)
		case "blocked":
			w.WriteHeader(451)
		default:
			w.Header().Set("Content-Type", "audio/flac")
			b, _ := os.ReadFile(audio)
			w.Write(b)
		}
	})
	a.musicDir = t.TempDir()
	tracks, err := (losslessProvider{a}).Search(context.Background(), "Song")
	if err != nil {
		t.Fatal(err)
	}
	track := tracks[0]
	for _, failure := range []string{"redirect", "json", "blocked"} {
		mode = failure
		_, _, err := a.acquireProvider(context.Background(), failure, trackTarget(track), track, `["lossless"]`)
		if err == nil {
			t.Fatalf("accepted %s", failure)
		}
	}
	mode = "audio"
	_, _, err = a.acquireProvider(context.Background(), "mp3", trackTarget(track), track, `["320"]`)
	if err == nil {
		t.Fatal("accepted unsupported quality")
	}
	rel, q, err := a.acquireProvider(context.Background(), "success", trackTarget(track), track, `["lossless"]`)
	if err != nil {
		t.Fatal(err)
	}
	if q != "lossless" {
		t.Fatal(q)
	}
	if _, err = os.Stat(filepath.Join(a.musicDir, rel)); err != nil {
		t.Fatal(err)
	}
	if err = a.validateAudio(filepath.Join(a.musicDir, rel), track, q); err != nil {
		t.Fatal(err)
	}
	// Deezer's source-ID validation remains mandatory.
	track.Provider = ""
	if err = a.validateAudio(filepath.Join(a.musicDir, rel), track, q); err == nil {
		t.Fatal("Deezer source ID validation bypassed")
	}
}

func TestLosslessRevalidationRejectsMissingIdentityAndDetectsChangedMetadata(t *testing.T) {
	response := losslessSearchFixture
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, response) })
	p := losslessProvider{a}
	tracks, err := p.Search(context.Background(), "Song")
	if err != nil {
		t.Fatal(err)
	}
	approved := tracks[0]
	response = strings.ReplaceAll(losslessSearchFixture, `"Song"`, `"Changed recording"`)
	current, err := a.track(context.Background(), trackTarget(approved))
	if err != nil {
		t.Fatal(err)
	}
	if sameApprovedRecording(approved, current) {
		t.Fatal("changed metadata accepted")
	}
	response = strings.ReplaceAll(losslessSearchFixture, `"source":"main"`, `"source":"other"`)
	if _, err = a.track(context.Background(), trackTarget(approved)); err == nil {
		t.Fatal("wrong source accepted")
	}
	response = `{"tracks":[]}`
	if _, err = a.track(context.Background(), trackTarget(approved)); err == nil {
		t.Fatal("missing recording accepted")
	}
}
