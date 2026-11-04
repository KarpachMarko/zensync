package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

const losslessSessionKey = "lossless.session"

type losslessSession struct {
	Email   string         `json:"email"`
	Cookies []*http.Cookie `json:"cookies"`
}

var losslessOrigin = &url.URL{Scheme: "https", Host: "lossless.am", Path: "/api/auth/"}

// Refresh and credential replacement share a lock so rotated cookies cannot
// overwrite a newer sign-in, including when several acquisitions receive 401.
func (p losslessProvider) session() losslessSession {
	var session losslessSession
	if p.app.unseal(losslessSessionKey, &session) == nil {
		return session
	}
	var legacy string
	if p.app.unseal("lossless", &legacy) == nil {
		request := &http.Request{Header: make(http.Header)}
		request.Header.Set("Cookie", legacy)
		for _, c := range request.Cookies() {
			c.Path = "/"
			c.Secure = true
			if c.Name == "jwt_refresh" || c.Name == "auth_scope" {
				c.Path = "/api/auth"
			}
			session.Cookies = append(session.Cookies, c)
		}
	}
	return session
}
func (s *losslessSession) merge(cookies []*http.Cookie) {
	for _, cookie := range cookies {
		switch cookie.Name {
		case "jwt_access", "jwt_refresh", "has_session", "auth_scope":
		default:
			continue
		}
		if cookie.Domain != "" && strings.TrimPrefix(cookie.Domain, ".") != "lossless.am" {
			continue
		}
		c := *cookie
		c.Domain = ""
		c.Secure = true
		if c.Path == "" {
			c.Path = "/api/auth"
		}
		if c.MaxAge > 0 {
			c.Expires = time.Now().Add(time.Duration(c.MaxAge) * time.Second)
		}
		keep := s.Cookies[:0]
		for _, old := range s.Cookies {
			if old.Name != c.Name || old.Path != c.Path {
				keep = append(keep, old)
			}
		}
		s.Cookies = keep
		if c.MaxAge >= 0 && (c.Expires.IsZero() || c.Expires.After(time.Now())) {
			s.Cookies = append(s.Cookies, &c)
		}
	}
}
func (s losslessSession) has(name string) bool {
	for _, c := range s.Cookies {
		if c.Name == name && c.Value != "" && (c.Expires.IsZero() || c.Expires.After(time.Now())) {
			return true
		}
	}
	return false
}
func (p losslessProvider) do(ctx context.Context, method, path string, body []byte, s losslessSession) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "https://lossless.am"+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("Invalid Lossless.am request")
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	jar, _ := cookiejar.New(nil)
	for _, stored := range s.Cookies {
		cookie := *stored
		cookie.MaxAge = 0 // Expires was fixed when received; never extend it on reload.
		jar.SetCookies(losslessOrigin, []*http.Cookie{&cookie})
	}
	for _, c := range jar.Cookies(req.URL) {
		req.AddCookie(c)
	}
	client := *p.app.HTTP
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.Timeout = 30 * time.Second
	if strings.HasPrefix(path, "/api/download/") {
		client.Timeout = 15 * time.Minute
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("Lossless.am connection failed")
	}
	return resp, nil
}
func losslessStatus(status int) error {
	if status == 401 {
		return errors.New("Lossless.am session expired or was revoked; sign in again in Settings")
	}
	return fmt.Errorf("Lossless.am returned HTTP %d; check service availability and your session in Settings", status)
}
func (p losslessProvider) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = io.ReadAll(io.LimitReader(body, 16385))
		if err != nil || len(payload) > 16384 {
			return nil, errors.New("Invalid Lossless.am request body")
		}
	}
	p.app.losslessAuthMu.Lock()
	defer p.app.losslessAuthMu.Unlock()
	session := p.session()
	resp, err := p.do(ctx, method, path, payload, session)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 && session.has("jwt_refresh") {
		resp.Body.Close()
		// The captured client sends device_id when available. Older login sessions
		// omit it, while device-bound JWTs carry it as did. This is a routing hint,
		// never a locally trusted authentication assertion.
		refreshBody, _ := json.Marshal(map[string]string{})
		if device := session.deviceID(); device != "" {
			refreshBody, _ = json.Marshal(map[string]string{"device_id": device})
		}
		refreshed, e := p.do(ctx, "POST", "/api/auth/refresh", refreshBody, session)
		if e != nil {
			return nil, e
		}
		refreshed.Body.Close()
		if refreshed.StatusCode != 200 {
			return nil, losslessStatus(refreshed.StatusCode)
		}
		hasAccess := false
		for _, c := range refreshed.Cookies() {
			if c.Name == "jwt_access" && c.Value != "" && c.MaxAge >= 0 {
				hasAccess = true
			}
		}
		if !hasAccess {
			return nil, errors.New("Lossless.am refresh did not return an access cookie; sign in again in Settings")
		}
		session.merge(refreshed.Cookies())
		if !session.has("jwt_access") {
			return nil, errors.New("Lossless.am refresh did not return an access cookie; sign in again in Settings")
		}
		if e = p.app.seal(losslessSessionKey, session); e != nil {
			return nil, errors.New("Could not save renewed Lossless.am session")
		}
		resp, err = p.do(ctx, method, path, payload, session)
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, losslessStatus(resp.StatusCode)
	}
	if len(resp.Cookies()) > 0 {
		session.merge(resp.Cookies())
		if err = p.app.seal(losslessSessionKey, session); err != nil {
			resp.Body.Close()
			return nil, errors.New("Could not save Lossless.am session")
		}
	}
	return resp, nil
}
func (s losslessSession) deviceID() string {
	for _, cookie := range s.Cookies {
		if cookie.Name != "jwt_refresh" {
			continue
		}
		parts := strings.Split(cookie.Value, ".")
		if len(parts) != 3 || len(parts[1]) > 8192 {
			return ""
		}
		data, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
		var claims struct {
			Device string `json:"did"`
		}
		if json.Unmarshal(data, &claims) == nil && len(claims.Device) <= 128 {
			return claims.Device
		}
	}
	return ""
}
func (p losslessProvider) login(ctx context.Context, email, password string) error {
	email = strings.TrimSpace(email)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 || len(password) < 8 || len(password) > 4096 {
		return errors.New("Enter your Lossless.am email and password (at least 8 characters)")
	}
	p.app.losslessAuthMu.Lock()
	defer p.app.losslessAuthMu.Unlock()
	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	session := losslessSession{Email: email}
	resp, err := p.do(ctx, "POST", "/api/auth/login", payload, session)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == 401 {
		return errors.New("Lossless.am rejected the email or password")
	}
	if resp.StatusCode != 200 {
		return losslessStatus(resp.StatusCode)
	}
	session.merge(resp.Cookies())
	if !session.has("jwt_access") || !session.has("jwt_refresh") {
		return errors.New("Lossless.am login did not return the required session cookies")
	}
	// Verify the new session before replacing a working connection. Passwords
	// are used only for this request and are never persisted.
	me, err := p.do(ctx, "GET", "/api/me", nil, session)
	if err != nil {
		return err
	}
	defer me.Body.Close()
	if me.StatusCode != 200 {
		return losslessStatus(me.StatusCode)
	}
	var profile struct {
		User *json.RawMessage `json:"user"`
	}
	if json.NewDecoder(io.LimitReader(me.Body, 64<<10)).Decode(&profile) != nil || profile.User == nil {
		return errors.New("Lossless.am did not verify the signed-in account")
	}
	session.merge(me.Cookies())
	if err = p.app.seal(losslessSessionKey, session); err != nil {
		return errors.New("Could not save Lossless.am session")
	}
	return nil
}
