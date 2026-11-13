package app

import (
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var audioContentRange = regexp.MustCompile(`^bytes [0-9]+-[0-9]+/[0-9]+$`)
var audioUnsatisfiedRange = regexp.MustCompile(`^bytes \*/[0-9]+$`)

// Listening never creates an acquisition or writes audio into the library.
func (a *App) streamHitmo(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "Method not allowed")
		return
	}
	target, err := hitmoTarget(r.URL.Query().Get("target"))
	if err != nil {
		fail(w, 400, "Choose a valid Hitmo recording")
		return
	}
	byteRange := r.Header.Get("Range")
	if len(byteRange) > 100 || (byteRange != "" && !streamRange.MatchString(byteRange)) {
		fail(w, 416, "Use a single audio byte range")
		return
	}
	if !streamSessionID.MatchString(r.URL.Query().Get("sid")) {
		fail(w, 400, "Invalid playback session")
		return
	}
	var metadata string
	if a.DB.QueryRowContext(r.Context(), "SELECT metadata FROM provider_tracks WHERE target=?", target).Scan(&metadata) != nil {
		fail(w, 404, "Search for this Hitmo recording again before streaming")
		return
	}
	p := hitmoProvider{a}
	recording, err := p.recording(r.Context(), target)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	resp, err := p.fetch(r.Context(), strings.TrimPrefix(recording.stream, hitmoOrigin), byteRange, true)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 416 {
		if value := resp.Header.Get("Content-Range"); audioUnsatisfiedRange.MatchString(value) {
			w.Header().Set("Content-Range", value)
		}
		w.WriteHeader(416)
		return
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "audio/mpeg" {
		fail(w, 502, "Hitmo did not return playable MP3 audio")
		return
	}
	if resp.ContentLength > 1<<30 {
		fail(w, 502, "Hitmo stream exceeds the audio size limit")
		return
	}
	if resp.StatusCode == 206 && !audioContentRange.MatchString(resp.Header.Get("Content-Range")) {
		fail(w, 502, "Invalid Hitmo audio range response")
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
