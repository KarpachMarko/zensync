package app

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func streamFixture(t *testing.T, handler http.HandlerFunc) (*App, client) {
	t.Helper()
	a := losslessFixture(t, handler)
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	c := login(t, a, "admin")
	if err := a.seal("lossless", "jwt_access=private-access; jwt_refresh=private-refresh"); err != nil {
		t.Fatal(err)
	}
	track := makeTrack(42, "Stream song")
	track.Provider = "lossless"
	track.Source = "main"
	if _, err := a.DB.Exec("INSERT INTO provider_tracks(target,metadata) VALUES(?,?)", trackTarget(track), mustJSON(track)); err != nil {
		t.Fatal(err)
	}
	return a, c
}
func streamCall(c client, target, sid, byteRange string) *httptest.ResponseRecorder {
	path := "/api/v2/providers/lossless/stream?" + url.Values{"target": {target}, "sid": {sid}}.Encode()
	r := httptest.NewRequest("GET", c.baseURL()+path, nil)
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	if byteRange != "" {
		r.Header.Set("Range", byteRange)
	}
	w := httptest.NewRecorder()
	c.a.Handler().ServeHTTP(w, r)
	return w
}
func TestLosslessStreamProxiesRangesWithoutExposingTokens(t *testing.T) {
	tokens := 0
	a, c := streamFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("jwt_refresh"); err == nil {
			t.Error("refresh cookie escaped auth scope")
		}
		if r.Header.Get("User-Agent") != losslessUserAgent {
			t.Error("missing compatible user agent")
		}
		if r.URL.Path == "/api/stream/token" {
			tokens++
			if r.Method != "POST" {
				t.Error("wrong token method")
			}
			send(w, 200, map[string]string{"stream_token": "private-stream-token"})
			return
		}
		if r.URL.Path != "/api/stream/track" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("id") != "42" || q.Get("source") != "main" || q.Get("sid") != "session-123" || q.Get("_token") != "private-stream-token" {
			t.Error("incorrect stream request")
		}
		w.Header().Set("Content-Type", "audio/mp4")
		w.Header().Set("Set-Cookie", "jwt_access=do-not-forward")
		w.Header().Set("Location", "https://lossless.am/?_token=do-not-forward")
		if r.Header.Get("Range") == "bytes=4-7" {
			w.Header().Set("Content-Range", "bytes 4-7/10")
			w.Header().Set("Content-Length", "4")
			w.WriteHeader(206)
			io.WriteString(w, "4567")
			return
		}
		w.Header().Set("Content-Length", "10")
		io.WriteString(w, "0123456789")
	})
	for _, byteRange := range []string{"", "bytes=4-7"} {
		w := streamCall(c, "lossless:track:main:42", "session-123", byteRange)
		code, want := 200, "0123456789"
		if byteRange != "" {
			code, want = 206, "4567"
		}
		expect(t, w, code)
		if w.Body.String() != want || w.Header().Get("Content-Type") != "audio/mp4" || w.Header().Get("Accept-Ranges") != "bytes" {
			t.Fatalf("invalid stream: %v %s", w.Header(), w.Body)
		}
		if byteRange != "" && w.Header().Get("Content-Range") != "bytes 4-7/10" {
			t.Fatal("seek range lost")
		}
		if w.Header().Get("Set-Cookie") != "" || w.Header().Get("Location") != "" || strings.Contains(fmt.Sprint(w.Header()), "private-") {
			t.Fatal("provider credentials exposed")
		}
	}
	if tokens != 2 {
		t.Fatalf("unexpected token requests %d", tokens)
	}
	var requests int
	a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&requests)
	if requests != 0 {
		t.Fatal("streaming created an acquisition")
	}
	expect(t, streamCall(client{a: a}, "lossless:track:main:42", "session-123", ""), 401)
	before := tokens
	expect(t, streamCall(c, "lossless:track:main:43", "session-123", ""), 404)
	expect(t, streamCall(c, "https://evil.test/track/42", "session-123", ""), 400)
	expect(t, streamCall(c, "lossless:track:main:42", "session-123", "bytes=0-1,5-6"), 416)
	expect(t, streamCall(c, "lossless:track:main:42", "bad", ""), 400)
	if tokens != before {
		t.Fatal("invalid stream reached the provider")
	}
}
func TestLosslessStreamRenewsRejectedTokenOnce(t *testing.T) {
	tokens, streams := 0, 0
	_, c := streamFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stream/token" {
			tokens++
			send(w, 200, map[string]string{"stream_token": "token-" + strconv.Itoa(tokens)})
			return
		}
		streams++
		if streams == 1 {
			w.WriteHeader(401)
			return
		}
		if r.URL.Query().Get("_token") != "token-2" {
			t.Error("stale token reused")
		}
		w.Header().Set("Content-Type", "audio/mp4")
		io.WriteString(w, "audio")
	})
	expect(t, streamCall(c, "lossless:track:main:42", "session-123", ""), 200)
	if tokens != 2 || streams != 2 {
		t.Fatalf("tokens=%d streams=%d", tokens, streams)
	}
}
func TestLosslessStreamRejectsErrorsAndNonAudio(t *testing.T) {
	for _, mode := range []string{"json", "redirect", "expired", "oversized", "range"} {
		t.Run(mode, func(t *testing.T) {
			streams := 0
			_, c := streamFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/stream/token" {
					send(w, 200, map[string]string{"stream_token": "private-token"})
					return
				}
				streams++
				switch mode {
				case "json":
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"error":"private-token"}`)
				case "redirect":
					http.Redirect(w, r, "https://evil.test/?token=private-token", 302)
				case "expired":
					w.WriteHeader(401)
				case "oversized":
					w.Header().Set("Content-Type", "audio/mp4")
					w.Header().Set("Content-Length", "2147483648")
				case "range":
					w.Header().Set("Content-Range", "bytes */10")
					w.WriteHeader(416)
				}
			})
			w := streamCall(c, "lossless:track:main:42", "session-123", "bytes=99-")
			code := 502
			if mode == "range" {
				code = 416
			}
			expect(t, w, code)
			if strings.Contains(w.Body.String(), "private-token") || strings.Contains(w.Body.String(), "evil.test") {
				t.Fatal("upstream error exposed")
			}
			max := 1
			if mode == "expired" {
				max = 2
			}
			if streams != max {
				t.Fatalf("unexpected retries: %d", streams)
			}
		})
	}
}
