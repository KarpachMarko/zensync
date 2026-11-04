package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"zensync/internal/apple"
)

// Reuse the import recording-review operations for failed standalone requests
// too. A private single-song workspace gives them the same explicit matching,
// approval and upload rules without navigating away from Activity.
func (a *App) recordingReview(w http.ResponseWriter, r *http.Request, u User) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v2/"), "/")
	if len(parts) != 3 {
		fail(w, 404, "Request not found")
		return
	}
	where := "j.id=?"
	if parts[0] == "requests" {
		where = "r.id=?"
	}
	var rid, owner, state, target, title, artist, metadata string
	err := a.DB.QueryRow(`SELECT r.id,r.owner,j.state,r.target,r.title,r.artist,COALESCE(rm.metadata,'') FROM jobs j JOIN requests r ON r.id=j.request_id LEFT JOIN request_metadata rm ON rm.request_id=r.id WHERE `+where, parts[1]).Scan(&rid, &owner, &state, &target, &title, &artist, &metadata)
	if err != nil || (owner != u.ID && u.Role != "admin") || !targetPattern.MatchString(target) {
		fail(w, 404, "Request not found")
		return
	}
	if state != "failed" && state != "blocked" {
		fail(w, 409, "Only failed or blocked acquisitions can be reviewed")
		return
	}
	a.opMu.Lock()
	defer a.opMu.Unlock()
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	if err = tx.QueryRow("SELECT state FROM jobs WHERE request_id=?", rid).Scan(&state); err != nil || (state != "failed" && state != "blocked") {
		fail(w, 409, "The acquisition is no longer failed or blocked")
		return
	}
	var mid, sid string
	err = tx.QueryRow(`SELECT s.migration_id,s.id FROM request_sources rs JOIN source_songs s ON s.id=rs.source_id JOIN migrations m ON m.id=s.migration_id WHERE rs.request_id=? AND m.owner=? AND m.state='open' ORDER BY s.id LIMIT 1`, rid, u.ID).Scan(&mid, &sid)
	if err == sql.ErrNoRows {
		fingerprint := digest("request-review:" + u.ID + ":" + rid)
		err = tx.QueryRow("SELECT m.id,s.id FROM migrations m JOIN source_songs s ON s.migration_id=m.id WHERE m.owner=? AND m.fingerprint=? AND s.source_id=?", u.ID, fingerprint, rid).Scan(&mid, &sid)
		if err == sql.ErrNoRows {
			mid, sid = id(), id()
			var track Track
			_ = json.Unmarshal([]byte(metadata), &track)
			source := apple.Song{SourceID: rid, Title: title, Artist: artist, Album: track.Album.Title, Duration: track.Duration}
			candidate := ""
			if track.ID > 0 {
				candidate = mustJSON(track)
			}
			_, err = tx.Exec("INSERT INTO migrations(id,owner,fingerprint,name,state,source,report) VALUES(?,?,?,?,'open','request-review',?)", mid, u.ID, fingerprint, title, mustJSON(map[string]any{"songs": 1, "playlists": 0, "unplaylisted": 1, "warnings": []string{}}))
			if err == nil {
				_, err = tx.Exec("INSERT INTO source_songs(id,migration_id,source_id,metadata,candidate,match_state) VALUES(?,?,?,?,?,'manual')", sid, mid, rid, mustJSON(source), candidate)
			}
			if err == nil {
				_, err = tx.Exec("INSERT INTO request_sources(source_id,request_id) VALUES(?,?)", sid, rid)
			}
		}
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		a.internal(w, err)
		return
	}
	migration, err := a.getMigration(mid, u.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	sources, err := a.sources(mid, sid)
	if err != nil {
		a.internal(w, err)
		return
	}
	if len(sources) != 1 {
		fail(w, 404, "Recording review no longer exists")
		return
	}
	send(w, 200, map[string]any{"migration": migration, "source": sources[0]})
}
