package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type client struct {
	a      *App
	cookie *http.Cookie
	origin string
	csrf   string
}

func (c client) baseURL() string {
	if c.origin != "" {
		return c.origin
	}
	return "http://localhost"
}
func (c client) call(method, path, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, c.baseURL()+path, bytes.NewBufferString(body))
	r.Header.Set("Origin", c.baseURL())
	r.Header.Set("X-CSRF-Token", c.csrf)
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.a.Handler().ServeHTTP(w, r)
	return w
}
func login(t *testing.T, a *App, name string, origins ...string) client {
	t.Helper()
	c := client{a: a}
	if len(origins) > 0 {
		c.origin = origins[0]
	}
	w := c.call("POST", "/api/v2/session", `{"username":"`+name+`","password":"test-password-long"}`, "")
	if w.Code != 200 {
		t.Fatalf("login %d %s", w.Code, w.Body)
	}
	var s struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &s)
	c.csrf = s.CSRF
	c.cookie = w.Result().Cookies()[0]
	return c
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("want %d got %d: %s", status, w.Code, w.Body)
	}
}
func fixtureMetadata(t *testing.T, a *App) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/track/"))
		title := "Private title"
		if id == 456 {
			title = "Other"
		}
		tr := makeTrack(id, title)
		tr.Artist.Name = "Artist"
		send(w, 200, tr)
	}))
	t.Cleanup(server.Close)
	a.deezerURL = server.URL
}
func TestFoundation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	a, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { a.DB.Close() }()
	fixtureMetadata(t, a)
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	admin := login(t, a, "admin")
	for _, name := range []string{"alice", "bob", "direct"} {
		body := `{"username":"` + name + `","password":"test-password-long","direct":`
		if name == "direct" {
			body += "true}"
		} else {
			body += "false}"
		}
		expect(t, admin.call("POST", "/api/v2/users", body, ""), 201)
	}
	alice, bob, direct := login(t, a, "alice"), login(t, a, "bob"), login(t, a, "direct")
	payload := `{"target":"deezer:track:123","title":"Private title","artist":"Artist","upgrade":false}`
	w := alice.call("POST", "/api/v2/requests", payload, "request-key-1")
	expect(t, w, 201)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	rid := result["id"]
	expect(t, alice.call("POST", "/api/v2/requests", payload, "request-key-1"), 200)
	expect(t, alice.call("POST", "/api/v2/requests", `{"target":"deezer:track:456","title":"Other","artist":"","upgrade":false}`, "request-key-1"), 409)
	var jobs int
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&jobs)
	if jobs != 0 {
		t.Fatal("unapproved request created job")
	}
	if w = bob.call("GET", "/api/v2/requests", "", ""); bytes.Contains(w.Body.Bytes(), []byte("Private title")) {
		t.Fatal("private metadata leaked")
	}
	expect(t, bob.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved","reason":""}`, ""), 404)
	expect(t, alice.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved","reason":""}`, ""), 403)
	expect(t, alice.call("POST", "/api/v2/users", `{"username":"evil","password":"test-password-long","direct":true}`, ""), 404)
	bad := alice
	bad.csrf = "invalid"
	expect(t, bad.call("POST", "/api/v2/requests", payload, "request-key-2"), 403)
	r := httptest.NewRequest("POST", "http://localhost/api/v2/requests", bytes.NewBufferString(payload))
	r.AddCookie(alice.cookie)
	r.Header.Set("Origin", "http://evil.example")
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	expect(t, w, 403)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved","reason":"Reviewed"}`, "").Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("approval race: %v", counts)
	}
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&jobs)
	if jobs != 1 {
		t.Fatalf("duplicate job: %d", jobs)
	}
	expect(t, direct.call("POST", "/api/v2/requests", `{"target":"deezer:track:123","title":"Upgrade","artist":"","upgrade":true}`, "upgrade-key"), 201)
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&jobs)
	if jobs != 1 {
		t.Fatal("direct grant bypassed upgrade approval")
	}
	expect(t, alice.call("PUT", "/api/v2/quality", `{"quality":["128"]}`, ""), 200)
	var quality string
	a.DB.QueryRow("SELECT quality FROM requests WHERE id=?", rid).Scan(&quality)
	if quality != `["lossless","320","128"]` {
		t.Fatal("quality snapshot changed")
	}
	expect(t, direct.call("POST", "/api/v2/requests", payload, "direct-key"), 201)
	expect(t, alice.call("POST", "/api/v2/requests", payload, "withdraw-key"), 201)
	var withdrawID string
	a.DB.QueryRow("SELECT id FROM requests WHERE idem='withdraw-key'").Scan(&withdrawID)
	expect(t, alice.call("POST", "/api/v2/requests/"+withdrawID, `{"decision":"withdrawn","reason":""}`, ""), 200)
	if w = bob.call("GET", "/api/v2/activity", "", ""); bytes.Contains(w.Body.Bytes(), []byte("Private title")) {
		t.Fatal("activity leaked")
	}
	a.DB.Close()
	a, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	alice.a = a
	expect(t, alice.call("GET", "/api/v2/session", "", ""), 200)
	a.DB.QueryRow("SELECT count(*) FROM jobs WHERE state='queued'").Scan(&jobs)
	if jobs != 2 {
		t.Fatalf("restart lost jobs: %d", jobs)
	}
	expect(t, alice.call("DELETE", "/api/v2/session", "", ""), 200)
	expect(t, alice.call("GET", "/api/v2/session", "", ""), 401)
}
func TestBootstrapAndLimits(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	if e = a.createAdmin("admin", "short"); e == nil {
		t.Fatal("accepted weak bootstrap")
	}
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	c := login(t, a, "admin", "https://music.example")
	if !c.cookie.Secure || !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	expect(t, c.call("POST", "/api/v2/requests", `{"owner":"someone","target":"deezer:track:1"}`, "test-key"), 400)
	for range 19 {
		c.call("POST", "/api/v2/session", `{"username":"missing","password":"wrong"}`, "")
	}
	expect(t, c.call("POST", "/api/v2/session", `{}`, ""), 429)
}

func TestSchemaRejectsNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.DB.Exec("INSERT INTO schema_version VALUES(99)"); err != nil {
		t.Fatal(err)
	}
	a.DB.Close()
	if other, err := Open(path); err == nil {
		other.DB.Close()
		t.Fatal("accepted a newer schema")
	}
}

func TestRequestOrigin(t *testing.T) {
	a, err := Open(filepath.Join(t.TempDir(), "origins.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	if err = a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"http://192.168.1.5:8080", "http://homelab:9090", "https://music.example"} {
		c := login(t, a, "admin", origin)
		expect(t, c.call("GET", "/api/v2/session", "", ""), 200)
		expect(t, c.call("DELETE", "/api/v2/session", "", ""), 200)
		if c.cookie.Secure != strings.HasPrefix(origin, "https:") {
			t.Fatal("incorrect cookie security")
		}
	}
	for _, origin := range []string{"", "null", "http://evil.example", "http://homelab:9091", "http://homelab:9090/path", "http://user@homelab:9090", "http://homelab:9090?", "ftp://homelab:9090"} {
		r := httptest.NewRequest("POST", "http://homelab:9090/api/v2/session", strings.NewReader(`{}`))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		expect(t, w, 403)
	}
	r := httptest.NewRequest("POST", "http://homelab:9090/api/v2/session", strings.NewReader(`{}`))
	r.Header.Set("Origin", "http://evil.example")
	r.Header.Set("X-Forwarded-Host", "evil.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	expect(t, w, 403)
	r.Header.Set("Origin", "http://homelab:9090")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	expect(t, w, 403)
}

func TestTLSProxyLogin(t *testing.T) {
	a, err := Open(filepath.Join(t.TempDir(), "proxy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	if err = a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	// The proxy terminates TLS and forwards HTTP with the original public Host.
	r := httptest.NewRequest("POST", "http://music.example/api/v2/session", strings.NewReader(`{"username":"admin","password":"test-password-long"}`))
	r.Header.Set("Origin", "https://music.example")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	expect(t, w, 200)
	if !w.Result().Cookies()[0].Secure {
		t.Fatal("TLS proxy login needs a Secure cookie")
	}
}
