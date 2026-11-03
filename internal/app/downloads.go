package app

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const defaultDownloads = 3

func (a *App) downloadLimit() int {
	var value string
	if a.DB.QueryRow("SELECT value FROM settings WHERE name='parallel_downloads'").Scan(&value) != nil {
		return defaultDownloads
	}
	n, e := strconv.Atoi(value)
	if e != nil || n < 1 {
		return defaultDownloads
	}
	return n
}
func (a *App) downloadSettings(w http.ResponseWriter, r *http.Request, u User) {
	if u.Role != "admin" {
		fail(w, 403, "Administrator required")
		return
	}
	if r.Method == "GET" {
		send(w, 200, map[string]int{"parallelDownloads": a.downloadLimit()})
		return
	}
	if r.Method != "PUT" {
		fail(w, 405, "Method not allowed")
		return
	}
	var in struct {
		ParallelDownloads int `json:"parallelDownloads"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ParallelDownloads < 1 {
		fail(w, 400, "Parallel downloads must be a positive whole number")
		return
	}
	_, e := a.DB.Exec("INSERT INTO settings(name,value) VALUES('parallel_downloads',?) ON CONFLICT(name) DO UPDATE SET value=excluded.value", strconv.Itoa(in.ParallelDownloads))
	if e != nil {
		a.internal(w, e)
		return
	}
	slog.Info("download concurrency changed", "limit", in.ParallelDownloads, "actor", u.ID)
	send(w, 200, in)
}

type acquisitionClaim struct {
	jid, rid, owner, target, quality, reason, actor, state string
	upgrade                                                bool
}

func (a *App) claimAcquisition() *acquisitionClaim {
	a.workMu.Lock()
	defer a.workMu.Unlock()
	var deleting int
	if a.DB.QueryRow("SELECT count(*) FROM song_deletions d JOIN jobs j ON j.request_id=d.request_id JOIN requests r ON r.id=d.request_id WHERE j.state='queued' AND r.decision='approved'").Scan(&deleting) != nil || deleting > 0 {
		return nil
	}
	if a.activeJobs == nil {
		a.activeJobs = map[string]string{}
		a.activeTargets = map[string]bool{}
		a.activeOwners = map[string]int{}
	}
	rows, e := a.DB.Query(`SELECT j.id,r.id,r.owner,r.target,r.quality,r.reason,r.upgrade,COALESCE(r.actor,''),j.state FROM jobs j JOIN requests r ON r.id=j.request_id LEFT JOIN scan_schedule sc ON sc.job_id=j.id LEFT JOIN acquisition_checks ck ON ck.job_id=j.id WHERE NOT EXISTS(SELECT 1 FROM song_deletions d WHERE d.request_id=r.id) AND r.decision='approved' AND r.target NOT LIKE 'deezer:album:%' AND NOT EXISTS(SELECT 1 FROM request_children cc JOIN requests pp ON pp.id=cc.parent_id WHERE cc.child_id=r.id AND pp.decision<>'approved') AND j.state IN ('queued','blocked','awaiting_scan','deferred') AND (j.state<>'awaiting_scan' OR COALESCE(sc.checked_at,0)<unixepoch()-30) AND (j.state NOT IN ('blocked','deferred') OR COALESCE(ck.checked_at,0)<unixepoch()-15) ORDER BY j.rowid`)
	if e != nil {
		return nil
	}
	var chosen *acquisitionClaim
	bestScore := int(^uint(0) >> 1)
	for rows.Next() {
		var c acquisitionClaim
		if e = rows.Scan(&c.jid, &c.rid, &c.owner, &c.target, &c.quality, &c.reason, &c.upgrade, &c.actor, &c.state); e != nil {
			rows.Close()
			return nil
		}
		if _, busy := a.activeJobs[c.jid]; !busy && !a.activeTargets[c.target] {
			score := a.activeOwners[c.owner]
			if chosen == nil || score < bestScore {
				copy := c
				chosen = &copy
				bestScore = score
			}
			if score == 0 {
				break
			}
		}
	}
	rows.Close()
	if chosen == nil {
		return nil
	}
	if _, e = a.DB.Exec("INSERT INTO acquisition_checks(job_id,checked_at) VALUES(?,unixepoch()) ON CONFLICT(job_id) DO UPDATE SET checked_at=excluded.checked_at", chosen.jid); e != nil {
		return nil
	}
	if chosen.state != "awaiting_scan" {
		result, e := a.DB.Exec("UPDATE jobs SET state='checking',stage='Checking library availability and acquisition permissions' WHERE id=? AND state=?", chosen.jid, chosen.state)
		if e != nil {
			return nil
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return nil
		}
	}
	a.activeJobs[chosen.jid] = chosen.target
	a.activeTargets[chosen.target] = true
	a.activeOwners[chosen.owner]++
	return chosen
}
func (a *App) releaseAcquisition(c *acquisitionClaim) {
	a.workMu.Lock()
	defer a.workMu.Unlock()
	delete(a.activeJobs, c.jid)
	delete(a.activeTargets, c.target)
	a.activeOwners[c.owner]--
	_, _ = a.DB.Exec("UPDATE jobs SET rowid=(SELECT COALESCE(max(rowid),0)+1 FROM jobs) WHERE id=?", c.jid)
}
func (a *App) activeAcquisitions() int {
	a.workMu.Lock()
	defer a.workMu.Unlock()
	return len(a.activeJobs)
}
func (a *App) runAcquisition(ctx context.Context) {
	c := a.claimAcquisition()
	if c == nil {
		return
	}
	defer a.releaseAcquisition(c)
	a.executeAcquisition(ctx, c)
}
func (a *App) scanLibrary(ctx context.Context, c Credentials) (NavResponse, error) {
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	scan, e := a.nav(ctx, c, "getScanStatus", nil)
	if e != nil {
		return scan, e
	}
	if !scan.Scan.Scanning && time.Since(a.scanRequestedAt) >= 2*time.Second {
		_, e = a.nav(ctx, c, "startScan", nil)
		if e == nil {
			a.scanRequestedAt = time.Now()
		}
	}
	return scan, e
}
