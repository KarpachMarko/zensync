package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func hitmoStreamCall(c client, target, sid, byteRange string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", c.baseURL()+"/api/v2/providers/hitmo/stream?"+url.Values{"target": {target}, "sid": {sid}}.Encode(), nil)
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	r.Header.Set("Range", byteRange)
	w := httptest.NewRecorder()
	c.a.Handler().ServeHTTP(w, r)
	return w
}
func TestHitmoStreamWithoutRequest(t *testing.T) {
	mode := "audio"
	audioRequests := 0
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("browser or provider credentials forwarded")
		}
		if r.URL.Path == "/search" || r.URL.Path == "/song/42" {
			io.WriteString(w, hitmoFixture)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/get/music/") {
			http.Redirect(w, r, "https://cdn13.deliciousoranges.com/s1"+r.URL.Path, 302)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/s1/get/music/") {
			t.Error(r.URL.Path)
		}
		audioRequests++
		if mode == "html" {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, "Login")
			return
		}
		if mode == "bad-range" {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.WriteHeader(206)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Set-Cookie", "sid=private")
		switch r.Header.Get("Range") {
		case "bytes=4-7":
			w.Header().Set("Content-Range", "bytes 4-7/10")
			w.Header().Set("Content-Length", "4")
			w.WriteHeader(206)
			io.WriteString(w, "4567")
		case "bytes=99-":
			w.Header().Set("Content-Range", "bytes */10")
			w.WriteHeader(416)
		default:
			w.Header().Set("Content-Length", "10")
			io.WriteString(w, "0123456789")
		}
	})
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	c := login(t, a, "admin")
	expect(t, hitmoStreamCall(c, "hitmo:track:42", "session-123", ""), 404)
	if _, err := (hitmoProvider{a}).Search(context.Background(), "Song"); err != nil {
		t.Fatal(err)
	}
	for _, byteRange := range []string{"", "bytes=4-7", "bytes=99-"} {
		w := hitmoStreamCall(c, "hitmo:track:42", "session-123", byteRange)
		switch byteRange {
		case "bytes=4-7":
			expect(t, w, 206)
			if w.Body.String() != "4567" || w.Header().Get("Content-Range") != "bytes 4-7/10" {
				t.Fatal("seek range lost")
			}
		case "bytes=99-":
			expect(t, w, 416)
			if w.Header().Get("Content-Range") != "bytes */10" {
				t.Fatal("unsatisfied range lost")
			}
		default:
			expect(t, w, 200)
			if w.Body.String() != "0123456789" {
				t.Fatal("truncated stream")
			}
		}
		if w.Header().Get("Set-Cookie") != "" || w.Header().Get("Location") != "" {
			t.Fatal("provider headers leaked")
		}
	}
	before := audioRequests
	expect(t, hitmoStreamCall(client{a: a}, "hitmo:track:42", "session-123", ""), 401)
	expect(t, hitmoStreamCall(c, "https://evil.test/song/42", "session-123", ""), 400)
	expect(t, hitmoStreamCall(c, "hitmo:track:42", "bad", ""), 400)
	expect(t, hitmoStreamCall(c, "hitmo:track:42", "session-123", "bytes=0-1,3-4"), 416)
	if before != audioRequests {
		t.Fatal("invalid playback reached audio provider")
	}
	for _, failure := range []string{"html", "bad-range"} {
		mode = failure
		expect(t, hitmoStreamCall(c, "hitmo:track:42", "session-123", ""), 502)
	}
	for _, table := range []string{"requests", "jobs", "assets"} {
		var count int
		if err := a.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("playback changed %s: %d %v", table, count, err)
		}
	}
}
