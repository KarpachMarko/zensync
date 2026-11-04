package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" // OpenSubsonic's password+salt authentication contract.
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) initRuntime(dbPath string) error {
	dir, e := filepath.Abs(filepath.Dir(dbPath))
	if e != nil {
		return e
	}
	a.dataDir = dir
	a.musicDir = os.Getenv("ZENSYNC_MUSIC_DIR")
	if a.musicDir == "" {
		a.musicDir = "/music"
	}
	a.dfiPath = os.Getenv("ZENSYNC_DFI_PATH")
	if a.dfiPath == "" {
		a.dfiPath = "/usr/local/bin/d-fi"
	}
	a.ffprobePath = os.Getenv("ZENSYNC_FFPROBE_PATH")
	if a.ffprobePath == "" {
		a.ffprobePath = "ffprobe"
	}
	path := filepath.Join(dir, "credentials.key")
	key, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		var count int
		if e = a.DB.QueryRow("SELECT count(*) FROM secrets").Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return errors.New("credentials.key is missing; restore it from backup")
		}
		key = make([]byte, 32)
		if _, e = rand.Read(key); e != nil {
			return e
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, e = f.Write(key)
		if e == nil {
			e = f.Sync()
		}
		f.Close()
	}
	if e != nil {
		return e
	}
	if len(key) != 32 {
		return errors.New("invalid credential encryption key")
	}
	a.key = key
	return nil
}
func (a *App) seal(name string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	block, e := aes.NewCipher(a.key)
	if e != nil {
		return e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	sealed := g.Seal(nonce, nonce, b, []byte(name))
	_, e = a.DB.Exec("INSERT INTO secrets(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value", name, sealed)
	return e
}
func (a *App) unseal(name string, v any) error {
	var data []byte
	if e := a.DB.QueryRow("SELECT value FROM secrets WHERE name=?", name).Scan(&data); e != nil {
		return errors.New("connection is not configured")
	}
	block, _ := aes.NewCipher(a.key)
	g, _ := cipher.NewGCM(block)
	if len(data) < g.NonceSize() {
		return errors.New("invalid stored credential")
	}
	plain, e := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte(name))
	if e != nil {
		return errors.New("cannot decrypt stored credential")
	}
	return json.Unmarshal(plain, v)
}

type Credentials struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}
type Song struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Album       string  `json:"album"`
	Duration    float64 `json:"duration"`
	Path        string  `json:"path"`
	Starred     string  `json:"starred"`
	ContentType string  `json:"contentType"`
	BitRate     int     `json:"bitRate"`
	Size        int64   `json:"size"`
}
type RemotePlaylist struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	Comment string `json:"comment"`
	Public  bool   `json:"public"`
	Entries []Song `json:"entry"`
}
type NavResponse struct {
	Type          string `json:"type"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	ServerVersion string `json:"serverVersion"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	User struct {
		Username  string `json:"username"`
		AdminRole bool   `json:"adminRole"`
	} `json:"user"`
	Search struct {
		Songs []Song `json:"song"`
	} `json:"searchResult3"`
	Song      Song           `json:"song"`
	Playlist  RemotePlaylist `json:"playlist"`
	Playlists struct {
		Items []RemotePlaylist `json:"playlist"`
	} `json:"playlists"`
	Scan struct {
		Scanning bool  `json:"scanning"`
		Count    int64 `json:"count"`
	} `json:"scanStatus"`
}

func (a *App) navRaw(ctx context.Context, c Credentials, endpoint string, params url.Values) (*http.Response, error) {
	if params == nil {
		params = url.Values{}
	}
	salt := id()[:16]
	h := md5.Sum([]byte(c.Password + salt))
	params.Set("u", c.Username)
	params.Set("s", salt)
	params.Set("t", hex.EncodeToString(h[:]))
	params.Set("v", "1.16.1")
	params.Set("c", "ZenSync")
	params.Set("f", "json")
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.URL, "/")+"/rest/"+endpoint+".view", strings.NewReader(params.Encode()))
	if e != nil {
		return nil, errors.New("invalid Navidrome URL")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := a.HTTP
	if endpoint == "download" {
		copy := *a.HTTP
		copy.Timeout = 3 * time.Minute
		client = &copy
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("Navidrome connection failed; check URL and connectivity")
	}
	return resp, nil
}
func (a *App) nav(ctx context.Context, c Credentials, endpoint string, params url.Values) (NavResponse, error) {
	var out NavResponse
	resp, e := a.navRaw(ctx, c, endpoint, params)
	if e != nil {
		return out, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("Navidrome returned HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Response NavResponse `json:"subsonic-response"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&envelope); e != nil {
		return out, errors.New("invalid Navidrome response")
	}
	out = envelope.Response
	if out.Status != "ok" {
		code := 0
		if out.Error != nil {
			code = out.Error.Code
		}
		return out, fmt.Errorf("Navidrome rejected %s (code %d); verify account permissions", endpoint, code)
	}
	return out, nil
}
func (a *App) shared() (Credentials, error) {
	var c Credentials
	e := a.unseal("navidrome", &c)
	return c, e
}
func (a *App) personal(owner string) (Credentials, error) {
	shared, e := a.shared()
	if e != nil {
		return shared, e
	}
	var c Credentials
	if e = a.unseal("navidrome:"+owner, &c); e != nil {
		return c, errors.New("link your personal Navidrome account in Settings first")
	}
	c.URL = shared.URL
	return c, nil
}
func (a *App) connectionRoutes(w http.ResponseWriter, r *http.Request, u User) {
	if r.Method == "GET" {
		var shared Credentials
		_ = a.unseal("navidrome", &shared)
		var own Credentials
		_ = a.unseal("navidrome:"+u.ID, &own)
		var arl, losslessCookie string
		_ = a.unseal("deezer", &arl)
		_ = a.unseal("lossless", &losslessCookie)
		session := (losslessProvider{a}).session()
		losslessEmail := ""
		if u.Role == "admin" {
			losslessEmail = session.Email
		}
		_, binaryErr := os.Stat(a.dfiPath)
		_, musicErr := os.Stat(a.musicDir)
		send(w, 200, map[string]any{"url": shared.URL, "serviceUsername": shared.Username, "configured": shared.Username != "", "servicePasswordConfigured": shared.Password != "", "personalPasswordConfigured": own.Password != "", "losslessEmail": losslessEmail, "personalUsername": own.Username, "deezerConfigured": arl != "", "losslessConfigured": losslessCookie != "" || session.has("jwt_refresh"), "downloaderReady": binaryErr == nil, "musicMounted": musicErr == nil, "downloaderVersion": "2.3.5"})
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "Method not allowed")
		return
	}
	var in struct {
		Kind     string `json:"kind"`
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		ARL      string `json:"arl"`
		Email    string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.opMu.Lock()
	defer a.opMu.Unlock()
	if in.Kind == "lossless-login" {
		if u.Role != "admin" {
			fail(w, 403, "Administrator required")
			return
		}
		if e := (losslessProvider{a}).login(r.Context(), in.Email, in.Password); e != nil {
			fail(w, 400, e.Error())
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if in.Kind == "lossless" {
		if u.Role != "admin" {
			fail(w, 403, "Administrator required")
			return
		}
		cookie := strings.TrimSpace(in.ARL)
		if len(cookie) < 3 || len(cookie) > 4096 || !strings.Contains(cookie, "=") || strings.ContainsAny(cookie, "\r\n") {
			fail(w, 400, "Enter a complete Lossless.am session Cookie header")
			return
		}
		a.losslessAuthMu.Lock()
		defer a.losslessAuthMu.Unlock()
		if e := a.seal("lossless", cookie); e != nil {
			a.internal(w, e)
			return
		}
		if _, e := a.DB.Exec("DELETE FROM secrets WHERE name=?", losslessSessionKey); e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if in.Kind == "deezer" {
		if u.Role != "admin" {
			fail(w, 403, "Administrator required")
			return
		}
		if len(in.ARL) < 32 || len(in.ARL) > 4096 {
			fail(w, 400, "Enter your Deezer ARL cookie")
			return
		}
		if e := a.seal("deezer", in.ARL); e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	c := Credentials{URL: strings.TrimRight(strings.TrimSpace(in.URL), "/"), Username: strings.TrimSpace(in.Username), Password: in.Password}
	key := "navidrome:" + u.ID
	if in.Kind == "shared" {
		if u.Role != "admin" {
			fail(w, 403, "Administrator required")
			return
		}
		key = "navidrome"
		parsed, e := url.Parse(c.URL)
		if e != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			fail(w, 400, "Enter an http(s) Navidrome base URL")
			return
		}
		old, _ := a.shared()
		if old.URL != "" && old.URL != c.URL {
			var n int
			a.DB.QueryRow("SELECT count(*) FROM migrations").Scan(&n)
			if n > 0 {
				fail(w, 409, "The server cannot change after migrations exist")
				return
			}
		}
	} else if in.Kind == "personal" {
		shared, e := a.shared()
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		c.URL = shared.URL
		var n int
		if e = a.DB.QueryRow("SELECT count(*) FROM migrations WHERE owner=? AND target_user<>'' AND target_user<>?", u.ID, c.Username).Scan(&n); e != nil {
			a.internal(w, e)
			return
		}
		if n > 0 {
			fail(w, 409, "Existing migrations are bound to another personal account")
			return
		}
	} else {
		fail(w, 400, "Unknown connection type")
		return
	}
	if c.Username == "" || c.Password == "" || len(c.Username) > 200 || len(c.Password) > 4096 {
		fail(w, 400, "Username and password required")
		return
	}
	verified, e := a.nav(r.Context(), c, "getUser", url.Values{"username": {c.Username}})
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	if verified.User.Username != c.Username {
		fail(w, 400, "Navidrome returned a different account identity")
		return
	}
	if e = a.seal(key, c); e != nil {
		a.internal(w, e)
		return
	}
	send(w, 200, map[string]any{"ok": true, "version": verified.ServerVersion})
}
func (a *App) navSearch(ctx context.Context, c Credentials, q string, offset int, rawCount ...*int) ([]Song, error) {
	out, e := a.nav(ctx, c, "search3", url.Values{"query": {q}, "songCount": {"100"}, "songOffset": {strconv.Itoa(offset)}, "albumCount": {"0"}, "artistCount": {"0"}})
	if e != nil {
		return nil, e
	}
	if len(rawCount) > 0 {
		*rawCount[0] = len(out.Search.Songs)
	}
	if out.Search.Songs == nil {
		return []Song{}, nil
	}
	rows, e := a.DB.Query("SELECT DISTINCT song_id FROM song_deletions WHERE phase='removed'")
	if e != nil {
		return nil, e
	}
	removed := map[string]bool{}
	for rows.Next() {
		var song string
		if e = rows.Scan(&song); e != nil {
			rows.Close()
			return nil, e
		}
		removed[song] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	songs := []Song{}
	for _, song := range out.Search.Songs {
		if !removed[song.ID] {
			songs = append(songs, song)
		}
	}
	return songs, nil
}
func (a *App) library(w http.ResponseWriter, r *http.Request) {
	c, e := a.shared()
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	var count int
	songs, e := a.navSearch(r.Context(), c, r.URL.Query().Get("q"), offset, &count)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	if r.URL.Query().Get("details") == "1" {
		next := -1
		if count == 100 {
			next = offset + 100
		}
		send(w, 200, map[string]any{"songs": songs, "nextOffset": next})
		return
	}
	send(w, 200, songs)
}
