package app

import (
	"database/sql"
	"net/http"
	"strings"
)

// Current links drive fulfillment. Keep old links separately so retrying an
// obsolete request cannot overwrite the user's replacement recording.
func archiveSourceRequests(tx *sql.Tx, sid string) error {
	if _, err := tx.Exec("INSERT OR IGNORE INTO request_source_history(source_id,request_id) SELECT source_id,request_id FROM request_sources WHERE source_id=?", sid); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM request_sources WHERE source_id=?", sid); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE jobs SET state='superseded',stage='Replaced by a new recording choice for the imported song. Previous task: '||stage
 WHERE state IN ('failed','blocked','queued','deferred') AND request_id IN (SELECT request_id FROM request_source_history WHERE source_id=?)
 AND NOT EXISTS(SELECT 1 FROM request_sources current WHERE current.request_id=jobs.request_id)`, sid)
	return err
}

func (a *App) dismissAcquisitionFailure(w http.ResponseWriter, r *http.Request, u User) {
	jid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/tasks/"), "/dismiss")
	a.workMu.Lock()
	defer a.workMu.Unlock()
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	var owner, state, stage, target string
	err = tx.QueryRow("SELECT r.owner,j.state,j.stage,r.target FROM jobs j JOIN requests r ON r.id=j.request_id WHERE j.id=?", jid).Scan(&owner, &state, &stage, &target)
	if err != nil || (u.ID != owner && u.Role != "admin") || !targetPattern.MatchString(target) {
		fail(w, 404, "Task not found")
		return
	}
	if state == "cancelled" && strings.HasPrefix(stage, "Dismissed failure.") {
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if _, active := a.activeJobs[jid]; active || state != "failed" {
		fail(w, 409, "Only an idle failed acquisition can be dismissed")
		return
	}
	_, err = tx.Exec("UPDATE jobs SET state='cancelled',stage='Dismissed failure. Previous failure: '||stage WHERE id=?", jid)
	if err == nil {
		err = audit(tx, owner, u.ID, "request.failure_dismissed", jid)
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
