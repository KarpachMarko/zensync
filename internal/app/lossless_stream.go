package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var streamRange = regexp.MustCompile(`^bytes=([0-9]+-[0-9]*|-[0-9]+)$`)
var streamSessionID = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,64}$`)

func (p losslessProvider) streamToken(ctx context.Context) (string, error) {
	resp, err := p.request(ctx, "POST", "/api/stream/token", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Token string `json:"stream_token"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&result) != nil || result.Token == "" || len(result.Token) > 8192 || strings.ContainsAny(result.Token, "\r\n") {
		return "", errors.New("Lossless.am did not return a valid stream token")
	}
	return result.Token, nil
}
func (a *App) streamLossless(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "Method not allowed")
		return
	}
	target, err := losslessTarget(r.URL.Query().Get("target"))
	if err != nil {
		fail(w, 400, "Choose a valid Lossless.am recording")
		return
	}
	var metadata string
	if a.DB.QueryRow("SELECT metadata FROM provider_tracks WHERE target=?", target).Scan(&metadata) != nil {
		fail(w, 404, "Search for this Lossless.am recording again before streaming")
		return
	}
	rangeHeader := r.Header.Get("Range")
	if len(rangeHeader) > 100 || (rangeHeader != "" && !streamRange.MatchString(rangeHeader)) {
		fail(w, 416, "Use a single audio byte range")
		return
	}
	sid := r.URL.Query().Get("sid")
	if !streamSessionID.MatchString(sid) {
		fail(w, 400, "Invalid playback session")
		return
	}
	p := losslessProvider{a}
	if !a.losslessConfigured() {
		fail(w, 409, "Connect Lossless.am in Settings before streaming")
		return
	}
	parts := strings.Split(target, ":")
	var resp *http.Response
	// Renew a rejected stream token once. Account-cookie renewal is handled by
	// the token request; no provider credentials or token URLs reach the browser.
	for attempt := 0; attempt < 2; attempt++ {
		token, e := p.streamToken(r.Context())
		if e != nil {
			fail(w, 502, e.Error())
			return
		}
		a.losslessAuthMu.Lock()
		session := p.session()
		a.losslessAuthMu.Unlock()
		query := url.Values{"id": {parts[3]}, "source": {parts[2]}, "sid": {sid}, "_token": {token}, "_net": {"cellular"}}
		// The captured client includes _did only when it has a device identity.
		if device := session.deviceID(); device != "" {
			query.Set("_did", device)
		}
		resp, err = p.do(r.Context(), "GET", "/api/stream/track?"+query.Encode(), nil, session, http.Header{"Range": {rangeHeader}})
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		if resp.StatusCode != 401 || attempt == 1 {
			break
		}
		resp.Body.Close()
	}
	defer resp.Body.Close()
	if resp.StatusCode == 416 {
		if value := resp.Header.Get("Content-Range"); regexp.MustCompile(`^bytes \*/[0-9]+$`).MatchString(value) {
			w.Header().Set("Content-Range", value)
		}
		w.WriteHeader(416)
		return
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		fail(w, 502, losslessStatus(resp.StatusCode).Error())
		return
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch contentType {
	case "audio/mp4", "audio/x-m4a", "audio/flac", "audio/x-flac", "audio/mpeg", "audio/aac", "audio/ogg", "audio/wav":
	default:
		fail(w, 502, "Lossless.am did not return playable audio")
		return
	}
	if err != nil || resp.ContentLength > 1<<30 {
		fail(w, 502, "Lossless.am stream exceeds the audio size limit")
		return
	}
	if resp.StatusCode == 206 && !regexp.MustCompile(`^bytes [0-9]+-[0-9]+/[0-9]+$`).MatchString(resp.Header.Get("Content-Range")) {
		fail(w, 502, "Invalid Lossless.am audio range response")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Accept-Ranges", "bytes")
	if resp.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	if resp.StatusCode == 206 {
		w.Header().Set("Content-Range", resp.Header.Get("Content-Range"))
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 1<<30))
}
