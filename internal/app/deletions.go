package app

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *App) deletePermission(w http.ResponseWriter, r *http.Request, actor User) {
	if actor.Role != "admin" {
		fail(w, 403, "Administrator required")
		return
	}
	uid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/users/"), "/permissions")
	var in struct {
		DirectDelete *bool `json:"directDelete"`
		Direct       *bool `json:"direct"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.DirectDelete == nil && in.Direct == nil {
		fail(w, 400, "A permission is required")
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var role string
	if e = tx.QueryRow("SELECT role FROM users WHERE id=?", uid).Scan(&role); e == sql.ErrNoRows {
		fail(w, 404, "User not found")
		return
	} else if e != nil {
		a.internal(w, e)
		return
	}
	if role == "admin" {
		fail(w, 400, "Administrators already have deletion permission")
		return
	}
	if in.Direct != nil {
		_, e = tx.Exec("UPDATE users SET direct=? WHERE id=?", *in.Direct, uid)
	}
	if e == nil && in.DirectDelete != nil {
		if *in.DirectDelete {
			_, e = tx.Exec("INSERT OR IGNORE INTO delete_permissions VALUES(?)", uid)
		} else {
			_, e = tx.Exec("DELETE FROM delete_permissions WHERE user_id=?", uid)
		}
	}
	if e == nil {
		e = audit(tx, uid, actor.ID, "permission.delete.updated", mustJSON(in))
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}

// Only server-verified IDs enter this API; callers never supply filesystem paths.
func (a *App) createDeletionRequest(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		SongID string `json:"songId"`
	}
	if !decode(w, r, &in) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 128 || strings.HasPrefix(key, "album:") || strings.TrimSpace(in.SongID) == "" || len(in.SongID) > 256 {
		fail(w, 400, "songId and an 8–128 character Idempotency-Key are required")
		return
	}
	a.opMu.Lock()
	defer a.opMu.Unlock()
	var existing, target string
	e := a.DB.QueryRow("SELECT id,target FROM requests WHERE owner=? AND idem=?", u.ID, key).Scan(&existing, &target)
	if e == nil {
		if target != "navidrome:song:"+in.SongID {
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
	c, e := a.shared()
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	remote, e := a.nav(r.Context(), c, "getSong", url.Values{"id": {in.SongID}, "realPath": {"true"}})
	if e != nil || remote.Song.ID != in.SongID {
		fail(w, 400, "Song is unavailable in the shared Navidrome library")
		return
	}
	rel, checksum, e := a.deletionPath(r.Context(), c, remote.Song)
	if e != nil {
		fail(w, 409, e.Error())
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var pending int
	e = tx.QueryRow("SELECT count(*) FROM song_deletions d JOIN requests r ON r.id=d.request_id LEFT JOIN jobs j ON j.request_id=r.id WHERE d.file_path=? AND (r.decision='awaiting_approval' OR (r.decision='approved' AND j.state NOT IN ('completed','failed')))", rel).Scan(&pending)
	if e != nil {
		a.internal(w, e)
		return
	}
	if pending > 0 {
		fail(w, 409, "This song already has a pending deletion request")
		return
	}
	rid := id()
	decision, reason := "awaiting_approval", ""
	if u.Role == "admin" || u.DirectDelete {
		decision, reason = "approved", "Deletion allowed without approval"
	}
	_, e = tx.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision,reason) VALUES(?,?,?,?,?,?,0,'[]',?,?)", rid, u.ID, key, "navidrome:song:"+in.SongID, remote.Song.Title, remote.Song.Artist, decision, reason)
	if e == nil {
		_, e = tx.Exec("INSERT INTO song_deletions(request_id,song_id,file_path,checksum) VALUES(?,?,?,?)", rid, in.SongID, rel, checksum)
	}
	if e == nil && decision == "approved" {
		_, e = tx.Exec("INSERT INTO jobs(id,request_id,stage) VALUES(?,?,'Queued for library removal')", id(), rid)
	}
	if e == nil {
		e = audit(tx, u.ID, u.ID, "deletion.requested", rid)
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

func (a *App) deletionPath(ctx context.Context, c Credentials, s Song) (string, string, error) {
	candidates := map[string]bool{}
	rows, e := a.DB.Query("SELECT file_path FROM assets WHERE server_id=?", s.ID)
	if e != nil {
		return "", "", e
	}
	for rows.Next() {
		var p string
		if e = rows.Scan(&p); e != nil {
			rows.Close()
			return "", "", e
		}
		candidates[p] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return "", "", e
	}
	// Container mount prefixes may differ. Never trust a suffix without checking original bytes.
	p := strings.ReplaceAll(s.Path, "\\", "/")
	for p != "" {
		if safeRelativePath(p) {
			candidates[p] = true
		}
		i := strings.Index(p, "/")
		if i < 0 {
			break
		}
		p = p[i+1:]
	}
	verify := func(rel string) (string, bool, error) {
		if a.checkMusicParents(rel) != nil {
			return "", false, nil
		}
		path := filepath.Join(a.musicDir, rel)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return "", false, nil
		}
		before, err := hashFile(path)
		if err != nil {
			return "", false, err
		}
		match, err := a.sameRemoteAudio(ctx, c, s, rel)
		if err != nil || !match {
			return "", false, err
		}
		after, err := hashFile(path)
		if err != nil || before != after {
			return "", false, errors.New("Song changed during verification; retry after the file is stable")
		}
		return before, true, nil
	}
	// An absolute real path inside the configured music root identifies the
	// intended file. Shorter suffixes may identify byte-identical extra copies,
	// but must never override that identity (even when the exact file is stale).
	root, err := filepath.Abs(a.musicDir)
	if err != nil {
		return "", "", err
	}
	reported := strings.ReplaceAll(s.Path, "\\", "/")
	if filepath.IsAbs(reported) {
		rel, err := filepath.Rel(root, reported)
		if err == nil && safeRelativePath(rel) {
			checksum, match, err := verify(rel)
			if err != nil {
				return "", "", err
			}
			if !match {
				slog.Warn("Exact Navidrome song path could not be verified", "song_id", s.ID, "reported_path", s.Path, "music_dir", root, "relative_path", rel)
				return "", "", errors.New("The exact Navidrome file path is missing, unsafe, or differs from the downloaded original; check the shared music mount before retrying")
			}
			slog.Info("Exact Navidrome song path verified", "song_id", s.ID, "relative_path", rel)
			return rel, checksum, nil
		}
	}
	matches := []string{}
	checksums := map[string]string{}
	for rel := range candidates {
		checksum, match, err := verify(rel)
		if err != nil {
			return "", "", err
		}
		if match {
			matches = append(matches, rel)
			checksums[rel] = checksum
		}
	}
	sort.Strings(matches)
	if len(matches) > 1 {
		slog.Warn("Song path resolution ambiguous", "song_id", s.ID, "reported_path", s.Path, "music_dir", root, "matching_paths", matches)
		return "", "", errors.New("Multiple verified local copies match this song; enable real-path reporting for the ZenSync player and check the shared music mount paths")
	}
	if len(matches) == 0 {
		slog.Warn("No verified local song path found", "song_id", s.ID, "reported_path", s.Path, "music_dir", root, "candidate_count", len(candidates))
		return "", "", errors.New("Cannot safely locate this song. For music acquired outside ZenSync, enable Subsonic.DefaultReportRealPath in Navidrome and ensure both services share the same music directory")
	}
	return matches[0], checksums[matches[0]], nil
}

// Deletions run between acquisitions and serialize with collection publication and
// relocation. The journal and recovery copy make removal safe to resume on restart.
func (a *App) runDeletion(ctx context.Context) {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.workMu.Lock()
	defer a.workMu.Unlock()
	if len(a.activeJobs) > 0 {
		return
	}
	var jid, rid, owner, actor, rel, checksum, song, phase string
	e := a.DB.QueryRow(`SELECT j.id,r.id,r.owner,COALESCE(r.actor,''),d.file_path,d.checksum,d.song_id,d.phase FROM song_deletions d JOIN requests r ON r.id=d.request_id JOIN jobs j ON j.request_id=r.id WHERE r.decision='approved' AND j.state='queued' ORDER BY j.rowid LIMIT 1`).Scan(&jid, &rid, &owner, &actor, &rel, &checksum, &song, &phase)
	if e != nil {
		return
	}
	if actor == "" && phase != "removed" {
		var allowed bool
		if e = a.DB.QueryRow("SELECT role='admin' OR EXISTS(SELECT 1 FROM delete_permissions WHERE user_id=users.id) FROM users WHERE id=?", owner).Scan(&allowed); e != nil {
			return
		}
		if !allowed {
			tx, e := a.DB.Begin()
			if e != nil {
				return
			}
			defer tx.Rollback()
			if _, e = tx.Exec("UPDATE requests SET decision='awaiting_approval',reason='Deletion permission changed; administrator approval required' WHERE id=?", rid); e == nil {
				_, e = tx.Exec("UPDATE jobs SET state='blocked',stage='Deletion requires administrator approval' WHERE id=?", jid)
			}
			if e == nil {
				e = tx.Commit()
			}
			return
		}
	}
	a.jobState(jid, "queued", "Removing song from the shared library")
	if e = a.removeLibrarySong(ctx, rid, owner, actor, rel, checksum, song, phase); e != nil {
		a.jobState(jid, "failed", e.Error())
		return
	}
	a.jobState(jid, "completed", "Removed from shared library; recovery copy retained")
}

func (a *App) removeLibrarySong(ctx context.Context, rid, owner, actor, rel, checksum, song, phase string) error {
	// A backup inside the scanned directory would simply re-import the removed song.
	data, e := filepath.EvalSymlinks(a.dataDir)
	if e != nil {
		return e
	}
	music, e := filepath.EvalSymlinks(a.musicDir)
	if e != nil {
		return e
	}
	within, e := filepath.Rel(music, data)
	if e != nil || within == "." || (within != ".." && !strings.HasPrefix(within, ".."+string(filepath.Separator))) {
		return errors.New("Recovery storage must be outside the scanned music directory")
	}
	backupDir := filepath.Join(a.dataDir, "trash", rid)
	backup := filepath.Join(backupDir, "audio")
	if phase == "pending" || phase == "prepared" {
		if e = a.checkMusicParents(rel); e != nil {
			return e
		}
		root, e := os.OpenRoot(a.musicDir)
		if e != nil {
			return e
		}
		defer root.Close()
		info, e := root.Lstat(rel)
		if os.IsNotExist(e) && phase == "prepared" {
			if h, err := hashFile(backup); err != nil || h != checksum {
				return errors.New("Recovery copy is missing or changed; removal cannot be reconciled")
			}
		} else {
			if e != nil || !info.Mode().IsRegular() {
				return errors.New("Song file is missing or unsafe; no file was removed")
			}
			h, e := hashFile(filepath.Join(a.musicDir, rel))
			if e != nil || h != checksum {
				return errors.New("Song file changed since the deletion request; submit a new request after review")
			}
			if phase == "pending" {
				if e = os.MkdirAll(backupDir, 0700); e != nil {
					return errors.New("Cannot create recovery directory")
				}
				// A partially written backup can be recreated while the source still exists.
				src, e := root.Open(rel)
				if e != nil {
					return e
				}
				dst, e := os.OpenFile(backup, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
				if e != nil {
					src.Close()
					return e
				}
				_, e = io.Copy(dst, src)
				src.Close()
				if e == nil {
					e = dst.Sync()
				}
				closeErr := dst.Close()
				if e == nil {
					e = closeErr
				}
				if e != nil {
					return errors.New("Cannot persist recovery copy; original retained")
				}
				if e = syncDir(backupDir); e != nil {
					return e
				}
				if e = syncDir(filepath.Dir(backupDir)); e != nil {
					return e
				}
				if e = syncDir(a.dataDir); e != nil {
					return e
				}
				if _, e = a.DB.Exec("UPDATE song_deletions SET phase='prepared' WHERE request_id=?", rid); e != nil {
					return e
				}
			}
			if h, e = hashFile(backup); e != nil || h != checksum {
				return errors.New("Recovery copy verification failed; original retained")
			}
			// Recheck after copying; replacement files must never inherit an old approval.
			current, e := root.Lstat(rel)
			if e != nil || !os.SameFile(info, current) {
				return errors.New("Song file was replaced during removal; original retained")
			}
			if h, e = hashFile(filepath.Join(a.musicDir, rel)); e != nil || h != checksum {
				return errors.New("Song file changed during removal; original retained")
			}
			if e = root.Remove(rel); e != nil {
				return errors.New("Cannot remove song from library; recovery copy retained")
			}
			if e = syncDir(filepath.Dir(filepath.Join(a.musicDir, rel))); e != nil {
				return e
			}
		}
		tx, e := a.DB.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		// Stop old acquisition intents from recreating the removed asset on retry.
		_, e = tx.Exec("UPDATE jobs SET state='cancelled',stage='Song intentionally removed from shared library' WHERE request_id IN (SELECT r.id FROM requests r JOIN assets a ON a.target=r.target WHERE a.file_path=? OR a.server_id=?)", rel, song)
		if e == nil {
			_, e = tx.Exec("DELETE FROM asset_relocations WHERE target IN (SELECT target FROM assets WHERE file_path=? OR server_id=?)", rel, song)
		}
		if e == nil {
			_, e = tx.Exec("DELETE FROM assets WHERE file_path=? OR server_id=?", rel, song)
		}
		if e == nil {
			_, e = tx.Exec("UPDATE source_songs SET server_id='',deferred=1 WHERE server_id=?", song)
		}
		if e == nil {
			_, e = tx.Exec("UPDATE song_deletions SET phase='removed' WHERE request_id=?", rid)
		}
		if e == nil {
			if actor == "" {
				actor = owner
			}
			e = audit(tx, owner, actor, "library.song.removed", rid)
		}
		if e == nil {
			e = tx.Commit()
		}
		if e != nil {
			return e
		}
	}
	c, e := a.shared()
	if e != nil {
		return e
	}
	// Always request a scan after removal, including retries after a network failure.
	if _, e = a.nav(ctx, c, "startScan", nil); e != nil {
		return errors.New("Song removed; Navidrome scan failed. Retry to request the scan again")
	}
	return nil
}
