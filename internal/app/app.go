package app

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type App struct {
	transferMu sync.Mutex
	transfers  map[string]TransferProgress

	albumMu          sync.Mutex
	activeOwners     map[string]int
	scanRequestedAt  time.Time
	workMu           sync.Mutex
	placementMu      sync.Mutex
	scanMu           sync.Mutex
	activeJobs       map[string]string
	activeTargets    map[string]bool
	importMu         sync.Mutex
	providerMu       sync.Mutex
	lastMetadata     time.Time
	metadataInterval time.Duration
	DB               *sql.DB
	mu               sync.Mutex
	failures         []time.Time
	opMu             sync.Mutex
	key              []byte
	dataDir          string
	musicDir         string
	dfiPath          string
	ffprobePath      string
	HTTP             *http.Client
	deezerURL        string
}
type User struct {
	ID           string          `json:"id"`
	Username     string          `json:"username"`
	Role         string          `json:"role"`
	DirectDelete bool            `json:"directDelete"`
	Direct       bool            `json:"direct"`
	Quality      json.RawMessage `json:"quality"`
}
type Request struct {
	Transfer   *TransferProgress `json:"transfer,omitempty"`
	State      string            `json:"state"`
	Detail     string            `json:"detail,omitempty"`
	Kind       string            `json:"kind"`
	TrackCount int               `json:"trackCount,omitempty"`
	Items      []AlbumItem       `json:"items,omitempty"`
	ID         string            `json:"id"`
	Owner      string            `json:"owner"`
	Target     string            `json:"target"`
	Title      string            `json:"title"`
	Artist     string            `json:"artist"`
	Upgrade    bool              `json:"upgrade"`
	Quality    json.RawMessage   `json:"quality"`
	Decision   string            `json:"decision"`
	Reason     string            `json:"reason"`
	Created    string            `json:"created"`
}

func Open(path string) (*App, error) {
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, e = db.Exec(q); e != nil {
			db.Close()
			return nil, e
		}
	}
	tx, e := db.Begin()
	if e != nil {
		db.Close()
		return nil, e
	}
	if _, e = tx.Exec(schema); e == nil {
		var version int
		e = tx.QueryRow("SELECT max(version) FROM schema_version").Scan(&version)
		if e == nil && version > 4 {
			e = errors.New("database schema is newer than this application")
		}
	}
	if e == nil {
		e = tx.Commit()
	} else {
		_ = tx.Rollback()
	}
	if e != nil {
		db.Close()
		return nil, e
	}
	a := &App{DB: db, HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}, deezerURL: "https://api.deezer.com", metadataInterval: 250 * time.Millisecond}
	if e = a.initRuntime(path); e != nil {
		db.Close()
		return nil, e
	}
	return a, nil
}
func id() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	send(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		fail(w, 400, "Invalid request body")
		return false
	}
	return true
}
func (a *App) internal(w http.ResponseWriter, e error) {
	slog.Error("database operation failed", "error", e)
	fail(w, 500, "Unable to complete operation")
}

// The proxy must preserve Host. Forwarded host/proto headers are deliberately
// ignored; a validated browser Origin supplies the public scheme after TLS termination.
func sameHostOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	u, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") &&
		u.Host != "" && strings.EqualFold(u.Host, r.Host) && u.User == nil &&
		u.Path == "" && !u.ForceQuery && u.RawQuery == "" && u.Fragment == "" &&
		(r.TLS == nil || u.Scheme == "https")
}
func secureRequest(r *http.Request) bool {
	return r.TLS != nil || (sameHostOrigin(r) && strings.HasPrefix(r.Header.Get("Origin"), "https://"))
}

func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/healthz" {
			send(w, 200, map[string]string{"status": "ok"})
			return
		}
		if r.URL.Path == "/readyz" {
			if a.DB.PingContext(r.Context()) != nil {
				fail(w, 503, "Database unavailable")
			} else {
				send(w, 200, map[string]string{"status": "ready"})
			}
			return
		}
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			a.deletionBearer(w, r)
			return
		}
		if r.Method != "GET" && !sameHostOrigin(r) {
			fail(w, 403, "Origin rejected")
			return
		}
		if r.URL.Path == "/api/v2/setup" {
			a.setup(w, r)
			return
		}
		if r.URL.Path == "/api/v2/session" && r.Method == "POST" {
			a.login(w, r)
			return
		}
		if r.URL.Path == "/api/v2/invitations/accept" && r.Method == "POST" {
			a.acceptInvitation(w, r)
			return
		}
		if r.URL.Path == "/api/v2/invitations/preview" && r.Method == "GET" {
			a.previewInvitation(w, r)
			return
		}
		u, csrf, e := a.authenticate(r)
		if e != nil {
			fail(w, 401, "Sign in required")
			return
		}
		if r.Method != "GET" && r.Header.Get("X-CSRF-Token") != csrf {
			fail(w, 403, "Invalid CSRF token")
			return
		}
		if a.migrationRoutes(w, r, u) {
			return
		}
		switch {
		case r.URL.Path == "/api/v2/session" && r.Method == "GET":
			send(w, 200, map[string]any{"user": u, "csrf": csrf})
		case r.URL.Path == "/api/v2/session" && r.Method == "DELETE":
			c, _ := r.Cookie("zensync_session")
			if _, e := a.DB.Exec("DELETE FROM sessions WHERE token=?", digest(c.Value)); e != nil {
				a.internal(w, e)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "zensync_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteStrictMode})
			send(w, 200, map[string]bool{"ok": true})
		case (r.URL.Path == "/api/v2/deletion-api-keys" || strings.HasPrefix(r.URL.Path, "/api/v2/deletion-api-keys/")):
			a.deletionKeyRoutes(w, r, u)
		case r.URL.Path == "/api/v2/deletion-requests" && r.Method == "POST":
			a.createDeletionRequest(w, r, u)
		case strings.HasPrefix(r.URL.Path, "/api/v2/users/") && strings.HasSuffix(r.URL.Path, "/permissions") && r.Method == "PUT":
			a.deletePermission(w, r, u)
		case r.URL.Path == "/api/v2/requests" && r.Method == "GET":
			a.listRequests(w, u)
		case r.URL.Path == "/api/v2/requests" && r.Method == "POST":
			a.createRequest(w, r, u)
		case strings.HasPrefix(r.URL.Path, "/api/v2/requests/") && r.Method == "POST":
			a.decide(w, r, u)
		case r.URL.Path == "/api/v2/activity" && r.Method == "GET":
			a.activity(w, u)
		case r.URL.Path == "/api/v2/invitations" || strings.HasPrefix(r.URL.Path, "/api/v2/invitations/"):
			a.invitations(w, r, u)
		case r.URL.Path == "/api/v2/users" && u.Role == "admin" && r.Method == "GET":
			a.users(w)
		case r.URL.Path == "/api/v2/users" && u.Role == "admin" && r.Method == "POST":
			a.createUser(w, r, u)
		case strings.HasPrefix(r.URL.Path, "/api/v2/users/") && u.Role == "admin" && r.Method == "PUT":
			a.updateUser(w, r, u)
		case r.URL.Path == "/api/v2/quality" && r.Method == "PUT":
			a.quality(w, r, u)
		default:
			fail(w, 404, "Endpoint unavailable")
		}
	})
}

func (a *App) authenticate(r *http.Request) (User, string, error) {
	var u User
	var csrf, q string
	c, e := r.Cookie("zensync_session")
	if e != nil {
		return u, "", e
	}
	e = a.DB.QueryRow("SELECT u.id,u.username,u.role,u.direct,u.quality,s.csrf,EXISTS(SELECT 1 FROM delete_permissions p WHERE p.user_id=u.id) FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token=? AND s.expires>?", digest(c.Value), time.Now().Unix()).Scan(&u.ID, &u.Username, &u.Role, &u.Direct, &q, &csrf, &u.DirectDelete)
	u.Quality = json.RawMessage(q)
	return u, csrf, e
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !a.allowLogin(w) {
		return
	}
	now := time.Now()
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	var uid string
	var hash []byte
	e := a.DB.QueryRow("SELECT id,password FROM users WHERE username=?", in.Username).Scan(&uid, &hash)
	if e != nil {
		hash = []byte("$2a$10$7EqJtq98hPqEX7fNZaFWoO5HqW05D4lFXP6VHx6J5dJAYTyj7F1sK")
	}
	check := bcrypt.CompareHashAndPassword(hash, []byte(in.Password))
	if e != nil || check != nil {
		fail(w, 401, "Invalid username or password")
		return
	}
	token, csrf := id(), id()
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("DELETE FROM sessions WHERE expires<=?", now.Unix()); e == nil {
		_, e = tx.Exec("INSERT INTO sessions VALUES(?,?,?,?)", digest(token), uid, csrf, now.Add(24*time.Hour).Unix())
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "zensync_session", Value: token, Path: "/", HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	send(w, 200, map[string]string{"csrf": csrf})
}
func audit(tx *sql.Tx, owner, actor, kind, resource string) error {
	_, e := tx.Exec("INSERT INTO events(owner,actor,kind,resource) VALUES(?,?,?,?)", owner, actor, kind, resource)
	return e
}

func (a *App) allowLogin(w http.ResponseWriter) bool {
	// Global bound also prevents spoofed forwarding headers from bypassing the limiter.
	a.mu.Lock()
	now := time.Now()
	kept := a.failures[:0]
	for _, t := range a.failures {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	a.failures = kept
	if len(kept) >= 20 {
		a.mu.Unlock()
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "Too many sign-in attempts; try again in one minute")
		return false
	}
	a.failures = append(a.failures, now)
	a.mu.Unlock()

	return true
}
