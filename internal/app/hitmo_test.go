package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const hitmoFixture = `<form class="search-form"></form><li class="track" data-musmeta='{"id":"track-id-42","title":"Song &amp; Melody","artist":"Artist","img":"https://statcore.hitmcdn.com/cover.jpg"}'><div class="track__fulltime">00:02</div><a class="track__download-btn" href="/get/music/20260506/Artist_-_Song_42.mp3"></a></li>`

func TestHitmoSearchAndRequest(t *testing.T) {
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("credentials sent to Hitmo")
		}
		if r.URL.Path != "/search" && r.URL.Path != "/song/42" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Path == "/search" && r.URL.Query().Get("q") != "Artist & Song" {
			t.Error("query not encoded correctly")
		}
		io.WriteString(w, hitmoFixture)
	})
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	result := admin.call("GET", "/api/v2/search?provider=hitmo&q=Artist%20%26%20Song&details=1", "", "")
	expect(t, result, 200)
	if !strings.Contains(result.Body.String(), `"provider":"hitmo"`) || !strings.Contains(result.Body.String(), `Song \u0026 Melody`) {
		t.Fatal(result.Body.String())
	}
	expect(t, admin.call("POST", "/api/v2/requests", `{"target":"hitmo:track:42"}`, "hitmo-request-1"), 201)
	expect(t, admin.call("POST", "/api/v2/requests", `{"target":"hitmo:track:42"}`, "hitmo-request-1"), 200)
	track, err := a.track(context.Background(), hitmoOrigin+"/song/42")
	if err != nil {
		t.Fatal(err)
	}
	if !sameApprovedRecording(track, track) {
		t.Fatal("unchanged Hitmo recording rejected")
	}
	deezer := track
	deezer.Provider = ""
	if sameApprovedRecording(track, deezer) || audioApprovalKey(track) == audioApprovalKey(deezer) {
		t.Fatal("cross-provider identity collision")
	}
}
func TestHitmoURLsAndPages(t *testing.T) {
	for _, raw := range []string{"http://eu.hitmoz.com/song/42", "https://evil.test/song/42", "https://eu.hitmoz.com@evil.test/song/42", "https://eu.hitmoz.com/song/0", "https://eu.hitmoz.com/song/42?x=1", "hitmo:track:0"} {
		if _, err := hitmoTarget(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"https://evil.test/get/music/42.mp3", "//evil.test/get/music/42.mp3", "/get/cuts/42.mp3", "/get/music/../secret.mp3", "http://eu.hitmoz.com/get/music/42.mp3"} {
		if _, err := hitmoDownloadURL(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"/get/music/42.mp3", "//eu.hitmoz.com/get/music/42.mp3"} {
		if _, err := hitmoDownloadURL(raw); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := parseHitmo(strings.NewReader("<html>Access denied</html>")); err == nil {
		t.Fatal("challenge treated as empty results")
	}
	tracks, err := parseHitmo(strings.NewReader(strings.ReplaceAll(hitmoFixture, "00:02", "00:99")))
	if err != nil || len(tracks) != 0 {
		t.Fatal("invalid duration accepted")
	}
}
func TestHitmoAcquisition(t *testing.T) {
	audio := filepath.Join(t.TempDir(), "song.mp3")
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	output, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "libmp3lame", "-b:a", "192k", "-metadata", "title=Song & Melody", "-metadata", "artist=Artist", audio).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	mode := "audio"
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/song/42" {
			io.WriteString(w, hitmoFixture)
			return
		}
		if r.URL.Path == "/s1/get/music/20260506/Artist_-_Song_42.mp3" {
			w.Header().Set("Content-Type", "audio/mpeg")
			data, _ := os.ReadFile(audio)
			w.Write(data)
			return
		}
		if r.URL.Path != "/get/music/20260506/Artist_-_Song_42.mp3" {
			t.Error(r.URL.Path)
		}
		switch mode {
		case "cdn":
			http.Redirect(w, r, "https://cdn13.deliciousoranges.com/s1"+r.URL.Path, 302)
		case "wrong-cdn-file":
			http.Redirect(w, r, "https://cdn13.deliciousoranges.com/s1/get/music/other.mp3", 302)
		case "redirect":
			http.Redirect(w, r, "https://evil.test/file", 302)
		case "html":
			io.WriteString(w, "<html>Denied</html>")
		default:
			w.Header().Set("Content-Type", "audio/mpeg")
			data, _ := os.ReadFile(audio)
			w.Write(data)
		}
	})
	a.dataDir = t.TempDir()
	a.musicDir = t.TempDir()
	a.ffprobePath = "ffprobe"
	p := hitmoProvider{a}
	track, err := p.Track(context.Background(), "hitmo:track:42")
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		mode, profile string
		success       bool
	}{{"audio", `["lossless"]`, false}, {"audio", `["320"]`, false}, {"html", `["128"]`, false}, {"redirect", `["128"]`, false}, {"wrong-cdn-file", `["128"]`, false}, {"cdn", `["128"]`, true}, {"audio", `["128"]`, true}} {
		mode = tc.mode
		rel, quality, err := p.Acquire(context.Background(), fmt.Sprintf("job-%d", i), "hitmo:track:42", track, tc.profile)
		if tc.success {
			if err != nil || rel == "" || quality != "mp3:192" {
				t.Fatalf("%s %s %v", rel, quality, err)
			}
		} else if err == nil {
			t.Fatalf("accepted %s %s", tc.mode, tc.profile)
		}
	}
	track.Title = "Different"
	if _, _, err := p.Acquire(context.Background(), "changed", "hitmo:track:42", track, `["128"]`); err == nil {
		t.Fatal("accepted changed approval")
	}
}

// Opt-in smoke test: exercises the current public site without credentials.
func TestHitmoLive(t *testing.T) {
	if os.Getenv("ZENSYNC_TEST_HITMO_LIVE") != "1" {
		t.Skip("set ZENSYNC_TEST_HITMO_LIVE=1 to check the public provider")
	}
	a, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	a.musicDir = t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p := hitmoProvider{a}
	tracks, err := p.Search(ctx, "ENZRO")
	if err != nil || len(tracks) == 0 {
		t.Fatalf("search: %d tracks, %v", len(tracks), err)
	}
	track, err := p.Track(ctx, hitmoOrigin+"/song/81423717")
	if err != nil {
		t.Fatal(err)
	}
	if !sameApprovedRecording(track, track) {
		t.Fatal("recording cannot pass revalidation")
	}
	rel, quality, err := p.Acquire(ctx, "live-smoke", trackTarget(track), track, `["320","128"]`)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Validated live download: %s (%s)", rel, quality)
	request := httptest.NewRequest("GET", "/api/v2/providers/hitmo/stream?target="+url.QueryEscape(trackTarget(track))+"&sid=live-session-123", nil).WithContext(ctx)
	request.Header.Set("Range", "bytes=0-1023")
	response := httptest.NewRecorder()
	a.streamHitmo(response, request)
	if response.Code != 206 || response.Body.Len() != 1024 {
		t.Fatalf("live seek: %d %d bytes: %s", response.Code, response.Body.Len(), response.Body.String())
	}
	t.Log("Validated live full-song streaming byte range")
}
