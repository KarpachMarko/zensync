package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type audioDurationMismatch struct{ Expected, Measured, Tolerance float64 }

func (e *audioDurationMismatch) Error() string {
	return fmt.Sprintf("Downloaded duration differs from the selected recording (expected %.3fs, measured %.3fs; tolerance %.3fs)", e.Expected, e.Measured, e.Tolerance)
}
func audioApprovalKey(t Track) string {
	var identity any = t.ID
	if t.Provider == "lossless" || t.Provider == "hitmo" {
		identity = trackTarget(t)
	}
	return digest(mustJSON([]any{identity, normalized(trackLabel(t)), normalized(t.Artist.Name), normalized(t.Album.Title), strings.ToUpper(t.ISRC), t.Duration}))
}

type reviewAudioCandidate struct {
	Quality  string  `json:"quality"`
	Checksum string  `json:"checksum"`
	Expected float64 `json:"expected"`
	Measured float64 `json:"measured"`
	path     string
}

// Only server-discovered retained files are reviewable. A client cannot choose
// a path, change metadata, skip decoding or approve a different provider ID.
func (a *App) reviewAcquisitionAudio(w http.ResponseWriter, r *http.Request, u User) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v2/"), "/")
	if len(parts) < 3 || len(parts) > 4 {
		fail(w, 404, "Request not found")
		return
	}
	var jid, rid, owner, state, decision, target, metadata, quality, title, artist, stage string
	where := "j.id=?"
	if parts[0] == "requests" {
		where = "r.id=?"
	}
	err := a.DB.QueryRow(`SELECT j.id,r.id,r.owner,j.state,r.decision,r.target,COALESCE(m.metadata,''),r.quality,r.title,r.artist,j.stage FROM jobs j JOIN requests r ON r.id=j.request_id LEFT JOIN request_metadata m ON m.request_id=r.id WHERE `+where, parts[1]).Scan(&jid, &rid, &owner, &state, &decision, &target, &metadata, &quality, &title, &artist, &stage)
	if err != nil || (owner != u.ID && u.Role != "admin") || !targetPattern.MatchString(target) {
		fail(w, 404, "Request not found")
		return
	}
	if state != "failed" && state != "blocked" {
		fail(w, 409, "Only failed or blocked acquisitions can be reviewed")
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		fail(w, 405, "Method not allowed")
		return
	}
	var track Track
	var qualities []string
	_ = json.Unmarshal([]byte(metadata), &track)
	_ = json.Unmarshal([]byte(quality), &qualities)
	candidates := []reviewAudioCandidate{}
	for index, q := range qualities {
		if q != "lossless" && q != "320" && q != "128" {
			continue
		}
		stageQuality := q
		if track.Provider == "lossless" || track.Provider == "hitmo" {
			if index > 0 {
				break
			}
			stageQuality = track.Provider
		}
		paths, e := stagedAudio(filepath.Join(a.dataDir, "staging", jid, stageQuality))
		if e != nil || len(paths) != 1 || track.ID <= 0 {
			continue
		}
		if track.Provider == "lossless" || track.Provider == "hitmo" {
			actual, _, err := a.nativeAudioQuality(paths[0])
			if err != nil || !nativeQualityAllowed(qualities, actual) {
				continue
			}
			q = actual
		}
		var mismatch *audioDurationMismatch
		if e = a.validateAudio(paths[0], track, q); !errors.As(e, &mismatch) {
			continue
		}
		checksum, e := hashFile(paths[0])
		if e != nil {
			continue
		}
		candidates = append(candidates, reviewAudioCandidate{Quality: q, Checksum: checksum, Expected: mismatch.Expected, Measured: mismatch.Measured, path: paths[0]})
	}
	if len(parts) == 4 {
		if parts[3] != "file" || r.Method != "GET" {
			fail(w, 404, "File not found")
			return
		}
		for _, c := range candidates {
			if c.Quality == r.URL.Query().Get("quality") && c.Checksum == r.URL.Query().Get("checksum") {
				w.Header().Set("Cache-Control", "private, no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				http.ServeFile(w, r, c.path)
				return
			}
		}
		fail(w, 404, "Review file changed or is no longer available")
		return
	}
	if r.Method == "GET" {
		reviewURL := "/search?q=" + url.QueryEscape(strings.TrimSpace(artist+" "+title))
		// A migration is private even when an administrator reviews another user's
		// acquisition. Only expose a source link accessible to the current user.
		var mid, sid string
		if a.DB.QueryRow(`SELECT s.migration_id,s.id FROM request_sources rs JOIN source_songs s ON s.id=rs.source_id JOIN migrations m ON m.id=s.migration_id WHERE rs.request_id=? AND m.owner=? ORDER BY s.id LIMIT 1`, rid, u.ID).Scan(&mid, &sid) == nil {
			reviewURL = "/imports/" + url.PathEscape(mid) + "?recording=" + url.QueryEscape(sid)
		}
		send(w, 200, map[string]any{"title": title, "artist": artist, "stage": stage, "reviewURL": reviewURL, "canAccept": decision == "approved" && state == "failed", "candidates": candidates})
		return
	}
	var in struct {
		Quality  string `json:"quality"`
		Checksum string `json:"checksum"`
	}
	if !decode(w, r, &in) {
		return
	}
	var selected *reviewAudioCandidate
	for i := range candidates {
		if candidates[i].Quality == in.Quality && candidates[i].Checksum == in.Checksum {
			selected = &candidates[i]
			break
		}
	}
	if selected == nil || !validDuration(selected.Measured) || math.Abs(selected.Measured-selected.Expected) < 0.01 {
		fail(w, 409, "Retained file changed or has no reviewable duration mismatch. Refresh the review.")
		return
	}
	a.workMu.Lock()
	defer a.workMu.Unlock()
	if _, active := a.activeJobs[jid]; active {
		fail(w, 409, "Task is already running")
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var currentState, currentDecision, currentMetadata string
	e = tx.QueryRow(`SELECT j.state,r.decision,m.metadata FROM jobs j JOIN requests r ON r.id=j.request_id JOIN request_metadata m ON m.request_id=r.id WHERE j.id=?`, jid).Scan(&currentState, &currentDecision, &currentMetadata)
	if e != nil || currentState != "failed" || currentDecision != "approved" || currentMetadata != metadata {
		fail(w, 409, "The request must still be approved and failed before accepting this file")
		return
	}
	_, e = tx.Exec(`INSERT INTO audio_duration_approvals(checksum,recording,quality,duration,actor,request_id) VALUES(?,?,?,?,?,?) ON CONFLICT(checksum,recording,quality) DO UPDATE SET duration=excluded.duration,actor=excluded.actor,request_id=excluded.request_id`, selected.Checksum, audioApprovalKey(track), selected.Quality, selected.Measured, u.ID, rid)
	if e == nil {
		_, e = tx.Exec("DELETE FROM acquisition_checks WHERE job_id=?", jid)
	}
	if e == nil {
		_, e = tx.Exec("DELETE FROM scan_waits WHERE job_id=?", jid)
	}
	if e == nil {
		_, e = tx.Exec("DELETE FROM scan_schedule WHERE job_id=?", jid)
	}
	if e == nil {
		_, e = tx.Exec("UPDATE jobs SET state='queued',stage='Duration mismatch explicitly accepted; rechecking retained audio' WHERE id=?", jid)
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
