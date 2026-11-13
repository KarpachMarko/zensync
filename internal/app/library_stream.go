package app

import (
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Navidrome credentials remain on the server. Library listening is independent
// of metadata administration and never creates acquisition or editing jobs.
func (a *App) libraryStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "Method not allowed")
		return
	}
	sid := r.URL.Query().Get("id")
	if sid == "" || len(sid) > 256 || strings.ContainsAny(sid, "\r\n\x00") {
		fail(w, 400, "Choose a valid library recording")
		return
	}
	byteRange := r.Header.Get("Range")
	if len(byteRange) > 100 || (byteRange != "" && !streamRange.MatchString(byteRange)) {
		fail(w, 416, "Use a single audio byte range")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	song, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {sid}})
	if err != nil || song.Song.ID != sid {
		fail(w, 404, "Library recording is unavailable")
		return
	}
	resp, err := a.navRawHeaders(r.Context(), c, "stream", url.Values{"id": {sid}, "maxBitRate": {"128"}, "format": {"mp3"}}, http.Header{"Range": {byteRange}})
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 416 {
		if v := resp.Header.Get("Content-Range"); audioUnsatisfiedRange.MatchString(v) {
			w.Header().Set("Content-Range", v)
		}
		w.WriteHeader(416)
		return
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		fail(w, 502, "Navidrome streaming is unavailable")
		return
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(contentType, "audio/") || resp.ContentLength > 1<<30 {
		fail(w, 502, "Navidrome did not return playable audio")
		return
	}
	if resp.StatusCode == 206 && !audioContentRange.MatchString(resp.Header.Get("Content-Range")) {
		fail(w, 502, "Invalid library audio range response")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Accept-Ranges", "bytes")
	if resp.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	if resp.StatusCode == 206 {
		w.Header().Set("Content-Range", resp.Header.Get("Content-Range"))
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != "HEAD" {
		_, _ = io.Copy(w, io.LimitReader(resp.Body, 1<<30))
	}
}
func (a *App) libraryCover(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "Method not allowed")
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" || len(id) > 256 || strings.ContainsAny(id, "\r\n\x00") {
		fail(w, 400, "Choose valid library artwork")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	resp, err := a.navRaw(r.Context(), c, "getCoverArt", url.Values{"id": {id}, "size": {"300"}})
	if err != nil {
		fail(w, 502, "Library artwork is unavailable")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(w, 404, "Library artwork is unavailable")
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		fail(w, 502, "Library artwork exceeds the size limit")
		return
	}
	contentType := http.DetectContentType(body)
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" && contentType != "image/gif" {
		fail(w, 502, "Navidrome did not return an image")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(body)
}
