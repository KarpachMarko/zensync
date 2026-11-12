package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (a *App) StartWorker(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		slog.Info("Background worker started")
		if err := a.pruneDeletionTrash(); err != nil {
			slog.Error("Trash retention cleanup failed", "error", err)
		}
		if err := a.cleanupDeletionRestoreTemps(); err != nil {
			slog.Error("Restore temporary-file cleanup failed", "error", err)
		}
		if err := a.cleanupMetadataTemps(); err != nil {
			slog.Warn("Metadata temporary-file cleanup failed", "error", err)
		}
		defer slog.Info("Background worker stopped")
		cleanupErr := filepath.WalkDir(filepath.Join(a.dataDir, "staging"), func(path string, d os.DirEntry, e error) error {
			if e == nil && !d.IsDir() && d.Name() == "d-fi.config.json" {
				return os.Remove(path)
			}
			return nil
		})
		if cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			slog.Warn("Staging cleanup failed", "error", cleanupErr)
		}
		// Remote journals are reconciled on retry; never blindly replay an uncertain favorite.
		for _, recovery := range []struct{ name, query string }{
			// Old databases retain jobs.state DEFAULT 'blocked': CREATE TABLE
			// IF NOT EXISTS does not change that default during an upgrade.
			{"legacy_deletions_resumed", "UPDATE jobs SET state='queued' WHERE state='blocked' AND stage='Queued for library removal' AND request_id IN (SELECT d.request_id FROM song_deletions d JOIN requests r ON r.id=d.request_id WHERE r.decision='approved')"},
			{"migration_tasks_resumed", "UPDATE migration_tasks SET state='queued',message='Resuming after restart' WHERE state='running'"},
			{"interrupted_acquisitions", "UPDATE jobs SET state='failed',stage='Interrupted during acquisition. Retry to reconcile staged files and library placement.' WHERE state IN ('downloading','validating','importing')"},
			{"blocked_acquisitions_resumed", "UPDATE jobs SET state='queued',stage='Queued for acquisition' WHERE state='blocked' AND stage='Integration not configured'"},
			{"checking_acquisitions_resumed", "UPDATE jobs SET state='queued',stage='Queued after restart' WHERE state='checking'"},
		} {
			result, err := a.DB.Exec(recovery.query)
			if err != nil {
				slog.Error("Worker recovery failed", "step", recovery.name, "error", err)
				continue
			}
			if count, err := result.RowsAffected(); err == nil && count > 0 {
				slog.Info("Worker recovery applied", "step", recovery.name, "records", count)
			}
		}
		var work sync.WaitGroup
		defer work.Wait()
		maintenanceDone := make(chan struct{}, 1)
		maintenanceRunning := false
		lastMaintenance := time.Time{}
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-maintenanceDone:
				maintenanceRunning = false
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				if e := a.scheduleRelocations(); e != nil {
					slog.Error("legacy relocation scheduling failed")
				}
				var pendingRelocation int
				if e := a.DB.QueryRow("SELECT count(*) FROM asset_relocations WHERE state NOT IN ('completed','failed')").Scan(&pendingRelocation); e != nil {
					continue
				}
				if pendingRelocation > 0 {
					if a.activeAcquisitions() == 0 && !maintenanceRunning {
						a.runRelocation(ctx)
					}
					continue
				}
				if !maintenanceRunning && time.Since(lastMaintenance) >= 2*time.Second {
					maintenanceRunning = true
					lastMaintenance = time.Now()
					work.Add(1)
					go func() {
						defer work.Done()

						if err := a.pruneDeletionTrash(); err != nil {
							slog.Error("Trash retention cleanup failed", "error", err)
						}
						a.runDeletion(ctx)
						a.runMetadata(ctx)
						a.runMigrationTask(ctx)
						a.refreshAlbums()
						maintenanceDone <- struct{}{}
					}()
				}
				for a.activeAcquisitions() < a.downloadLimit() {
					claim := a.claimAcquisition()
					if claim == nil {
						break
					}
					work.Add(1)
					go func(c *acquisitionClaim) {
						defer work.Done()
						defer a.releaseAcquisition(c)
						a.executeAcquisition(ctx, c)
						a.refreshAlbums()
					}(claim)
				}
			}
		}
	}()
	return done
}
func (a *App) runMigrationTask(ctx context.Context) {
	if !a.migrationMu.TryLock() {
		return
	}
	defer a.migrationMu.Unlock()
	var tid, owner, mid, action string
	e := a.DB.QueryRow("SELECT id,owner,migration_id,action FROM migration_tasks WHERE state='queued' ORDER BY created,rowid LIMIT 1").Scan(&tid, &owner, &mid, &action)
	if e != nil {
		return
	}
	a.opMu.Lock()
	m, e := a.getMigration(mid, owner)
	if e != nil {
		a.opMu.Unlock()
		return
	}
	if m.State != "open" {
		_, _ = a.DB.Exec("UPDATE migration_tasks SET state='cancelled',message='Migration no longer open' WHERE id=?", tid)
		a.opMu.Unlock()
		return
	}
	claim, e := a.DB.Exec("UPDATE migration_tasks SET state='running',message=? WHERE id=? AND state='queued'", map[string]string{"match": "Matching existing library and Deezer recordings", "publish": "Verifying and publishing personal collections"}[action], tid)
	a.opMu.Unlock()
	if e != nil {
		return
	}
	if count, err := claim.RowsAffected(); err != nil || count != 1 {
		return
	}
	started := time.Now()
	logger := slog.With("task_id", tid, "migration_id", mid, "action", action, "user_id", owner)
	ctx = context.WithValue(ctx, diagnosticLoggerKey{}, logger)
	logger.Info("Migration task started")
	if action == "match" {
		e = a.matchMigration(ctx, owner, mid)
	} else {
		e = a.publishMigration(ctx, owner, mid)
	}
	if current, err := a.getMigration(mid, owner); err != nil || current.State != "open" {
		_, _ = a.DB.Exec("UPDATE migration_tasks SET state='cancelled',message='Migration no longer open' WHERE id=?", tid)
		return
	}
	if e != nil {
		logger.Warn("Migration task failed", "reason", e.Error(), "duration_ms", time.Since(started).Milliseconds())
	} else {
		logger.Info("Migration task finished", "duration_ms", time.Since(started).Milliseconds())
	}
	state, message := "completed", "Finished"
	if e != nil {
		state, message = "failed", e.Error()
	}
	if err := a.finishMigrationTask(tid, owner, mid, action, state, message); err != nil {
		slog.Error("migration task completion persistence failed", "task_id", tid)
		return
	}
	_, _ = a.DB.Exec("UPDATE migrations SET error=? WHERE id=?", func() string {
		if e != nil {
			return message
		}
		return ""
	}(), mid)
	if e == nil && action == "match" && m.TargetUser != "" {
		_, _ = a.queueMigration(owner, mid, "publish")
	}
}
func (a *App) retryTask(w http.ResponseWriter, r *http.Request, u User) {
	tid := strings.TrimPrefix(r.URL.Path, "/api/v2/tasks/")
	var relocationState string
	if e := a.DB.QueryRow("SELECT state FROM asset_relocations WHERE id=?", tid).Scan(&relocationState); e == nil {
		if u.Role != "admin" {
			fail(w, 404, "Task not found")
			return
		}
		a.opMu.Lock()
		defer a.opMu.Unlock()
		result, e := a.DB.Exec("UPDATE asset_relocations SET state=resume_state,checked_at=0,started_at=unixepoch(),message='Relocation retry requested' WHERE id=? AND state='failed'", tid)
		if e != nil {
			a.internal(w, e)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			fail(w, 409, "Only failed relocations can be retried")
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}

	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var owner, state string
	e = tx.QueryRow("SELECT owner,state FROM migration_tasks WHERE id=?", tid).Scan(&owner, &state)
	if e == nil {
		if owner != u.ID {
			fail(w, 404, "Task not found")
			return
		}
		if state != "failed" {
			fail(w, 409, "Only failed tasks can be retried")
			return
		}
		_, e = tx.Exec("UPDATE migration_tasks SET state='queued',message='' WHERE id=?", tid)
	} else if e == sql.ErrNoRows {
		e = tx.QueryRow("SELECT r.owner,j.state FROM jobs j JOIN requests r ON r.id=j.request_id WHERE j.id=?", tid).Scan(&owner, &state)
		if e != nil || (owner != u.ID && u.Role != "admin") {
			fail(w, 404, "Task not found")
			return
		}
		if state != "failed" && state != "blocked" {
			fail(w, 409, "Only blocked or failed tasks can be retried")
			return
		}
		var parent string
		if err := tx.QueryRow("SELECT a.request_id FROM album_requests a JOIN jobs j ON j.request_id=a.request_id WHERE j.id=?", tid).Scan(&parent); err == nil {
			var decision string
			if e = tx.QueryRow("SELECT decision FROM requests WHERE id=?", parent).Scan(&decision); e != nil {
				a.internal(w, e)
				return
			}
			if decision != "approved" {
				fail(w, 409, "The album must be approved before retrying")
				return
			}
			e = a.retryAlbum(tx, parent)
			if e != nil {
				a.internal(w, e)
				return
			}
		}
		if _, e = tx.Exec("DELETE FROM acquisition_checks WHERE job_id=?", tid); e != nil {
			a.internal(w, e)
			return
		}
		_, e = tx.Exec("DELETE FROM scan_waits WHERE job_id=?", tid)
		if e == nil {
			_, e = tx.Exec("DELETE FROM scan_schedule WHERE job_id=?", tid)
		}
		if e == nil {
			_, e = tx.Exec("UPDATE jobs SET state='queued',stage='Retry requested' WHERE id=?", tid)
		}
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		fail(w, 409, "A task is already active or cannot be retried")
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}
func (a *App) executeAcquisition(ctx context.Context, claim *acquisitionClaim) {
	jid, rid, owner, target, quality, reason, actor, upgrade := claim.jid, claim.rid, claim.owner, claim.target, claim.quality, claim.reason, claim.actor, claim.upgrade
	started := time.Now()
	logger := slog.With("job_id", jid, "acquisition_request_id", rid, "user_id", owner)
	ctx = context.WithValue(ctx, diagnosticLoggerKey{}, logger)
	logger.Info("Acquisition started")
	defer func() { logger.Info("Acquisition attempt finished", "duration_ms", time.Since(started).Milliseconds()) }()
	var e error
	// A legacy/stale queued claim may outlive a source replacement. Never fetch
	// the old provider target or fulfill its former source with different audio.
	var replaced bool
	if e = a.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM request_source_history WHERE request_id=?) AND NOT EXISTS(SELECT 1 FROM request_sources WHERE request_id=?)`, rid, rid).Scan(&replaced); e != nil {
		return
	}
	if replaced {
		var previous string
		_ = a.DB.QueryRow("SELECT stage FROM jobs WHERE id=?", jid).Scan(&previous)
		if !strings.HasPrefix(previous, "Replaced by ") {
			previous = "Replaced by a new recording choice for the imported song. Previous task: " + previous
		}
		a.jobState(jid, "superseded", previous)
		return
	}
	if upgrade {
		a.jobState(jid, "blocked", "Asset replacement is not enabled; existing audio is retained")
		return
	}
	var direct bool
	var role string
	if e = a.DB.QueryRow("SELECT role,direct FROM users WHERE id=?", owner).Scan(&role, &direct); e != nil {
		return
	}
	if reason == "Allowed without approval" && actor == "" && role != "admin" && !direct {
		var parent string
		if a.DB.QueryRow("SELECT parent_id FROM request_children WHERE child_id=?", rid).Scan(&parent) == nil {
			tx, e := a.DB.Begin()
			if e != nil {
				return
			}
			defer tx.Rollback()
			if _, e = tx.Exec("UPDATE requests SET decision='awaiting_approval',reason='Permission changed; album approval required' WHERE id=? OR id IN (SELECT child_id FROM request_children WHERE parent_id=?)", parent, parent); e != nil {
				return
			}
			if _, e = tx.Exec("UPDATE jobs SET state='blocked',stage='Album requires administrator approval after permission change' WHERE request_id=?", parent); e != nil {
				return
			}
			if e = tx.Commit(); e != nil {
				return
			}
			return
		}
		_, _ = a.DB.Exec("UPDATE requests SET decision='awaiting_approval',reason='Permission changed; administrator approval required' WHERE id=?", rid)
		a.jobState(jid, "blocked", "Awaiting approval after permission change")
		return
	}
	var linked, active int
	if e = a.DB.QueryRow("SELECT count(*),COALESCE(sum(CASE WHEN s.deferred=0 THEN 1 ELSE 0 END),0) FROM request_sources rs JOIN source_songs s ON s.id=rs.source_id WHERE rs.request_id=?", rid).Scan(&linked, &active); e != nil {
		return
	}
	// Existing verified assets can satisfy deferred intents without acquiring again.
	var existing string
	e = a.DB.QueryRow("SELECT server_id FROM assets WHERE target=? AND server_id<>''", target).Scan(&existing)
	if e == nil {
		c, err := a.shared()
		if err == nil {
			song, err := a.nav(ctx, c, "getSong", mapValues("id", existing))
			if err == nil && song.Song.ID == existing {
				a.fulfill(jid, rid, target, existing)
				return
			}
		}
	}
	if linked > 0 && active == 0 {
		a.jobState(jid, "deferred", "Skipped for now; no new acquisition will start")
		return
	}
	c, e := a.shared()
	if e != nil {
		a.jobState(jid, "blocked", e.Error())
		return
	}
	var file, actual, metadata string
	e = a.DB.QueryRow("SELECT file_path,quality,metadata FROM assets WHERE target=?", target).Scan(&file, &actual, &metadata)
	if e == sql.ErrNoRows && strings.HasPrefix(target, "upload:") {
		file, actual, metadata, e = a.placeUploadedAudio(jid, rid, target)
		if e != nil {
			a.jobState(jid, "failed", e.Error())
			return
		}
	} else if e == sql.ErrNoRows {
		t, err := a.track(ctx, target)
		if err != nil {
			a.jobState(jid, "failed", err.Error())
			return
		}
		var approvedJSON string
		if a.DB.QueryRow("SELECT metadata FROM request_metadata WHERE request_id=?", rid).Scan(&approvedJSON) != nil {
			a.jobState(jid, "failed", "This legacy request has no verified recording snapshot. Submit it again from Search.")
			return
		}
		var approved Track
		if json.Unmarshal([]byte(approvedJSON), &approved) != nil || !sameApprovedRecording(approved, t) {
			a.jobState(jid, "failed", "Provider metadata changed since approval; review and submit a fresh request")
			return
		}
		// Reuse an exact existing recording/version without quality-driven upgrades.
		songs, err := a.navSearch(ctx, c, t.Title, 0)
		if err != nil {
			a.jobState(jid, "failed", err.Error())
			return
		}
		matches := []Song{}
		for _, s := range songs {
			if trackMatches(t, s) {
				matches = append(matches, s)
			}
		}
		if len(matches) == 1 {
			a.fulfill(jid, rid, target, matches[0].ID)
			return
		}
		provider, err := a.mediaProvider(t.Provider)
		if err == nil {
			err = provider.Ready()
		}
		if err != nil {
			a.jobState(jid, "blocked", err.Error())
			return
		}
		attempt := id()
		_, e = a.DB.Exec("INSERT INTO attempts(id,job_id,stage,outcome) VALUES(?,?,'acquisition','running')", attempt, jid)
		if e != nil {
			return
		}
		file, actual, e = a.acquireProvider(ctx, jid, target, t, quality)
		outcome := "completed"
		if e != nil {
			outcome = "failed"
		}
		_, _ = a.DB.Exec("UPDATE attempts SET outcome=? WHERE id=?", outcome, attempt)
		if e != nil {
			a.jobState(jid, "failed", e.Error())
			return
		}
		metadata = mustJSON(t)
	} else if e != nil {
		a.jobState(jid, "failed", "Unable to read asset journal")
		return
	}
	if file == "" {
		a.jobState(jid, "failed", "Asset journal has no path")
		return
	}
	if e = a.verifyPlaced(file, metadata, actual); e != nil {
		a.jobState(jid, "failed", e.Error())
		return
	}
	if _, e = a.DB.Exec("INSERT OR IGNORE INTO scan_waits(job_id,started_at) VALUES(?,unixepoch())", jid); e != nil {
		a.jobState(jid, "failed", "Cannot persist indexing wait")
		return
	}
	a.jobState(jid, "awaiting_scan", "File validated; waiting for Navidrome indexing ("+actual+")")
	if _, e = a.DB.Exec("INSERT INTO scan_schedule(job_id,checked_at) VALUES(?,unixepoch()) ON CONFLICT(job_id) DO UPDATE SET checked_at=excluded.checked_at", jid); e != nil {
		return
	}
	scan, scanErr := a.scanLibrary(ctx, c)
	if scanErr != nil {
		a.jobState(jid, "failed", scanErr.Error())
		return
	}
	slog.Info("Navidrome indexing check", "job_id", jid, "scanning", scan.Scan.Scanning, "scanned_count", scan.Scan.Count, "file_path", file)
	songID, lookup, e := a.findPlacedWithReport(ctx, c, file, metadata, false)
	if e != nil {
		a.jobState(jid, "failed", e.Error())
		return
	}
	if songID == "" {
		var elapsed int64
		if e = a.DB.QueryRow("SELECT unixepoch()-started_at FROM scan_waits WHERE job_id=?", jid).Scan(&elapsed); e != nil {
			a.jobState(jid, "failed", "Cannot read indexing wait")
			return
		}
		slog.Warn("Placed file not found in Navidrome", "job_id", jid, "file_path", file, "wait_seconds", elapsed, "scanning", scan.Scan.Scanning)
		if elapsed >= 300 {
			a.jobState(jid, "failed", "Could not verify the downloaded file in Navidrome after 5 minutes. "+lookup.reason()+". Retained file: "+file+". Retry checks this file again without downloading it again.")
		} else {
			a.jobState(jid, "awaiting_scan", lookup.reason()+". The validated file is retained; checking again shortly.")
		}
		return
	}
	if _, e = a.DB.Exec("UPDATE assets SET server_id=? WHERE target=?", songID, target); e != nil {
		return
	}
	a.fulfill(jid, rid, target, songID)
}
func (a *App) jobState(jid, state, message string) {
	if state != "downloading" {
		a.clearTransfer(jid)
	}
	var previous, previousMessage string
	if e := a.DB.QueryRow("SELECT state,stage FROM jobs WHERE id=?", jid).Scan(&previous, &previousMessage); e != nil {
		slog.Error("job state lookup failed", "job_id", jid)
		return
	}
	if _, e := a.DB.Exec("UPDATE jobs SET state=?,stage=? WHERE id=?", state, message, jid); e != nil {
		slog.Error("job state persistence failed", "job_id", jid)
		return
	}
	if previous != state || previousMessage != message {
		slog.Info("acquisition state changed", "job_id", jid, "from", previous, "state", state, "detail", message)
	}
}
func (a *App) fulfill(jid, rid, target, songID string) {
	tx, e := a.DB.Begin()
	if e != nil {
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE song_deletions SET phase='reacquired' WHERE song_id=? AND phase='removed'", songID); e != nil {
		return
	}
	if _, e = tx.Exec("UPDATE jobs SET state='available',stage='Verified in Navidrome' WHERE id=?", jid); e != nil {
		return
	}
	if _, e = tx.Exec("UPDATE source_songs SET server_id=? WHERE id IN (SELECT source_id FROM request_sources WHERE request_id=?)", songID, rid); e != nil {
		return
	}
	provider, source, number := "deezer", "", strings.TrimPrefix(target, "deezer:track:")
	if strings.HasPrefix(target, "lossless:track:") {
		parts := strings.Split(target, ":")
		if len(parts) == 4 {
			provider, source, number = "lossless", parts[2], parts[3]
		}
	}
	providerID, _ := strconv.ParseInt(number, 10, 64)
	if providerID > 0 {
		if _, e = tx.Exec(`UPDATE source_songs SET server_id=? WHERE candidate<>'' AND json_extract(candidate,'$.id')=? AND COALESCE(NULLIF(json_extract(candidate,'$.provider'),''),'deezer')=? AND COALESCE(json_extract(candidate,'$.source'),'')=? AND match_state IN ('confident','manual') AND server_id=''`, songID, providerID, provider, source); e != nil {
			return
		}
	}

	if e = tx.Commit(); e != nil {
		return
	}
	_ = os.RemoveAll(filepath.Join(a.dataDir, "staging", jid))
	a.cleanAudioUpload(target)
	rows, e := a.DB.Query("SELECT DISTINCT m.owner,m.id,m.target_user,m.state FROM source_songs s JOIN migrations m ON m.id=s.migration_id WHERE s.server_id=?", songID)
	if e != nil {
		return
	}
	type dest struct{ owner, id, target, state string }
	destinations := []dest{}
	for rows.Next() {
		var d dest
		if rows.Scan(&d.owner, &d.id, &d.target, &d.state) == nil {
			destinations = append(destinations, d)
		}
	}
	rows.Close()
	for _, d := range destinations {
		if d.state == "open" && d.target != "" {
			_, _ = a.queueMigration(d.owner, d.id, "publish")
		}
	}
}
