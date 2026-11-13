package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeezerFullStreamUsesLowQualityWithoutAcquisition(t *testing.T) {
	track := makeTrack(42, "Song")
	track.Duration = 2
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) { send(w, 200, track) })
	a.deezerURL = "https://api.deezer.com"
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	c := login(t, a, "admin")
	call := func(client client, sid, byteRange string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", c.baseURL()+"/api/v2/providers/deezer/stream?"+url.Values{"target": {"deezer:track:42"}, "sid": {sid}}.Encode(), nil)
		if client.cookie != nil {
			r.AddCookie(client.cookie)
		}
		r.Header.Set("Range", byteRange)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		return w
	}
	expect(t, call(client{a: a}, "session-123", ""), 401)
	expect(t, call(c, "session-123", ""), 409)
	if err := a.seal("deezer", "secret-arl"); err != nil {
		t.Fatal(err)
	}
	expect(t, call(c, "bad", ""), 400)
	expect(t, call(c, "session-123", "bytes=0-1,4-5"), 416)
	audio := filepath.Join(t.TempDir(), "song.mp3")
	output, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "libmp3lame", "-b:a", "128k", "-metadata", "title=Song", "-metadata", "artist="+track.Artist.Name, "-metadata", "album="+track.Album.Title, "-metadata", "sourceid=42", audio).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v", output, err)
	}
	count := filepath.Join(t.TempDir(), "runs")
	a.dfiPath = filepath.Join(t.TempDir(), "fake-dfi")
	// The only process allowed for listening requests exactly 128, never fallback.
	script := "#!/bin/sh\ncase \"$*\" in *'--quality 128'*) ;; *) exit 2;; esac\nprintf 'run\\n' >> '" + count + "'\ncp '" + audio + "' audio.mp3\n"
	if err = os.WriteFile(a.dfiPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	w := call(c, "session-123", "")
	expect(t, w, 200)
	data, _ := os.ReadFile(audio)
	if w.Body.String() != string(data) || w.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatal("audio not served")
	}
	partial := call(c, "session-123", "bytes=4-7")
	expect(t, partial, 206)
	if partial.Body.String() != string(data[4:8]) {
		t.Fatal("seek bytes differ")
	}
	expect(t, call(c, "session-123", "bytes=999999-"), 416)
	runs, _ := os.ReadFile(count)
	if string(runs) != "run\n" {
		t.Fatal("seeking fetched the song again")
	}
	dirs, _ := filepath.Glob(filepath.Join(a.dataDir, "preview-*"))
	if len(dirs) != 0 {
		t.Fatal("temporary credentials or audio retained on disk")
	}
	for _, table := range []string{"requests", "jobs", "assets"} {
		var count int
		a.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
		if count != 0 {
			t.Fatal("listening created " + table)
		}
	}
	if err := a.seal("deezer", "replacement-arl"); err != nil {
		t.Fatal(err)
	}
	expect(t, call(c, "session-123", ""), 200)
	runs, _ = os.ReadFile(count)
	if string(runs) != "run\nrun\n" {
		t.Fatal("cached audio crossed credential change")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.deezerPreviewAudio(cancelled, "new-session", "deezer:track:42", "secret-arl"); err == nil {
		t.Fatal("cancelled preview continued")
	}
	if strings.Contains(w.Header().Get("Content-Disposition"), "secret") {
		t.Fatal("credential exposed")
	}
}
