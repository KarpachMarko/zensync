package app

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) restoreDeletedSong(w http.ResponseWriter, r *http.Request, u User) {
	if u.Role != "admin" {
		fail(w, 403, "Administrator required to restore a shared song")
		return
	}
	jid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/tasks/"), "/restore")
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.workMu.Lock()
	defer a.workMu.Unlock()
	if len(a.activeJobs) > 0 {
		fail(w, 409, "Wait for active downloads before restoring")
		return
	}
	var rid, rel, checksum, state, phase string
	err := a.DB.QueryRow(`SELECT d.request_id,d.file_path,d.checksum,j.state,d.phase FROM song_deletions d JOIN jobs j ON j.request_id=d.request_id WHERE j.id=?`, jid).Scan(&rid, &rel, &checksum, &state, &phase)
	if err != nil {
		fail(w, 404, "Deleted song not found")
		return
	}
	if state != "completed" || (phase != "removed" && phase != "restored") {
		fail(w, 409, "Only completed song deletions can be restored")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	var restorePhase, server string
	err = a.DB.QueryRow("SELECT phase,server_url FROM deletion_restores WHERE request_id=?", rid).Scan(&restorePhase, &server)
	if err != nil && err != sql.ErrNoRows {
		a.internal(w, err)
		return
	}
	if restorePhase != "" && server != c.URL {
		fail(w, 409, "Navidrome connection changed since restoration started")
		return
	}
	if restorePhase == "completed" {
		send(w, 200, map[string]bool{"restored": true})
		return
	}
	var boundServer string
	err = a.DB.QueryRow("SELECT server_url FROM deletion_replacements WHERE request_id=?", rid).Scan(&boundServer)
	if err != nil && err != sql.ErrNoRows {
		a.internal(w, err)
		return
	}
	if boundServer != "" && boundServer != c.URL {
		fail(w, 409, "Navidrome connection changed since deletion")
		return
	}
	if !safeRelativePath(rel) {
		fail(w, 409, "Invalid original music path")
		return
	}
	info, err := os.Lstat(a.musicDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		fail(w, 409, "Music directory unavailable or unsafe")
		return
	}
	root, err := os.OpenRoot(a.musicDir)
	if err != nil {
		fail(w, 409, "Music directory unavailable")
		return
	}
	defer root.Close()
	// Create only ordinary directories within the mounted music root.
	parent := ""
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
		if part == "." {
			continue
		}
		parent = filepath.Join(parent, part)
		if err = root.Mkdir(parent, 0755); err != nil && !os.IsExist(err) {
			fail(w, 409, "Cannot create original directory")
			return
		}
		info, err := root.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			fail(w, 409, "Unsafe original directory")
			return
		}
	}
	if err = a.checkMusicParents(rel); err != nil {
		fail(w, 409, err.Error())
		return
	}
	tmp := filepath.Join(filepath.Dir(rel), ".zensync-restore-"+rid+filepath.Ext(rel))
	if restorePhase != "" {
		if info, err := root.Lstat(tmp); err == nil && info.Mode().IsRegular() {
			if err = root.Remove(tmp); err != nil {
				fail(w, 409, "Cannot clear interrupted restoration")
				return
			}
		}
	}
	destination := filepath.Join(a.musicDir, rel)
	if existing, statErr := root.Lstat(rel); statErr == nil {
		if !existing.Mode().IsRegular() {
			fail(w, 409, "Original path is unsafe; no file overwritten")
			return
		}
		hash, hashErr := hashFile(destination)
		if restorePhase == "" || hashErr != nil || hash != checksum {
			fail(w, 409, "Original path is occupied; no file overwritten")
			return
		}
	} else if !os.IsNotExist(statErr) {
		fail(w, 409, "Cannot inspect original path")
		return
	} else {
		if !a.availableDeletionCopy(rid) {
			fail(w, 410, "Recovery copy expired or was removed by the 500 MB retention limit")
			return
		}
		backup, err := a.deletionTrashPath(rid)
		if err != nil {
			fail(w, 410, "Recovery copy unavailable")
			return
		}
		if hash, err := hashFile(backup); err != nil || hash != checksum {
			fail(w, 409, "Recovery copy verification failed")
			return
		}
		if _, err = a.DB.Exec("INSERT INTO deletion_restores(request_id,server_url) VALUES(?,?) ON CONFLICT(request_id) DO NOTHING", rid, c.URL); err != nil {
			a.internal(w, err)
			return
		}
		output, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			fail(w, 409, "Cannot prepare restored file")
			return
		}
		defer root.Remove(tmp)
		input, err := os.Open(backup)
		if err == nil {
			_, err = io.Copy(output, input)
			input.Close()
		}
		if err == nil {
			err = output.Sync()
		}
		closeErr := output.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			var hash string
			hash, err = hashFile(filepath.Join(a.musicDir, tmp))
			if hash != checksum {
				err = errors.New("Restored file checksum differs")
			}
		}
		if err != nil {
			fail(w, 409, "Could not prepare verified restoration; original path untouched")
			return
		}
		// Link publishes without overwriting anything that appeared during copying.
		if err = root.Link(tmp, rel); err != nil {
			fail(w, 409, "Cannot restore without overwriting an existing file")
			return
		}
		if err = syncDir(filepath.Dir(destination)); err != nil {
			a.internal(w, err)
			return
		}
	}
	if _, err = a.DB.Exec("UPDATE deletion_restores SET phase='published' WHERE request_id=?", rid); err != nil {
		a.internal(w, err)
		return
	}
	if _, err = a.nav(r.Context(), c, "startScan", nil); err != nil {
		fail(w, 502, "File restored; scan failed. Retry Restore to request the scan again")
		return
	}
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE deletion_restores SET phase='completed' WHERE request_id=?", rid); err == nil {
		_, err = tx.Exec("UPDATE song_deletions SET phase='restored' WHERE request_id=?", rid)
	}
	if err == nil {
		_, err = tx.Exec("UPDATE jobs SET stage='Deleted file restored; playlist replacements unchanged' WHERE request_id=?", rid)
	}
	if err == nil {
		err = audit(tx, u.ID, u.ID, "library.song.restored", rid)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if backup, err := a.deletionTrashPath(rid); err == nil {
		_ = os.Remove(backup)
		_ = os.Remove(filepath.Dir(backup))
	}
	a.navIndexMu.Lock()
	clear(a.navIndexes)
	a.navIndexMu.Unlock()
	send(w, 200, map[string]bool{"restored": true})
}

// Remove only known restore working files after a process interruption.
func (a *App) cleanupDeletionRestoreTemps() error {
	rows, err := a.DB.Query("SELECT x.request_id,d.file_path FROM deletion_restores x JOIN song_deletions d ON d.request_id=x.request_id")
	if err != nil {
		return err
	}
	paths := []string{}
	for rows.Next() {
		var rid, rel string
		if err = rows.Scan(&rid, &rel); err != nil {
			rows.Close()
			return err
		}
		if safeRelativePath(rel) && len(rid) == 64 && strings.Trim(rid, "0123456789abcdef") == "" {
			paths = append(paths, filepath.Join(filepath.Dir(rel), ".zensync-restore-"+rid+filepath.Ext(rel)))
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	root, err := os.OpenRoot(a.musicDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	for _, rel := range paths {
		if a.checkMusicParents(rel) != nil {
			continue
		}
		if info, err := root.Lstat(rel); err == nil && info.Mode().IsRegular() {
			if err = root.Remove(rel); err != nil {
				return err
			}
		}
	}
	return nil
}
