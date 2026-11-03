package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"zensync/internal/apple"
)

type Migration struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	State      string          `json:"state"`
	Report     json.RawMessage `json:"report"`
	Favorites  bool            `json:"favorites"`
	TargetUser string          `json:"targetUser"`
	Error      string          `json:"error"`
	Created    string          `json:"created"`
}
type Source struct {
	ID            string     `json:"id"`
	Metadata      apple.Song `json:"metadata"`
	ServerID      string     `json:"serverId"`
	Candidate     *Track     `json:"candidate"`
	MatchState    string     `json:"matchState"`
	Deferred      bool       `json:"deferred"`
	FavoriteState string     `json:"favoriteState"`
	RequestID     string     `json:"requestId"`
	Decision      string     `json:"decision"`
	JobState      string     `json:"jobState"`
}
type Playlist struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Metadata apple.Playlist `json:"metadata"`
	Entries  []string       `json:"entries"`
	Selected bool           `json:"selected"`
	State    string         `json:"state"`
	RemoteID string         `json:"remoteId"`
	Baseline string         `json:"baseline"`
	Desired  string         `json:"desired"`
	Error    string         `json:"error"`
}

func (a *App) migrationRoutes(w http.ResponseWriter, r *http.Request, u User) bool {
	path := r.URL.Path
	switch {
	case path == "/api/v2/settings/downloads":
		a.downloadSettings(w, r, u)
	case path == "/api/v2/connections":
		a.connectionRoutes(w, r, u)
	case path == "/api/v2/search" && r.Method == "GET":
		a.search(w, r)
	case path == "/api/v2/library" && r.Method == "GET":
		a.library(w, r)
	case path == "/api/v2/imports" && r.Method == "GET":
		a.listMigrations(w, u)
	case path == "/api/v2/imports" && r.Method == "POST":
		a.upload(w, r, u)
	case strings.HasPrefix(path, "/api/v2/imports/"):
		a.migrationRoute(w, r, u)
	case strings.HasPrefix(path, "/api/v2/tasks/") && r.Method == "POST":
		if strings.HasSuffix(path, "/cancel") {
			a.cancelMigrationTask(w, r, u)
		} else {
			a.retryTask(w, r, u)
		}
	default:
		return false
	}
	return true
}
func (a *App) listMigrations(w http.ResponseWriter, u User) {
	rows, e := a.DB.Query("SELECT id,name,state,report,favorites,target_user,error,created FROM migrations WHERE owner=? ORDER BY created DESC", u.ID)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	out := []Migration{}
	for rows.Next() {
		var m Migration
		var report string
		if e = rows.Scan(&m.ID, &m.Name, &m.State, &report, &m.Favorites, &m.TargetUser, &m.Error, &m.Created); e != nil {
			a.internal(w, e)
			return
		}
		m.Report = json.RawMessage(report)
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		a.internal(w, e)
		return
	}
	send(w, 200, out)
}
func (a *App) upload(w http.ResponseWriter, r *http.Request, u User) {
	if !a.importMu.TryLock() {
		fail(w, 429, "Another export is being parsed; retry shortly")
		return
	}
	defer a.importMu.Unlock()

	r.Body = http.MaxBytesReader(w, r.Body, apple.MaxBytes)
	data, e := io.ReadAll(r.Body)
	if e != nil {
		fail(w, 413, "Export exceeds 32 MiB or could not be read")
		return
	}
	name := strings.TrimSpace(r.Header.Get("X-Import-Name"))
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}
	snapshot, e := apple.ParseFile(data, name)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	if name == "" {
		name = "Apple Music migration"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	fingerprint := digest(string(data))
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var existing string
	e = tx.QueryRow("SELECT id FROM migrations WHERE owner=? AND fingerprint=?", u.ID, fingerprint).Scan(&existing)
	if e == nil {
		send(w, 200, map[string]string{"id": existing})
		return
	}
	if e != sql.ErrNoRows {
		a.internal(w, e)
		return
	}
	mid := id()
	report := map[string]any{"songs": len(snapshot.Songs), "playlists": len(snapshot.Playlists), "unplaylisted": snapshot.Unplaylisted, "missing": snapshot.Missing, "excluded": snapshot.Excluded, "warnings": snapshot.Warnings, "libraryId": snapshot.LibraryID}
	if _, e = tx.Exec("INSERT INTO migrations(id,owner,fingerprint,name,source,report) VALUES(?,?,?,?,?,?)", mid, u.ID, fingerprint, name, string(data), apple.JSON(report)); e != nil {
		a.internal(w, e)
		return
	}
	sourceIDs := map[string]string{}
	for _, s := range snapshot.Songs {
		sid := id()
		sourceIDs[s.SourceID] = sid
		if _, e = tx.Exec("INSERT INTO source_songs(id,migration_id,source_id,metadata) VALUES(?,?,?,?)", sid, mid, s.SourceID, apple.JSON(s)); e != nil {
			a.internal(w, e)
			return
		}
	}
	for _, p := range snapshot.Playlists {
		entries := []string{}
		for _, ref := range p.Entries {
			entries = append(entries, sourceIDs[ref])
		}
		if _, e = tx.Exec("INSERT INTO migration_playlists(id,migration_id,name,metadata,entries,selected) VALUES(?,?,?,?,?,?)", id(), mid, p.Name, apple.JSON(p), apple.JSON(entries), !p.Folder && !p.System); e != nil {
			a.internal(w, e)
			return
		}
	}
	if e = audit(tx, u.ID, u.ID, "migration.staged", mid); e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 201, map[string]string{"id": mid})
}
func (a *App) getMigration(mid, owner string) (Migration, error) {
	var m Migration
	var report string
	e := a.DB.QueryRow("SELECT id,name,state,report,favorites,target_user,error,created FROM migrations WHERE id=? AND owner=?", mid, owner).Scan(&m.ID, &m.Name, &m.State, &report, &m.Favorites, &m.TargetUser, &m.Error, &m.Created)
	m.Report = json.RawMessage(report)
	return m, e
}
func (a *App) sources(mid string, sourceIDs ...string) ([]Source, error) {
	query := `SELECT s.id,s.metadata,s.server_id,s.candidate,s.match_state,s.deferred,s.favorite_state,COALESCE(r.id,''),COALESCE(r.decision,''),COALESCE(j.state,'') FROM source_songs s LEFT JOIN requests r ON r.id=(SELECT rs.request_id FROM request_sources rs JOIN requests rq ON rq.id=rs.request_id WHERE rs.source_id=s.id ORDER BY rq.created DESC,rq.rowid DESC LIMIT 1) LEFT JOIN jobs j ON j.request_id=r.id WHERE s.migration_id=?`
	args := []any{mid}
	if len(sourceIDs) > 0 {
		query += " AND s.id=?"
		args = append(args, sourceIDs[0])
	}
	rows, e := a.DB.Query(query+" ORDER BY s.rowid", args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var s Source
		var data, candidate string
		if e = rows.Scan(&s.ID, &data, &s.ServerID, &candidate, &s.MatchState, &s.Deferred, &s.FavoriteState, &s.RequestID, &s.Decision, &s.JobState); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(data), &s.Metadata); e != nil {
			return nil, e
		}
		if candidate != "" {
			if e = json.Unmarshal([]byte(candidate), &s.Candidate); e != nil {
				return nil, e
			}
		}
		s.Metadata.Original = nil
		out = append(out, s)
	}
	return out, rows.Err()
}
func (a *App) playlists(mid string) ([]Playlist, error) {
	rows, e := a.DB.Query("SELECT id,name,metadata,entries,selected,state,remote_id,baseline,desired,error FROM migration_playlists WHERE migration_id=? ORDER BY rowid", mid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Playlist{}
	for rows.Next() {
		var p Playlist
		var metadata, entries string
		if e = rows.Scan(&p.ID, &p.Name, &metadata, &entries, &p.Selected, &p.State, &p.RemoteID, &p.Baseline, &p.Desired, &p.Error); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(metadata), &p.Metadata); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(entries), &p.Entries); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (a *App) migrationRoute(w http.ResponseWriter, r *http.Request, u User) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v2/imports/"), "/")
	mid := parts[0]
	m, e := a.getMigration(mid, u.ID)
	if e != nil {
		fail(w, 404, "Migration not found")
		return
	}
	if len(parts) == 1 && r.Method == "DELETE" {
		a.deleteMigration(w, u, mid)
		return
	}
	if len(parts) == 1 && r.Method == "GET" {
		songs, e := a.sources(mid)
		if e != nil {
			a.internal(w, e)
			return
		}
		pls, e := a.playlists(mid)
		if e != nil {
			a.internal(w, e)
			return
		}
		var matchTask struct {
			State   string `json:"state"`
			Message string `json:"message"`
		}
		e = a.DB.QueryRow("SELECT state,message FROM migration_tasks WHERE migration_id=? AND action='match' ORDER BY created DESC,rowid DESC LIMIT 1", mid).Scan(&matchTask.State, &matchTask.Message)
		if e != nil && e != sql.ErrNoRows {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]any{"migration": m, "songs": songs, "playlists": pls, "matchTask": matchTask})
		return
	}
	if r.Method != "POST" || len(parts) < 2 {
		fail(w, 404, "Endpoint unavailable")
		return
	}
	if parts[1] == "songs" && len(parts) == 4 {
		if parts[3] == "upload" {
			a.uploadSourceAudio(w, r, u, mid, parts[2])
		} else {
			a.sourceAction(w, r, u, m, parts[2], parts[3])
		}
		return
	}
	a.opMu.Lock()
	defer a.opMu.Unlock()
	m, e = a.getMigration(mid, u.ID)
	if e != nil {
		fail(w, 404, "Migration not found")
		return
	}
	switch parts[1] {
	case "commit":
		if m.State != "staged" {
			send(w, 200, map[string]bool{"ok": true})
			return
		}
		var in struct {
			Playlists []string `json:"playlists"`
			Favorites bool     `json:"favorites"`
		}
		if !decode(w, r, &in) {
			return
		}
		tx, e := a.DB.Begin()
		if e != nil {
			a.internal(w, e)
			return
		}
		defer tx.Rollback()
		if _, e = tx.Exec("UPDATE migration_playlists SET selected=0 WHERE migration_id=?", mid); e != nil {
			a.internal(w, e)
			return
		}
		for _, pid := range in.Playlists {
			var meta string
			if e = tx.QueryRow("SELECT metadata FROM migration_playlists WHERE id=? AND migration_id=?", pid, mid).Scan(&meta); e != nil {
				fail(w, 400, "Playlist does not belong to this migration")
				return
			}
			var p apple.Playlist
			json.Unmarshal([]byte(meta), &p)
			if p.Folder || p.System {
				fail(w, 400, "Folders and system playlists cannot be published")
				return
			}
			if _, e = tx.Exec("UPDATE migration_playlists SET selected=1 WHERE id=?", pid); e != nil {
				a.internal(w, e)
				return
			}
		}
		_, e = tx.Exec("UPDATE migrations SET state='open',favorites=? WHERE id=?", in.Favorites, mid)
		if e == nil {
			// Commit and the initial scan are atomic: retries cannot leave an
			// open migration without its first matching task.
			_, e = tx.Exec("INSERT INTO migration_tasks(id,owner,migration_id,action) VALUES(?,?,?,'match')", id(), u.ID, mid)
		}
		if e == nil {
			e = audit(tx, u.ID, u.ID, "migration.committed", mid)
		}
		if e == nil {
			e = tx.Commit()
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	case "match", "publish":
		if m.State != "open" {
			fail(w, 409, "Only open migrations can be processed")
			return
		}
		if parts[1] == "publish" {
			c, err := a.personal(u.ID)
			if err != nil {
				fail(w, 400, err.Error())
				return
			}
			if m.TargetUser != "" && m.TargetUser != c.Username {
				fail(w, 409, "Personal account changed")
				return
			}
			if _, e = a.DB.Exec("UPDATE migrations SET target_user=? WHERE id=?", c.Username, mid); e != nil {
				a.internal(w, e)
				return
			}
		}
		task, e := a.queueMigration(u.ID, mid, parts[1])
		if e != nil {
			a.internal(w, e)
			return
		}
		send(w, 202, map[string]string{"id": task})
	case "handover":
		if m.State == "staged" {
			fail(w, 409, "Commit the migration first")
			return
		}
		tx, e := a.DB.Begin()
		if e != nil {
			a.internal(w, e)
			return
		}
		defer tx.Rollback()
		_, e = tx.Exec("UPDATE migrations SET state='handed_over' WHERE id=?", mid)
		if e == nil {
			_, e = tx.Exec("UPDATE migration_playlists SET state='handed_over' WHERE migration_id=?", mid)
		}
		if e == nil {
			e = audit(tx, u.ID, u.ID, "migration.handed_over", mid)
		}
		if e == nil {
			e = tx.Commit()
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	case "playlists":
		if len(parts) != 4 {
			fail(w, 404, "Playlist action not found")
			return
		}
		pid := parts[2]
		var state string
		if e = a.DB.QueryRow("SELECT state FROM migration_playlists WHERE id=? AND migration_id=?", pid, mid).Scan(&state); e != nil {
			fail(w, 404, "Playlist not found")
			return
		}
		if parts[3] == "handover" {
			_, e = a.DB.Exec("UPDATE migration_playlists SET state='handed_over' WHERE id=?", pid)
		} else if parts[3] == "resume" {
			if m.State != "open" || state != "paused" {
				fail(w, 409, "Playlist is not paused in an open migration")
				return
			}
			var in struct {
				Baseline string `json:"baseline"`
			}
			if !decode(w, r, &in) {
				return
			}
			e = a.resumePlaylist(r.Context(), u.ID, pid, in.Baseline)
		} else {
			fail(w, 404, "Unknown playlist action")
			return
		}
		if e != nil {
			fail(w, 409, e.Error())
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	default:
		fail(w, 404, "Unknown migration action")
	}
}
func (a *App) cancelMigrationTask(w http.ResponseWriter, r *http.Request, u User) {
	tid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/tasks/"), "/cancel")
	a.opMu.Lock()
	defer a.opMu.Unlock()
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	var owner, mid, action, state, message string
	if err = tx.QueryRow("SELECT owner,migration_id,action,state,message FROM migration_tasks WHERE id=?", tid).Scan(&owner, &mid, &action, &state, &message); err != nil || (owner != u.ID && u.Role != "admin") {
		fail(w, 404, "Task not found")
		return
	}
	if state == "cancelled" {
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if state != "queued" && state != "failed" && state != "blocked" {
		fail(w, 409, "Only queued or failed migration tasks can be cancelled")
		return
	}
	if _, err = tx.Exec("UPDATE migration_tasks SET state='cancelled',message='Cancelled by user' WHERE id=?", tid); err == nil {
		_, err = tx.Exec("DELETE FROM migration_task_wakeups WHERE migration_id=? AND action=? AND NOT EXISTS(SELECT 1 FROM migration_tasks WHERE migration_id=? AND action=? AND state IN ('queued','running'))", mid, action, mid, action)
	}
	if err == nil {
		_, err = tx.Exec("UPDATE migrations SET error='' WHERE id=? AND error=? AND NOT EXISTS(SELECT 1 FROM migration_tasks WHERE migration_id=? AND state IN ('queued','running','failed','blocked'))", mid, message, mid)
	}
	if err == nil {
		err = audit(tx, owner, u.ID, "migration.task_cancelled", tid)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}

func (a *App) queueMigration(owner, mid, action string) (string, error) {
	tx, e := a.DB.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	tid := id()
	if _, e = tx.Exec("INSERT OR IGNORE INTO migration_tasks(id,owner,migration_id,action) VALUES(?,?,?,?)", tid, owner, mid, action); e != nil {
		return "", e
	}
	var state string
	if e = tx.QueryRow("SELECT id,state FROM migration_tasks WHERE migration_id=? AND action=? AND state IN ('queued','running')", mid, action).Scan(&tid, &state); e != nil {
		return "", e
	}
	if state == "running" {
		if _, e = tx.Exec("INSERT OR IGNORE INTO migration_task_wakeups VALUES(?,?)", mid, action); e != nil {
			return "", e
		}
	}
	return tid, tx.Commit()
}
func (a *App) finishMigrationTask(tid, owner, mid, action, state, message string) error {
	tx, e := a.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE migration_tasks SET state=?,message=? WHERE id=?", state, message, tid); e != nil {
		return e
	}
	var pending int
	if e = tx.QueryRow("SELECT count(*) FROM migration_task_wakeups WHERE migration_id=? AND action=?", mid, action).Scan(&pending); e != nil {
		return e
	}
	if pending > 0 {
		if _, e = tx.Exec("INSERT OR IGNORE INTO migration_tasks(id,owner,migration_id,action) SELECT ?,?,?,? WHERE EXISTS(SELECT 1 FROM migrations WHERE id=? AND state='open')", id(), owner, mid, action, mid); e != nil {
			return e
		}
		if _, e = tx.Exec("DELETE FROM migration_task_wakeups WHERE migration_id=? AND action=?", mid, action); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (a *App) sourceAction(w http.ResponseWriter, r *http.Request, u User, m Migration, sid, action string) {
	var data, candidate, server string
	var deferred bool
	e := a.DB.QueryRow("SELECT metadata,candidate,server_id,deferred FROM source_songs WHERE id=? AND migration_id=?", sid, m.ID).Scan(&data, &candidate, &server, &deferred)
	if e != nil {
		fail(w, 404, "Source song not found")
		return
	}
	var source apple.Song
	json.Unmarshal([]byte(data), &source)
	if action == "candidates" {
		var in struct {
			Query string `json:"query"`
		}
		if !decode(w, r, &in) {
			return
		}
		q := strings.TrimSpace(in.Query)
		if q == "" {
			q = source.Artist + " " + source.Title
		}
		if len(q) > 500 {
			fail(w, 400, "Search is too long")
			return
		}
		tracks := []Track{}
		navSongs := []Song{}
		libraryQuery := q
		if strings.HasPrefix(q, "http") {
			// A Deezer URL identifies a recording; use the source metadata
			// for the accompanying local search instead of the literal URL.
			libraryQuery = strings.TrimSpace(source.Artist + " " + source.Title)
		}
		var libraryError, deezerError string
		if c, err := a.shared(); err != nil {
			libraryError = err.Error()
		} else if results, err := a.navSearch(r.Context(), c, libraryQuery, 0); err != nil {
			libraryError = err.Error()
		} else {
			navSongs = results
		}
		if strings.HasPrefix(q, "http") {
			if track, err := a.track(r.Context(), q); err != nil {
				deezerError = err.Error()
			} else {
				tracks = append(tracks, track)
			}
		} else {
			results, err := a.searchImportRecordings(r.Context(), source, q)
			tracks = results
			if err != nil {
				deezerError = err.Error()
			}
		}
		assessments := map[int64]RecordingMatch{}
		confident := 0
		for _, track := range tracks {
			assessments[track.ID] = assessRecording(source, track)
			if assessments[track.ID].Confident {
				confident++
			}
		}
		a.rememberReviewTracks(tracks)
		libraryMatches := map[string]RecordingMatch{}
		for _, song := range navSongs {
			libraryMatches[song.ID] = assessLibraryRecording(source, song)
		}
		send(w, 200, map[string]any{"deezer": tracks, "deezerMatches": assessments, "deezerAmbiguous": confident > 1 && confidentRecording(source, tracks) == nil, "library": navSongs, "libraryMatches": libraryMatches, "libraryError": libraryError, "deezerError": deezerError})
		return
	}
	var resolvedServer, resolvedCandidate string
	if action == "resolve" {
		if m.State == "staged" {
			fail(w, 409, "Commit the migration first")
			return
		}
		var in struct {
			Target   string `json:"target"`
			ServerID string `json:"serverId"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.ServerID != "" {
			c, err := a.personal(u.ID)
			if err != nil {
				fail(w, 400, err.Error())
				return
			}
			response, err := a.nav(r.Context(), c, "getSong", mapValues("id", in.ServerID))
			if err != nil || response.Song.ID != in.ServerID {
				fail(w, 400, "Song is not available to your linked Navidrome account")
				return
			}
			resolvedServer = in.ServerID
		} else {
			track, err := a.reviewTrack(r.Context(), in.Target)
			if err != nil {
				fail(w, 400, err.Error())
				return
			}
			resolvedCandidate = apple.JSON(track)
		}
	}
	a.opMu.Lock()
	defer a.opMu.Unlock()
	// Reads above may have overlapped a deletion or another source action.
	m, e = a.getMigration(m.ID, u.ID)
	if e != nil {
		fail(w, 404, "Migration not found")
		return
	}
	e = a.DB.QueryRow("SELECT candidate,server_id,deferred FROM source_songs WHERE id=? AND migration_id=?", sid, m.ID).Scan(&candidate, &server, &deferred)
	if e != nil {
		fail(w, 404, "Source song not found")
		return
	}
	if m.State == "staged" {
		fail(w, 409, "Commit the migration first")
		return
	}
	switch action {
	case "favorite-done", "favorite-retry":
		current, err := a.getMigration(m.ID, u.ID)
		if err != nil || current.State != "open" {
			fail(w, 409, "Favorites cannot change after handover")
			return
		}
		var state string
		if e = a.DB.QueryRow("SELECT favorite_state FROM source_songs WHERE id=?", sid).Scan(&state); e != nil {
			a.internal(w, e)
			return
		}
		if state != "review" {
			fail(w, 409, "Favorite does not need review")
			return
		}
		next := "applied"
		if action == "favorite-retry" {
			next = "pending"
		}
		_, e = a.DB.Exec("UPDATE source_songs SET favorite_state=? WHERE id=?", next, sid)
		if e == nil && action == "favorite-retry" {
			_, e = a.queueMigration(u.ID, m.ID, "publish")
		}
	case "defer", "resume":
		_, e = a.DB.Exec("UPDATE source_songs SET deferred=? WHERE id=?", action == "defer", sid)
	case "resolve":
		var pending int
		if e = a.DB.QueryRow(`SELECT count(*) FROM request_sources rs JOIN requests r ON r.id=rs.request_id LEFT JOIN jobs j ON j.request_id=r.id WHERE rs.source_id=? AND r.decision IN ('approved','awaiting_approval') AND COALESCE(j.state,'') NOT IN ('failed','cancelled','available')`, sid).Scan(&pending); e != nil {
			a.internal(w, e)
			return
		}
		if pending > 0 {
			fail(w, 409, "Withdraw or finish the existing request before changing its recording")
			return
		}
		e = a.resolveSource(sid, resolvedServer, resolvedCandidate)
	case "acquire":
		if deferred {
			fail(w, 409, "Resume this song before requesting acquisition")
			return
		}
		if server != "" {
			fail(w, 409, "This song is already available")
			return
		}
		if candidate == "" {
			fail(w, 409, "Resolve this song to a Deezer recording first")
			return
		}
		var t Track
		json.Unmarshal([]byte(candidate), &t)
		body := apple.JSON(map[string]any{"target": fmt.Sprintf("deezer:track:%d", t.ID), "title": trackLabel(t), "artist": t.Artist.Name, "upgrade": false, "sourceId": sid})
		r.Body = io.NopCloser(strings.NewReader(body))
		var prior, decision string
		_ = a.DB.QueryRow("SELECT r.id,r.decision FROM request_sources rs JOIN requests r ON r.id=rs.request_id WHERE rs.source_id=? ORDER BY r.rowid DESC LIMIT 1", sid).Scan(&prior, &decision)
		if decision == "approved" || decision == "awaiting_approval" {
			send(w, 200, map[string]string{"id": prior})
			return
		}
		var revision string
		_ = a.DB.QueryRow("SELECT revision FROM source_resolutions WHERE source_id=?", sid).Scan(&revision)
		r.Header.Set("Idempotency-Key", digest(sid+fmt.Sprint(t.ID)+prior+revision))
		a.createVerifiedRequest(w, r, u, &t)
		return
	default:
		fail(w, 404, "Unknown source action")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	if m.State == "open" && m.TargetUser != "" && action == "resolve" {
		_, _ = a.queueMigration(u.ID, m.ID, "publish")
	}
	response := map[string]any{"ok": true}
	if songs, err := a.sources(m.ID, sid); err == nil && len(songs) == 1 {
		response["source"] = songs[0]
	}
	send(w, 200, response)
}

func (a *App) resolveSource(sid, serverID, candidate string) error {
	tx, e := a.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("INSERT INTO source_resolutions(source_id,revision) VALUES(?,?) ON CONFLICT(source_id) DO UPDATE SET revision=excluded.revision", sid, id()); e != nil {
		return e
	}
	if _, e = tx.Exec("DELETE FROM request_sources WHERE source_id=?", sid); e != nil {
		return e
	}
	if _, e = tx.Exec("UPDATE source_songs SET server_id=?,candidate=?,match_state='manual',deferred=0 WHERE id=?", serverID, candidate, sid); e != nil {
		return e
	}
	return tx.Commit()
}

// deleteMigration removes a private snapshot under the publication write lock,
// so no collection writes can outlive deletion. Matching checks the latest
// migration state in its conditional updates.
// Acquisition requests are independent records and retain their lifecycle.
func (a *App) deleteMigration(w http.ResponseWriter, u User, mid string) {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	m, err := a.getMigration(mid, u.ID)
	if err != nil {
		fail(w, 404, "Migration not found")
		return
	}
	if m.State != "staged" && m.State != "open" {
		fail(w, 409, "Only unfinished migrations can be deleted")
		return
	}
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	for _, statement := range []string{
		"DELETE FROM request_sources WHERE source_id IN (SELECT id FROM source_songs WHERE migration_id=?)",
		"DELETE FROM source_resolutions WHERE source_id IN (SELECT id FROM source_songs WHERE migration_id=?)",
		"DELETE FROM source_songs WHERE migration_id=?",
		"DELETE FROM migration_playlists WHERE migration_id=?",
		"DELETE FROM migration_task_wakeups WHERE migration_id=?",
		"DELETE FROM migration_tasks WHERE migration_id=?",
		"DELETE FROM migrations WHERE id=?",
	} {
		if _, err = tx.Exec(statement, mid); err != nil {
			a.internal(w, err)
			return
		}
	}
	if err = audit(tx, u.ID, u.ID, "migration.deleted", mid); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}
