package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func authCookie(w http.ResponseWriter, name, value, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, Secure: true, HttpOnly: true, MaxAge: 86400})
}
func TestLosslessPasswordLoginAndRefresh(t *testing.T) {
	var refreshes atomic.Int32
	var searches atomic.Int32
	var rejectLogin atomic.Bool
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["email"] != "test@example.com" || body["password"] != "test-password" {
				t.Error("wrong login payload")
			}
			if rejectLogin.Load() {
				w.WriteHeader(401)
				io.WriteString(w, "test-password")
				return
			}
			authCookie(w, "jwt_access", "old-access", "/")
			authCookie(w, "jwt_refresh", "old-refresh", "/api/auth")
			authCookie(w, "has_session", "1", "/")
			http.SetCookie(w, &http.Cookie{Name: "auth_scope", Path: "/api/auth", MaxAge: -1, Secure: true})
		case "/api/me":
			if _, err := r.Cookie("jwt_refresh"); err == nil {
				t.Error("refresh cookie leaked to metadata endpoint")
			}
			io.WriteString(w, `{"user":{"id":7}}`)
		case "/api/auth/refresh":
			refreshes.Add(1)
			c, err := r.Cookie("jwt_refresh")
			if err != nil || c.Value != "old-refresh" {
				t.Error("missing refresh cookie")
			}
			authCookie(w, "jwt_access", "new-access", "/")
			authCookie(w, "jwt_refresh", "new-refresh", "/api/auth")
		case "/api/search":
			searches.Add(1)
			if _, err := r.Cookie("jwt_refresh"); err == nil {
				t.Error("refresh cookie leaked to search")
			}
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["query"] != "Song" {
				t.Error("POST body not replayed")
			}
			c, err := r.Cookie("jwt_access")
			if err != nil || c.Value != "new-access" {
				w.WriteHeader(401)
				return
			}
			io.WriteString(w, losslessSearchFixture)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	body := `{"kind":"lossless-login","email":"test@example.com","password":"test-password"}`
	expect(t, admin.call("PUT", "/api/v2/connections", body, ""), 200)
	session := (losslessProvider{a}).session()
	if session.Email != "test@example.com" || !session.has("jwt_refresh") {
		t.Fatal("session not persisted")
	}
	var raw []byte
	a.DB.QueryRow("SELECT value FROM secrets WHERE name=?", losslessSessionKey).Scan(&raw)
	if strings.Contains(string(raw), "old-refresh") || strings.Contains(string(raw), "test-password") {
		t.Fatal("plaintext credentials stored")
	}
	var decrypted map[string]json.RawMessage
	if err := a.unseal(losslessSessionKey, &decrypted); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(decrypted)
	if strings.Contains(string(encoded), "test-password") {
		t.Fatal("password persisted")
	}
	// Failed replacement leaves the working session intact and never echoes a provider error body.
	rejectLogin.Store(true)
	failed := admin.call("PUT", "/api/v2/connections", body, "")
	expect(t, failed, 400)
	if strings.Contains(failed.Body.String(), "test-password") {
		t.Fatal("password exposed")
	}
	if !(losslessProvider{a}).session().has("jwt_refresh") {
		t.Fatal("working session lost")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := (losslessProvider{a}).Search(context.Background(), "Song"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if refreshes.Load() != 1 || searches.Load() != 5 {
		t.Fatalf("refreshes=%d searches=%d", refreshes.Load(), searches.Load())
	}
	if (losslessProvider{a}).session().Cookies[0].Value == "old-access" {
		t.Fatal("rotation not saved")
	}
	connections := admin.call("GET", "/api/v2/connections", "", "")
	if strings.Contains(connections.Body.String(), "new-access") || strings.Contains(connections.Body.String(), "test@example.com") {
		t.Fatal("credentials exposed")
	}
}
func TestLosslessRefreshFailureDoesNotLoop(t *testing.T) {
	for _, status := range []int{401, 451, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			refreshes := 0
			a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/auth/refresh" {
					refreshes++
					w.WriteHeader(status)
					return
				}
				w.WriteHeader(401)
			})
			if err := a.seal(losslessSessionKey, losslessSession{Cookies: []*http.Cookie{{Name: "jwt_refresh", Value: "refresh", Path: "/api/auth", Secure: true}}}); err != nil {
				t.Fatal(err)
			}
			_, err := (losslessProvider{a}).Search(context.Background(), "Song")
			if err == nil || refreshes != 1 {
				t.Fatalf("err=%v refreshes=%d", err, refreshes)
			}
		})
	}
}
func TestLosslessExpiredRefreshIsNotUsed(t *testing.T) {
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			t.Error("expired refresh used")
		}
		if _, err := r.Cookie("jwt_access"); err == nil {
			t.Error("expired access expiry extended")
		}
		w.WriteHeader(401)
	})
	expired := time.Now().Add(-time.Hour)
	session := losslessSession{Cookies: []*http.Cookie{
		{Name: "jwt_access", Value: "access", Path: "/", Secure: true, MaxAge: 86400, Expires: expired},
		{Name: "jwt_refresh", Value: "refresh", Path: "/api/auth", Secure: true, MaxAge: 86400, Expires: expired},
	}}
	if err := a.seal(losslessSessionKey, session); err != nil {
		t.Fatal(err)
	}
	if _, err := (losslessProvider{a}).Search(context.Background(), "Song"); err == nil {
		t.Fatal("expired session accepted")
	}
}
