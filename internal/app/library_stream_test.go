package app

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLibraryPlaybackProxiesNavidromeWithSeekingAndNoCredentialLeak(t *testing.T) {
	mode := "audio"
	streams := 0
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Header.Get("Cookie") != "" {
			t.Error("browser cookie forwarded")
		}
		switch r.URL.Path {
		case "/rest/getSong.view":
			send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "song": Song{ID: r.Form.Get("id"), Title: "Song"}}})
		case "/rest/stream.view":
			streams++
			if r.Form.Get("u") != "service" || r.Form.Get("t") == "" || r.Form.Get("maxBitRate") != "128" || r.Form.Get("format") != "mp3" || r.Form.Get("id") != "song" {
				t.Fatal("invalid stream parameters")
			}
			if mode == "html" {
				w.Header().Set("Content-Type", "text/html")
				io.WriteString(w, "login")
				return
			}
			if mode == "redirect" {
				http.Redirect(w, r, "https://untrusted.test/audio", 302)
				return
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Set-Cookie", "private=secret")
			if r.Header.Get("Range") == "bytes=4-7" {
				w.Header().Set("Content-Range", "bytes 4-7/10")
				w.WriteHeader(206)
				io.WriteString(w, "4567")
				return
			}
			if r.Header.Get("Range") == "bytes=99-" {
				w.Header().Set("Content-Range", "bytes */10")
				w.WriteHeader(416)
				return
			}
			io.WriteString(w, "0123456789")
		case "/rest/getCoverArt.view":
			data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
			w.Write(data)
		default:
			t.Error(r.URL.Path)
		}
	})
	a.seal("navidrome", Credentials{URL: "http://navidrome.test", Username: "service", Password: "never-exposed"})
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	path := "/api/v2/library/stream?id=song"
	expect(t, client{a: a}.call("GET", path, "", ""), 401)
	full := member.call("GET", path, "", "")
	expect(t, full, 200)
	if full.Body.String() != "0123456789" || full.Header().Get("Set-Cookie") != "" || strings.Contains(full.Body.String(), "never-exposed") {
		t.Fatal("stream or credentials changed")
	}
	ranged := func(value string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", member.baseURL()+path, nil)
		r.AddCookie(member.cookie)
		r.Header.Set("Range", value)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		return w
	}
	partial := ranged("bytes=4-7")
	expect(t, partial, 206)
	if partial.Body.String() != "4567" || partial.Header().Get("Content-Range") != "bytes 4-7/10" {
		t.Fatal("seek range lost")
	}
	expect(t, ranged("bytes=99-"), 416)
	before := streams
	expect(t, ranged("bytes=1-2,3-4"), 416)
	if streams != before {
		t.Fatal("invalid range reached upstream")
	}
	for _, failure := range []string{"html", "redirect"} {
		mode = failure
		expect(t, member.call("GET", path, "", ""), 502)
	}
	cover := member.call("GET", "/api/v2/library/cover?id=cover", "", "")
	expect(t, cover, 200)
	if cover.Header().Get("Content-Type") != "image/png" {
		t.Fatal("cover not served")
	}
	for _, table := range []string{"requests", "jobs", "metadata_jobs"} {
		var count int
		a.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
		if count != 0 {
			t.Fatal("listening created " + table)
		}
	}
}
