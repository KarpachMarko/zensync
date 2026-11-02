package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var legacyAssetPath = regexp.MustCompile(`^ZenSync/[1-9][0-9]*/([0-9a-f]{64})\.(flac|mp3)$`)
var relocationVersion = regexp.MustCompile(`^0\.64\.([0-9]+)(?:\s|$)`)

type relocation struct {
	id, target, owner, old, new, hash, server, state, message, metadata, quality string
	started                                                                      int64
}

func (a *App) scheduleRelocations() error {
	_, e := a.DB.Exec(`INSERT OR IGNORE INTO asset_relocations(id,target,owner,old_path)
 SELECT lower(hex(randomblob(32))),target,(SELECT id FROM users WHERE role='admin' ORDER BY created,id LIMIT 1),file_path
 FROM assets WHERE file_path LIKE 'ZenSync/%' AND EXISTS(SELECT 1 FROM users WHERE role='admin')`)
	return e
}
func (a *App) relocationState(r relocation, state, message string) {
	if _, e := a.DB.Exec("UPDATE asset_relocations SET state=?,message=?,checked_at=unixepoch() WHERE id=?", state, message, r.id); e != nil {
		slog.Error("relocation state persistence failed", "relocation_id", r.id)
		return
	}
	slog.Info("legacy asset relocation", "relocation_id", r.id, "state", state, "old_path", r.old, "new_path", r.new, "detail", message)
}
func (a *App) failRelocation(r relocation, e error) {
	_, _ = a.DB.Exec("UPDATE asset_relocations SET resume_state=? WHERE id=?", r.state, r.id)
	a.relocationState(r, "failed", e.Error())
}

// Relocation owns the worker while a moved file is awaiting reconciliation. This
// prevents acquisitions/collection writers from acting on transitional mappings.
func (a *App) runRelocation(ctx context.Context) bool {
	var r relocation
	e := a.DB.QueryRow(`SELECT x.id,x.target,x.owner,x.old_path,x.new_path,x.checksum,x.server_id,x.state,x.message,x.started_at,a.metadata,a.quality FROM asset_relocations x JOIN assets a ON a.target=x.target WHERE x.state NOT IN ('completed','failed') ORDER BY CASE WHEN x.state='queued' THEN 1 ELSE 0 END,x.rowid LIMIT 1`).Scan(&r.id, &r.target, &r.owner, &r.old, &r.new, &r.hash, &r.server, &r.state, &r.message, &r.started, &r.metadata, &r.quality)
	if e == sql.ErrNoRows {
		return false
	}
	if e != nil {
		slog.Error("relocation lookup failed")
		return false
	}
	var checked int64
	_ = a.DB.QueryRow("SELECT checked_at FROM asset_relocations WHERE id=?", r.id).Scan(&checked)
	var now int64
	_ = a.DB.QueryRow("SELECT unixepoch()").Scan(&now)
	if checked > now-15 {
		return r.state != "queued"
	}
	a.opMu.Lock()
	defer a.opMu.Unlock()
	if e = a.processRelocation(ctx, &r, now); e != nil {
		a.failRelocation(r, e)
	}
	return true
}
func (a *App) processRelocation(ctx context.Context, r *relocation, now int64) error {
	match := legacyAssetPath.FindStringSubmatch(r.old)
	if match == nil {
		return errors.New("Only tracked legacy ZenSync ID/hash paths can be reorganized")
	}
	c, e := a.shared()
	if e != nil {
		return e
	}
	server, e := a.nav(ctx, c, "ping", nil)
	if e != nil {
		return e
	}
	version := relocationVersion.FindStringSubmatch(server.ServerVersion)
	if server.Type != "navidrome" || version == nil {
		return errors.New("Automatic relocation requires verified Navidrome 0.64.2–0.64.x rename support; the file was retained")
	}
	patch, _ := strconv.Atoi(version[1])
	if patch < 2 {
		return errors.New("Update Navidrome to 0.64.2 or later in the 0.64 series before reorganizing files")
	}
	scan, e := a.nav(ctx, c, "getScanStatus", nil)
	if e != nil {
		return e
	}
	if scan.Scan.Scanning {
		a.relocationState(*r, r.state, "Waiting for the current Navidrome scan")
		return nil
	}
	if e = a.checkMusicParents(r.old); e != nil {
		return e
	}
	old := filepath.Join(a.musicDir, filepath.FromSlash(r.old))
	if r.state == "queued" {
		if e = a.verifyPlaced(r.old, r.metadata, r.quality); e != nil {
			return e
		}
		hash, e := hashFile(old)
		if e != nil {
			return e
		}
		if hash != match[1] {
			return errors.New("Legacy file checksum differs from its recorded name; no file was moved")
		}
		song, e := a.findPlaced(ctx, c, r.old, r.metadata, true)
		if e != nil {
			return e
		}
		if song == "" {
			return errors.New("The legacy file must be indexed in Navidrome before it can be safely relocated; scan the library and retry")
		}
		var track Track
		if e = json.Unmarshal([]byte(r.metadata), &track); e != nil {
			return e
		}
		dest, e := a.libraryPath(old, track, hash)
		if e != nil {
			return e
		}
		if _, e = os.Lstat(filepath.Join(a.musicDir, dest)); e == nil {
			return errors.New("Readable destination already exists; existing files were preserved")
		}
		if !os.IsNotExist(e) {
			return e
		}
		r.new = filepath.ToSlash(dest)
		r.hash = hash
		r.server = song
		if _, e = a.DB.Exec("UPDATE asset_relocations SET new_path=?,checksum=?,server_id=?,state='planned',resume_state='planned',message='Backing up and relocating validated audio' WHERE id=?", r.new, r.hash, r.server, r.id); e != nil {
			return e
		}
		r.state = "planned"
	}
	if r.state == "planned" {
		if !safeRelativePath(r.new) || strings.HasPrefix(r.new, "ZenSync/") {
			return errors.New("Invalid readable destination in relocation journal")
		}
		dest := filepath.Join(a.musicDir, filepath.FromSlash(r.new))
		if info, err := os.Lstat(old); err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("Legacy source is not a regular file")
			}
			hash, e := hashFile(old)
			if e != nil || hash != r.hash {
				return errors.New("Legacy source changed before relocation")
			}
			if e = a.backupRelocation(*r, old); e != nil {
				return e
			}
			if e = a.ensureMusicDirs(filepath.Dir(r.new)); e != nil {
				return e
			}
			if e = renameNoReplace(old, dest); e != nil {
				return fmt.Errorf("Cannot safely rename legacy file without overwriting a destination: %w", e)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		// A restart after rename but before the database commit resumes from here.
		if e = a.verifyRelocationDestination(*r); e != nil {
			return e
		}
		if e = syncDir(filepath.Dir(old)); e != nil {
			return e
		}
		if e = syncDir(filepath.Dir(dest)); e != nil {
			return e
		}
		tx, e := a.DB.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if _, e = tx.Exec("UPDATE assets SET file_path=? WHERE target=? AND file_path IN (?,?)", r.new, r.target, r.old, r.new); e != nil {
			return e
		}
		if _, e = tx.Exec("UPDATE asset_relocations SET state='awaiting_scan',resume_state='awaiting_scan',started_at=unixepoch(),checked_at=0,message='Readable file placed; verifying unchanged Navidrome song identity' WHERE id=?", r.id); e != nil {
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		r.state = "awaiting_scan"
		r.started = now
		if _, e = a.nav(ctx, c, "startScan", nil); e != nil {
			return e
		}
		a.relocationState(*r, r.state, "Readable file placed; waiting for Navidrome scan")
		return nil
	}
	if r.state == "awaiting_scan" {
		if _, err := os.Lstat(filepath.Join(a.musicDir, r.new)); os.IsNotExist(err) {
			if hash, err := hashFile(old); err == nil && hash == r.hash {
				if _, e = a.DB.Exec("UPDATE assets SET file_path=? WHERE target=?", r.old, r.target); e != nil {
					return e
				}
				r.state = "queued"
				return errors.New("Recovered the original path after an interrupted rollback; review and retry relocation")
			}
		}
		if e = a.verifyRelocationDestination(*r); e != nil {
			return e
		}
		out, e := a.nav(ctx, c, "getSong", mapValues("id", r.server))
		same := false
		if e == nil && out.Song.ID == r.server {
			same, e = a.sameRemoteAudio(ctx, c, out.Song, r.new)
		}
		if e == nil && same {
			tx, e := a.DB.Begin()
			if e != nil {
				return e
			}
			defer tx.Rollback()
			if _, e = tx.Exec("UPDATE assets SET server_id=? WHERE target=?", r.server, r.target); e != nil {
				return e
			}
			if _, e = tx.Exec("UPDATE asset_relocations SET state='completed',message=? WHERE id=?", "Organized as "+r.new+"; Navidrome song ID preserved", r.id); e != nil {
				return e
			}
			if e = audit(tx, r.owner, r.owner, "asset.relocated", r.target); e != nil {
				return e
			}
			if e = tx.Commit(); e != nil {
				return e
			}
			slog.Info("legacy asset relocated", "target", r.target, "old_path", r.old, "new_path", r.new, "song_id", r.server)
			// Only remove empty legacy directories; never traverse/delete unrelated music.
			_ = os.Remove(filepath.Dir(old))
			_ = os.Remove(filepath.Join(a.musicDir, "ZenSync"))
			return nil
		}
		if now-r.started >= 300 {
			// Do not remap a changed song ID: private collections may reference the old ID.
			if e = renameNoReplace(filepath.Join(a.musicDir, r.new), old); e != nil {
				return errors.New("Could not verify Navidrome identity and could not restore the old path; backup retained for recovery")
			}
			_, e = a.DB.Exec("UPDATE assets SET file_path=? WHERE target=?", r.old, r.target)
			if e != nil {
				return e
			}
			_, _ = a.nav(ctx, c, "startScan", nil)
			r.state = "queued"
			return errors.New("Navidrome did not preserve the verified song identity; original path restored. Backup retained; review the server before retrying")
		}
		// Start/retry a scan after restart or an uncertain startScan outcome.
		if _, e = a.nav(ctx, c, "startScan", nil); e != nil {
			return e
		}
		a.relocationState(*r, r.state, "Waiting for the original song ID at its readable location")
		return nil
	}
	return nil
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (a *App) verifyRelocationDestination(r relocation) error {
	if !safeRelativePath(r.new) {
		return errors.New("Invalid destination path")
	}
	if e := a.checkMusicParents(r.new); e != nil {
		return e
	}
	path := filepath.Join(a.musicDir, r.new)
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		return errors.New("Relocated audio is missing or unsafe; backup retained")
	}
	hash, e := hashFile(path)
	if e != nil || hash != r.hash {
		return errors.New("Relocated audio checksum mismatch; backup retained")
	}
	return nil
}
func (a *App) backupRelocation(r relocation, old string) error {
	dir := filepath.Join(a.dataDir, "relocation-backups")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	backup := filepath.Join(dir, r.id+filepath.Ext(old))
	if hash, e := hashFile(backup); e == nil {
		if hash == r.hash {
			return nil
		}
		return errors.New("Relocation backup checksum mismatch; original file retained")
	}
	info, e := os.Stat(old)
	if e != nil {
		return e
	}
	if e = diskHeadroom(dir, uint64(info.Size())+(64<<20)); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".backup-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	f, e := os.Open(old)
	if e != nil {
		return e
	}
	_, e = io.Copy(tmp, f)
	f.Close()
	if e != nil {
		return e
	}
	if e = tmp.Sync(); e != nil {
		return e
	}
	hash, e := hashFile(tmp.Name())
	if e != nil || hash != r.hash {
		return errors.New("Could not verify relocation backup")
	}
	if e = os.Link(tmp.Name(), backup); e != nil {
		return e
	}
	return syncDir(dir)
}

func (a *App) checkMusicParents(rel string) error {
	if !safeRelativePath(rel) {
		return errors.New("Invalid music path")
	}
	parts := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")
	current := a.musicDir
	for i := 0; i <= len(parts); i++ {
		info, e := os.Lstat(current)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Music path has a missing or unsafe directory")
		}
		if i < len(parts) {
			current = filepath.Join(current, parts[i])
		}
	}
	return nil
}
