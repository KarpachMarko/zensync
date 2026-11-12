package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var targetPattern = regexp.MustCompile(`^(deezer:track:[1-9][0-9]{0,19}|lossless:track:[a-zA-Z0-9_-]{1,40}:[1-9][0-9]{0,18})$`)

func (a *App) listRequests(w http.ResponseWriter, u User) {
	query := `WITH request_rows AS (
 SELECT r.*, CASE WHEN r.decision<>'approved' THEN r.decision ELSE (
 SELECT CASE
 WHEN count(*)=0 THEN 'queued'
 WHEN sum(state='superseded')=count(*) THEN 'superseded'
 WHEN sum(state IN ('available','completed','cancelled','superseded'))=count(*) THEN 'completed'
 WHEN sum(state NOT IN ('available','completed','cancelled','failed','blocked','deferred','superseded'))>0 THEN
   CASE WHEN count(DISTINCT state)=1 THEN min(state) ELSE 'running' END
 WHEN sum(state='failed')>0 THEN 'failed'
 ELSE 'blocked' END FROM jobs
 WHERE (request_id=r.id AND NOT EXISTS(SELECT 1 FROM request_children WHERE parent_id=r.id))
 OR request_id IN (SELECT child_id FROM request_children WHERE parent_id=r.id)
 ) END AS execution_state FROM requests r
 WHERE NOT EXISTS(SELECT 1 FROM request_children c WHERE c.child_id=r.id)`
	args := []any{}
	if u.Role != "admin" {
		query += " AND r.owner=?"
		args = append(args, u.ID)
	}
	query += `) SELECT id,owner,target,title,artist,upgrade,quality,decision,reason,created,execution_state FROM request_rows ORDER BY ` + executionOrderSQL("execution_state") + `,created DESC,id LIMIT 200`
	rows, e := a.DB.Query(query, args...)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	out := []Request{}
	for rows.Next() {
		var x Request
		var q string
		if e = rows.Scan(&x.ID, &x.Owner, &x.Target, &x.Title, &x.Artist, &x.Upgrade, &q, &x.Decision, &x.Reason, &x.Created, &x.State); e != nil {
			a.internal(w, e)
			return
		}
		x.Quality = json.RawMessage(q)
		out = append(out, x)
	}
	if e = rows.Err(); e != nil {
		a.internal(w, e)
		return
	}
	rows.Close()
	for index := range out {
		if strings.HasPrefix(out[index].Target, "navidrome:song:") {
			out[index].Kind = "delete"
			err := a.DB.QueryRow("SELECT state,stage FROM jobs WHERE request_id=?", out[index].ID).Scan(&out[index].State, &out[index].Detail)
			if err != nil && err != sql.ErrNoRows {
				a.internal(w, err)
				return
			}
		} else if strings.HasPrefix(out[index].Target, "deezer:album:") {
			out[index].Kind = "album"
			out[index].Items, e = a.albumItems(out[index].ID)
			if e != nil {
				a.internal(w, e)
				return
			}
			var snapshot string
			if e = a.DB.QueryRow("SELECT snapshot FROM album_requests WHERE request_id=?", out[index].ID).Scan(&snapshot); e != nil {
				a.internal(w, e)
				return
			}
			var album AlbumSnapshot
			json.Unmarshal([]byte(snapshot), &album)
			out[index].TrackCount = len(album.Tracks)
			if len(out[index].Items) == 0 {
				for _, t := range album.Tracks {
					out[index].Items = append(out[index].Items, AlbumItem{Title: trackLabel(t), Artist: t.Artist.Name, State: "awaiting_approval"})
				}
			}
		} else {
			out[index].Kind = "track"
			if err := a.DB.QueryRow("SELECT id,stage FROM jobs WHERE request_id=?", out[index].ID).Scan(&out[index].JobID, &out[index].Detail); err != nil && err != sql.ErrNoRows {
				a.internal(w, err)
				return
			}
			if strings.HasPrefix(out[index].Target, "upload:") {
				out[index].Kind = "upload"
			}
			if out[index].State == "downloading" {
				var job string
				if err := a.DB.QueryRow("SELECT id FROM jobs WHERE request_id=?", out[index].ID).Scan(&job); err != nil && err != sql.ErrNoRows {
					a.internal(w, err)
					return
				}
				out[index].Transfer = a.transfer(job, out[index].State)
			}
		}
	}
	send(w, 200, out)
}
func (a *App) createRequest(w http.ResponseWriter, r *http.Request, u User) {
	a.createVerifiedRequest(w, r, u, nil)
}

// Internal migration requests already have a server-verified recording. The
// worker still rechecks provider metadata before executing an approved request.
func (a *App) createVerifiedRequest(w http.ResponseWriter, r *http.Request, u User, recording *Track) {
	var in struct {
		Target   string `json:"target"`
		Title    string `json:"title"`
		Artist   string `json:"artist"`
		Upgrade  bool   `json:"upgrade"`
		SourceID string `json:"sourceId"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.HasPrefix(r.Header.Get("Idempotency-Key"), "album:") {
		fail(w, 400, "Reserved request key prefix")
		return
	}
	if strings.HasPrefix(in.Target, "deezer:album:") {
		if in.Upgrade || in.SourceID != "" {
			fail(w, 400, "Album upgrades and migration-entry album substitutions are not supported")
			return
		}
		a.createAlbumRequest(w, r, u, in.Target, r.Header.Get("Idempotency-Key"))
		return
	}
	var verified Track
	{
		var t Track
		var e error
		if recording != nil {
			t = *recording
		} else {
			t, e = a.track(r.Context(), in.Target)
		}
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		in.Target = trackTarget(t)
		in.Title = trackLabel(t)
		in.Artist = t.Artist.Name
		verified = t
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 128 || !targetPattern.MatchString(in.Target) || strings.TrimSpace(in.Title) == "" || len(in.Title) > 300 || len(in.Artist) > 300 {
		fail(w, 400, "A verified provider song and an 8–128 character Idempotency-Key are required")
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	if in.SourceID != "" {
		var candidate string
		var deferred bool
		if e = tx.QueryRow("SELECT s.candidate,s.deferred FROM source_songs s JOIN migrations m ON m.id=s.migration_id WHERE s.id=? AND m.owner=? AND m.state<>'staged'", in.SourceID, u.ID).Scan(&candidate, &deferred); e != nil {
			fail(w, 404, "Source song not found")
			return
		}
		var chosen Track
		_ = json.Unmarshal([]byte(candidate), &chosen)
		if deferred || in.Target != trackTarget(chosen) {
			fail(w, 409, "Source song target changed or is deferred")
			return
		}
	}
	var existing, t, title, artist string
	var upgrade bool
	e = tx.QueryRow("SELECT id,target,title,artist,upgrade FROM requests WHERE owner=? AND idem=?", u.ID, key).Scan(&existing, &t, &title, &artist, &upgrade)
	if e == nil {
		if t != in.Target || title != in.Title || artist != in.Artist || upgrade != in.Upgrade {
			fail(w, 409, "Idempotency key already used for another request")
			return
		}
		send(w, 200, map[string]string{"id": existing})
		return
	}
	if e != sql.ErrNoRows {
		a.internal(w, e)
		return
	}
	rid, e := insertTrackRequest(tx, u, key, in.SourceID, verified, in.Upgrade)
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	response := map[string]any{"id": rid}
	if in.SourceID != "" {
		var mid string
		if a.DB.QueryRow("SELECT migration_id FROM source_songs WHERE id=?", in.SourceID).Scan(&mid) == nil {
			if songs, err := a.sources(mid, in.SourceID); err == nil && len(songs) == 1 {
				response["source"] = songs[0]
			}
		}
	}
	send(w, 201, response)
}

// Callers validate the server-fetched recording and handle idempotency before
// insertion. Single and bulk requests share approval, quality and job creation.
func insertTrackRequest(tx *sql.Tx, u User, key, sourceID string, track Track, upgrade bool) (string, error) {
	rid := id()
	decision, reason := "awaiting_approval", ""
	if !upgrade && (u.Role == "admin" || u.Direct) {
		decision, reason = "approved", "Allowed without approval"
	}
	_, err := tx.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision,reason) VALUES(?,?,?,?,?,?,?,?,?,?)", rid, u.ID, key, trackTarget(track), trackLabel(track), track.Artist.Name, upgrade, string(u.Quality), decision, reason)
	if err == nil {
		_, err = tx.Exec("INSERT INTO request_metadata(request_id,metadata) VALUES(?,?)", rid, mustJSON(track))
	}
	if err == nil && sourceID != "" {
		_, err = tx.Exec("INSERT INTO request_sources(source_id,request_id) VALUES(?,?)", sourceID, rid)
	}
	if err == nil && decision == "approved" {
		_, err = tx.Exec("INSERT INTO jobs(id,request_id,state,stage) VALUES(?,?,'queued','Queued for acquisition')", id(), rid)
	}
	if err == nil {
		err = audit(tx, u.ID, u.ID, "request.created", rid)
	}
	return rid, err
}
func (a *App) decide(w http.ResponseWriter, r *http.Request, u User) {
	rid := strings.TrimPrefix(r.URL.Path, "/api/v2/requests/")
	var in struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Decision != "approved" && in.Decision != "declined" && in.Decision != "withdrawn") || len(in.Reason) > 1000 || (in.Decision == "declined" && strings.TrimSpace(in.Reason) == "") {
		fail(w, 400, "Choose a valid decision; declining requires a reason")
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var owner, state string
	e = tx.QueryRow("SELECT owner,decision FROM requests WHERE id=?", rid).Scan(&owner, &state)
	if e == sql.ErrNoRows || (e == nil && u.Role != "admin" && owner != u.ID) {
		fail(w, 404, "Request not found")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	var deletion int
	if e = tx.QueryRow("SELECT count(*) FROM song_deletions WHERE request_id=?", rid).Scan(&deletion); e != nil {
		a.internal(w, e)
		return
	}
	var children, parentAlbum int
	if e = tx.QueryRow("SELECT count(*) FROM request_children WHERE child_id=?", rid).Scan(&children); e != nil {
		a.internal(w, e)
		return
	}
	if children > 0 {
		fail(w, 409, "Decide the parent album request instead")
		return
	}
	if e = tx.QueryRow("SELECT count(*) FROM album_requests WHERE request_id=?", rid).Scan(&parentAlbum); e != nil {
		a.internal(w, e)
		return
	}
	if (in.Decision == "withdrawn" && owner != u.ID) || (in.Decision != "withdrawn" && u.Role != "admin") {
		fail(w, 403, "Decision not permitted")
		return
	}
	if state != "awaiting_approval" {
		fail(w, 409, "Request is no longer awaiting approval")
		return
	}
	_, e = tx.Exec("UPDATE requests SET decision=?,reason=?,actor=? WHERE id=?", in.Decision, in.Reason, u.ID, rid)
	if e == nil && in.Decision == "approved" {
		if deletion > 0 {
			_, e = tx.Exec("INSERT INTO jobs(id,request_id,state,stage) VALUES(?,?,'queued','Queued for library removal') ON CONFLICT(request_id) DO UPDATE SET state='queued',stage='Queued for library removal'", id(), rid)
		} else if parentAlbum > 0 {
			e = a.expandAlbum(tx, rid)
		} else {
			_, e = tx.Exec("INSERT OR IGNORE INTO jobs(id,request_id,state,stage) VALUES(?,?,'queued','Queued for acquisition')", id(), rid)
		}
	}
	if e == nil && deletion > 0 && in.Decision != "approved" {
		_, e = tx.Exec("UPDATE jobs SET state='cancelled',stage=? WHERE request_id=?", in.Decision, rid)
	}
	if e == nil && parentAlbum > 0 && in.Decision != "approved" {
		_, e = tx.Exec("UPDATE requests SET decision=?,reason=?,actor=? WHERE id IN (SELECT child_id FROM request_children WHERE parent_id=?)", in.Decision, in.Reason, u.ID, rid)
		if e == nil {
			_, e = tx.Exec("UPDATE jobs SET state='cancelled',stage=? WHERE request_id IN (SELECT child_id FROM request_children WHERE parent_id=?) AND state<>'available'", in.Decision, rid)
		}
		if e == nil {
			_, e = tx.Exec("UPDATE jobs SET state='cancelled',stage=? WHERE request_id=?", in.Decision, rid)
		}
	}
	if e == nil {
		e = audit(tx, owner, u.ID, "request."+in.Decision, rid)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	if in.Decision != "approved" {
		var target string
		if a.DB.QueryRow("SELECT target FROM requests WHERE id=?", rid).Scan(&target) == nil {
			a.cleanAudioUpload(target)
		}
	}
	send(w, 200, map[string]bool{"ok": true})
}
func (a *App) activity(w http.ResponseWriter, u User) {
	query := `SELECT id,title,state,CASE WHEN state='superseded' AND stage NOT LIKE 'Replaced by %' THEN 'Superseded by a newer successful request. Previous failure: '||stage ELSE stage END,created,kind,owner FROM (
 SELECT j.id,r.title,CASE WHEN j.state IN ('failed','blocked') AND EXISTS (
 SELECT 1 FROM requests newer JOIN jobs success ON success.request_id=newer.id
 WHERE newer.owner=r.owner AND newer.target=r.target AND newer.upgrade=r.upgrade AND newer.quality=r.quality AND newer.rowid>r.rowid AND success.state='available'
 ) THEN 'superseded' ELSE j.state END AS state,j.stage,j.created,r.owner,CASE WHEN (r.target LIKE 'deezer:track:%' OR r.target LIKE 'lossless:track:%') THEN 'acquisition' ELSE 'other' END AS kind FROM jobs j JOIN requests r ON r.id=j.request_id
 UNION ALL SELECT t.id,m.name||' / '||t.action,t.state,t.message,t.created,t.owner,'migration' FROM migration_tasks t JOIN migrations m ON m.id=t.migration_id
 UNION ALL SELECT id,'Organize legacy download / '||old_path,state,message,created,owner,'relocation' FROM asset_relocations
 `
	if u.Role == "admin" {
		query += " UNION ALL SELECT id," + metadataActivityTitleSQL + ",state," + metadataActivityStageSQL + ",created,actor,'metadata' FROM metadata_jobs "
	}
	query += ")"
	args := []any{}
	if u.Role != "admin" {
		query += " WHERE owner=?"
		args = append(args, u.ID)
	}
	query += " ORDER BY " + executionOrderSQL("state") + ", created DESC, id LIMIT 200"
	rows, e := a.DB.Query(query, args...)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, title, state, stage, created, kind, owner string
		if e = rows.Scan(&id, &title, &state, &stage, &created, &kind, &owner); e != nil {
			a.internal(w, e)
			return
		}
		item := map[string]any{"id": id, "title": title, "state": state, "stage": stage, "created": created, "kind": kind}
		item["canCancel"] = kind == "migration" && (state == "queued" || state == "failed" || state == "blocked") && (owner == u.ID || u.Role == "admin")
		item["canReview"] = kind == "acquisition" && (state == "failed" || state == "blocked")
		item["canDismiss"] = kind == "acquisition" && state == "failed"
		if transfer := a.transfer(id, state); transfer != nil {
			item["transfer"] = transfer
		}
		out = append(out, item)
	}
	if e = rows.Err(); e != nil {
		a.internal(w, e)
		return
	}
	rows.Close() // SQLite uses a single connection; enrich only after releasing rows.
	for _, item := range out {
		if item["state"] != "completed" {
			continue
		}
		var rid, phase, restorePhase string
		err := a.DB.QueryRow(`SELECT d.request_id,d.phase,COALESCE(x.phase,'') FROM song_deletions d JOIN jobs j ON j.request_id=d.request_id LEFT JOIN deletion_restores x ON x.request_id=d.request_id WHERE j.id=?`, item["id"]).Scan(&rid, &phase, &restorePhase)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		item["canRestore"] = u.Role == "admin" && phase == "removed" && (a.availableDeletionCopy(rid) || restorePhase == "published")
		if phase == "restored" {
			item["stage"] = "Deleted file restored; playlist replacements unchanged"
		} else if restorePhase == "published" {
			item["stage"] = "File restored; retry Restore to request a library scan"
		} else if !a.availableDeletionCopy(rid) {
			item["stage"] = "Removed from shared library; recovery copy expired or removed"
		}
	}
	send(w, 200, out)
}
