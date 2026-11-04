package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type migrationAcquisitionResult struct {
	Created  int `json:"created"`
	Existing int `json:"existing"`
	Skipped  int `json:"skipped"`
}

// Enqueue the complete selection atomically, without provider I/O or a
// browser-driven loop. Once committed, ordinary durable requests/jobs drive
// approval and acquisition independently of this HTTP connection or process.
// The caller holds opMu, just as for individual source acquisition actions.
func (a *App) acquireMigrationSongs(w http.ResponseWriter, r *http.Request, u User, m Migration) {
	var in struct {
		SourceIDs []string `json:"sourceIds"`
	}
	if !decodeLimit(w, r, &in, 1<<20) {
		return
	}
	if m.State == "staged" {
		fail(w, 409, "Commit the migration first")
		return
	}
	if len(in.SourceIDs) == 0 || len(in.SourceIDs) > 20000 {
		fail(w, 400, "Select between 1 and 20,000 imported songs")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 128 {
		fail(w, 400, "An 8–128 character Idempotency-Key is required")
		return
	}
	sort.Strings(in.SourceIDs)
	ids := []string{}
	for _, sid := range in.SourceIDs {
		if sid == "" || len(sid) > 128 {
			fail(w, 400, "Invalid source song ID")
			return
		}
		if len(ids) == 0 || ids[len(ids)-1] != sid {
			ids = append(ids, sid)
		}
	}
	fingerprint := digest(m.ID + mustJSON(ids))
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	var priorFingerprint, priorResult string
	err = tx.QueryRow("SELECT fingerprint,result FROM migration_acquisition_batches WHERE owner=? AND idem=?", u.ID, key).Scan(&priorFingerprint, &priorResult)
	if err == nil {
		if fingerprint != priorFingerprint {
			fail(w, 409, "Batch key already used for another selection")
			return
		}
		send(w, 200, map[string]any{"bulk": json.RawMessage(priorResult)})
		return
	}
	if err != sql.ErrNoRows {
		a.internal(w, err)
		return
	}
	result := migrationAcquisitionResult{}
	for _, sid := range ids {
		var candidate, server string
		var deferred bool
		err = tx.QueryRow("SELECT candidate,server_id,deferred FROM source_songs WHERE id=? AND migration_id=?", sid, m.ID).Scan(&candidate, &server, &deferred)
		if err == sql.ErrNoRows {
			fail(w, 404, "A selected song does not belong to this migration")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if server != "" || deferred || candidate == "" {
			result.Skipped++
			continue
		}
		var track Track
		if json.Unmarshal([]byte(candidate), &track) != nil || track.ID <= 0 || strings.TrimSpace(trackLabel(track)) == "" || len(trackLabel(track)) > 300 || len(track.Artist.Name) > 300 {
			fail(w, 409, "A selected song has invalid recording metadata; review its recording first")
			return
		}
		var prior, decision string
		err = tx.QueryRow("SELECT r.id,r.decision FROM request_sources rs JOIN requests r ON r.id=rs.request_id WHERE rs.source_id=? ORDER BY r.rowid DESC LIMIT 1", sid).Scan(&prior, &decision)
		if err != nil && err != sql.ErrNoRows {
			a.internal(w, err)
			return
		}
		if decision == "approved" || decision == "awaiting_approval" {
			result.Existing++
			continue
		}
		var revision string
		err = tx.QueryRow("SELECT revision FROM source_resolutions WHERE source_id=?", sid).Scan(&revision)
		if err != nil && err != sql.ErrNoRows {
			a.internal(w, err)
			return
		}
		identity := fmt.Sprint(track.ID)
		if track.Provider == "lossless" {
			identity = trackTarget(track)
		}
		sourceKey := digest(sid + identity + prior + revision)
		if _, err = insertTrackRequest(tx, u, sourceKey, sid, track, false); err != nil {
			a.internal(w, err)
			return
		}
		result.Created++
	}
	if _, err = tx.Exec("INSERT INTO migration_acquisition_batches(owner,idem,migration_id,fingerprint,result) VALUES(?,?,?,?,?)", u.ID, key, m.ID, fingerprint, mustJSON(result)); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	send(w, 201, map[string]any{"bulk": result})
}
