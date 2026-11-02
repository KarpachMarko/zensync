package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type AlbumSnapshot struct {
	ID     int64   `json:"id"`
	Title  string  `json:"title"`
	Artist Artist  `json:"artist"`
	Tracks []Track `json:"tracks"`
}
type AlbumItem struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

func (a *App) albumSnapshot(ctx context.Context, id string) (AlbumSnapshot, error) {
	var album AlbumSnapshot
	var metadata struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Artist Artist `json:"artist"`
	}
	if e := a.deezer(ctx, "/album/"+id, &metadata); e != nil {
		return album, e
	}
	album.ID = metadata.ID
	album.Title = metadata.Title
	album.Artist = metadata.Artist
	if album.ID == 0 || strconv.FormatInt(album.ID, 10) != id || album.Title == "" {
		return album, errors.New("Deezer did not return the requested album")
	}
	tracks, e := a.albumTracks(ctx, id)
	if e != nil {
		return album, e
	}
	if len(tracks) == 0 {
		return album, errors.New("Album has no tracks")
	}
	seen := map[int64]bool{}
	album.Tracks = []Track{}
	for _, t := range tracks {
		if t.ID <= 0 || t.Title == "" || t.Artist.Name == "" || t.Duration <= 0 {
			return album, errors.New("Album includes an incomplete recording; request individual tracks after review")
		}
		if !seen[t.ID] {
			seen[t.ID] = true
			album.Tracks = append(album.Tracks, t)
		}
	}
	if album.Artist.Name == "" {
		album.Artist.Name = album.Tracks[0].Artist.Name
	}
	return album, nil
}
func (a *App) createAlbumRequest(w http.ResponseWriter, r *http.Request, u User, target, key string) {
	idString := strings.TrimPrefix(target, "deezer:album:")
	if !digits.MatchString(idString) || len(key) < 8 || len(key) > 128 {
		fail(w, 400, "Valid album target and Idempotency-Key required")
		return
	}
	var existing, oldTarget string
	e := a.DB.QueryRow("SELECT id,target FROM requests WHERE owner=? AND idem=?", u.ID, key).Scan(&existing, &oldTarget)
	if e == nil {
		if oldTarget != target {
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
	album, e := a.albumSnapshot(r.Context(), idString)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	// A concurrent retry may have committed while metadata was fetched.
	e = tx.QueryRow("SELECT id,target FROM requests WHERE owner=? AND idem=?", u.ID, key).Scan(&existing, &oldTarget)
	if e == nil {
		if oldTarget != target {
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
	rid := id()
	decision, reason := "awaiting_approval", ""
	if u.Role == "admin" || u.Direct {
		decision, reason = "approved", "Allowed without approval"
	}
	if _, e = tx.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision,reason) VALUES(?,?,?,?,?,?,0,?,?,?)", rid, u.ID, key, target, album.Title, album.Artist.Name, string(u.Quality), decision, reason); e != nil {
		a.internal(w, e)
		return
	}
	if _, e = tx.Exec("INSERT INTO album_requests(request_id,snapshot) VALUES(?,?)", rid, mustJSON(album)); e != nil {
		a.internal(w, e)
		return
	}
	if decision == "approved" {
		e = a.expandAlbum(tx, rid)
	}
	if e == nil {
		e = audit(tx, u.ID, u.ID, "album.requested", rid)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 201, map[string]string{"id": rid})
}

// Child execution intents inherit the single album decision and frozen quality/
// recording list. They never appear as separate approval requests.
func (a *App) expandAlbum(tx *sql.Tx, parent string) error {
	var snapshot, owner, quality, reason string
	var actor sql.NullString
	if e := tx.QueryRow("SELECT a.snapshot,r.owner,r.quality,r.reason,r.actor FROM album_requests a JOIN requests r ON r.id=a.request_id WHERE a.request_id=?", parent).Scan(&snapshot, &owner, &quality, &reason, &actor); e != nil {
		return e
	}
	var album AlbumSnapshot
	if e := json.Unmarshal([]byte(snapshot), &album); e != nil {
		return e
	}
	if _, e := tx.Exec("INSERT OR IGNORE INTO jobs(id,request_id,state,stage) VALUES(?,?,'queued','Album approved; tracks queued')", id(), parent); e != nil {
		return e
	}
	for i, t := range album.Tracks {
		var child string
		e := tx.QueryRow("SELECT child_id FROM request_children WHERE parent_id=? AND position=?", parent, i).Scan(&child)
		if e == sql.ErrNoRows {
			child = id()
			if _, e = tx.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision,reason,actor) VALUES(?,?,?,?,?,?,0,?,'approved',?,?)", child, owner, "album:"+parent+":"+strconv.Itoa(i), fmt.Sprintf("deezer:track:%d", t.ID), trackLabel(t), t.Artist.Name, quality, reason, actor); e != nil {
				return e
			}
			if _, e = tx.Exec("INSERT INTO request_metadata(request_id,metadata) VALUES(?,?)", child, mustJSON(t)); e != nil {
				return e
			}
			if _, e = tx.Exec("INSERT INTO request_children VALUES(?,?,?)", parent, child, i); e != nil {
				return e
			}
		} else if e != nil {
			return e
		} else {
			if _, e = tx.Exec("UPDATE requests SET decision='approved',reason=?,actor=? WHERE id=?", reason, actor, child); e != nil {
				return e
			}
		}
		if _, e = tx.Exec("INSERT OR IGNORE INTO jobs(id,request_id) VALUES(?,?)", id(), child); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) albumItems(parent string) ([]AlbumItem, error) {
	rows, e := a.DB.Query("SELECT r.title,r.artist,j.state,j.stage FROM request_children c JOIN requests r ON r.id=c.child_id JOIN jobs j ON j.request_id=r.id WHERE c.parent_id=? ORDER BY c.position", parent)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []AlbumItem{}
	for rows.Next() {
		var i AlbumItem
		if e = rows.Scan(&i.Title, &i.Artist, &i.State, &i.Detail); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (a *App) refreshAlbums() {
	rows, e := a.DB.Query("SELECT a.request_id,r.decision FROM album_requests a JOIN requests r ON r.id=a.request_id JOIN jobs j ON j.request_id=r.id WHERE r.decision='approved' AND j.state<>'available' ")
	if e != nil {
		return
	}
	type parent struct{ id, decision string }
	parents := []parent{}
	for rows.Next() {
		var p parent
		if rows.Scan(&p.id, &p.decision) == nil {
			parents = append(parents, p)
		}
	}
	rows.Close()
	for _, p := range parents {
		if p.decision != "approved" {
			continue
		}
		items, e := a.albumItems(p.id)
		if e != nil || len(items) == 0 {
			continue
		}
		available, failed := 0, 0
		for _, i := range items {
			if i.State == "available" {
				available++
			}
			if i.State == "failed" {
				failed++
			}
		}
		state := "processing"
		if available == len(items) {
			state = "available"
		} else if available+failed == len(items) {
			state = "failed"
		}
		_, _ = a.DB.Exec("UPDATE jobs SET state=?,stage=? WHERE request_id=?", state, fmt.Sprintf("Album: %d/%d available, %d failed", available, len(items), failed), p.id)
	}
}
func (a *App) retryAlbum(tx *sql.Tx, parent string) error {
	if _, e := tx.Exec("DELETE FROM scan_waits WHERE job_id IN (SELECT j.id FROM jobs j JOIN request_children c ON c.child_id=j.request_id WHERE c.parent_id=? AND j.state='failed')", parent); e != nil {
		return e
	}
	if _, e := tx.Exec("DELETE FROM scan_schedule WHERE job_id IN (SELECT j.id FROM jobs j JOIN request_children c ON c.child_id=j.request_id WHERE c.parent_id=? AND j.state='failed')", parent); e != nil {
		return e
	}
	_, e := tx.Exec("UPDATE jobs SET state='queued',stage='Retry album track' WHERE request_id IN (SELECT child_id FROM request_children WHERE parent_id=?) AND state='failed'", parent)
	return e
}
