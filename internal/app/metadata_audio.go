package app

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Audio stays behind the metadata administrator session; no Navidrome credentials
// or local music paths are sent to the browser. ServeContent supports seeking.
func (a *App) metadataAudio(w http.ResponseWriter, r *http.Request, sid string) {
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "Method not allowed")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	remote, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {sid}, "realPath": {"true"}})
	if err != nil || remote.Song.ID != sid {
		fail(w, 404, "Song is unavailable")
		return
	}
	rel, _, err := a.deletionPath(r.Context(), c, remote.Song)
	if err != nil {
		fail(w, 409, "Cannot safely locate the original audio file")
		return
	}
	if err = a.checkMusicParents(rel); err != nil {
		fail(w, 409, "Audio file is unavailable")
		return
	}
	path := filepath.Join(a.musicDir, rel)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		fail(w, 404, "Audio file is unavailable")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		fail(w, 404, "Audio file is unavailable")
		return
	}
	defer file.Close()
	types := map[string]string{".flac": "audio/flac", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".ogg": "audio/ogg", ".opus": "audio/ogg", ".wav": "audio/wav", ".aiff": "audio/aiff"}
	contentType := types[strings.ToLower(filepath.Ext(rel))]
	if contentType == "" {
		fail(w, 415, "Unsupported audio format")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "audio"+filepath.Ext(rel), info.ModTime(), file)
}
