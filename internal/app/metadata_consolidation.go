package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// A library merge retains the selected file and ID. Replacing audio is a
// separate operation; it deliberately retains both library entries.
type songConsolidation struct {
	Source        Song   `json:"source"`
	SourcePath    string `json:"sourcePath"`
	SourceHash    string `json:"sourceHash"`
	KeptPath      string `json:"keptPath"`
	KeptHash      string `json:"keptHash"`
	RemovalID     string `json:"removalId"`
	WriteMetadata bool   `json:"writeMetadata"`
}
type mergeSongMetadata struct {
	Title       *string `json:"title"`
	Artist      *string `json:"artist"`
	Album       *string `json:"album"`
	AlbumArtist *string `json:"albumArtist"`
}

func (a *App) queueSongConsolidation(w http.ResponseWriter, r *http.Request, u User, source, target string, patch *mergeSongMetadata) {
	if source == "" || target == "" || source == target || len(source) > 256 || len(target) > 256 {
		fail(w, 400, "Choose distinct original and selected songs")
		return
	}
	a.opMu.Lock()
	defer a.opMu.Unlock()
	c, err := a.shared()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	original, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {source}, "realPath": {"true"}})
	if err != nil || original.Song.ID != source {
		fail(w, 404, "Original song is unavailable")
		return
	}
	selected, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {target}, "realPath": {"true"}})
	if err != nil || selected.Song.ID != target {
		fail(w, 404, "Selected song is unavailable")
		return
	}
	from, fromHash, err := a.deletionPath(r.Context(), c, original.Song)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	to, toHash, err := a.deletionPath(r.Context(), c, selected.Song)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	if from == to {
		fail(w, 409, "The original and selected song point to the same file")
		return
	}
	edit, err := a.existingMetadataEdit(selected.Song)
	if err != nil {
		a.internal(w, err)
		return
	}
	before := mustJSON(edit)
	if patch != nil {
		for _, field := range []struct {
			value       *string
			destination *string
			required    bool
			max         int
		}{{patch.Title, &edit.Title, true, 500}, {patch.Artist, &edit.Artist, true, 10500}, {patch.Album, &edit.Album, false, 500}, {patch.AlbumArtist, &edit.AlbumArtist, false, 10500}} {
			if field.value == nil {
				continue
			}
			value := strings.TrimSpace(*field.value)
			if len(value) > field.max || (field.required && value == "") {
				fail(w, 400, "Title and artist are required; metadata exceeds the allowed length")
				return
			}
			*field.destination = value
		}
		if patch.Artist != nil && edit.Artist != selected.Song.Artist {
			edit.ArtistNames = nil
		}
		if patch.AlbumArtist != nil && edit.AlbumArtist != selected.Song.AlbumArtist {
			edit.AlbumArtistNames = nil
		}
	}
	merge := &songConsolidation{Source: original.Song, SourcePath: from, SourceHash: fromHash, KeptPath: to, KeptHash: toHash, RemovalID: id(), WriteMetadata: before != mustJSON(edit)}
	jid, err := a.queueMetadata([]metadataChange{{Song: selected.Song, Edit: edit, Consolidation: merge}}, u)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	slog.Info("Library song consolidation queued", "job_id", jid, "source_song_id", source, "source_path", from, "kept_song_id", target, "kept_path", to, "metadata_changed", merge.WriteMetadata)
	send(w, 202, map[string]any{"id": jid, "songs": 1})
}

func prepareConsolidationRemoval(tx *sql.Tx, jid, server string, u User, change metadataChange) error {
	merge := change.Consolidation
	if merge == nil {
		return nil
	}
	// A removal journal and recovery trail belong to the same transaction as the
	// metadata job. The ordinary deletion worker cannot run this blocked job.
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision,reason) VALUES(?,?,?,?,?,?,0,'[]','approved','Library song merge')", []any{merge.RemovalID, u.ID, "metadata-merge:" + jid, "navidrome:song:" + merge.Source.ID, merge.Source.Title, merge.Source.Artist}},
		{"INSERT INTO song_deletions(request_id,song_id,file_path,checksum) VALUES(?,?,?,?)", []any{merge.RemovalID, merge.Source.ID, merge.SourcePath, merge.SourceHash}},
		{"INSERT INTO deletion_replacements(request_id,keep_song_id,server_url) VALUES(?,?,?)", []any{merge.RemovalID, change.Song.ID, server}},
		{"INSERT INTO jobs(id,request_id,state,stage) VALUES(?,?,'blocked','Waiting for library merge metadata review')", []any{id(), merge.RemovalID}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement.query, statement.args...); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) finishSongConsolidation(ctx context.Context, jid string, change metadataChange) error {
	merge := change.Consolidation
	if merge == nil {
		return nil
	}
	var owner, actor, phase string
	if err := a.DB.QueryRow("SELECT r.owner,COALESCE(r.actor,''),d.phase FROM requests r JOIN song_deletions d ON d.request_id=r.id WHERE r.id=?", merge.RemovalID).Scan(&owner, &actor, &phase); err != nil {
		return err
	}
	if phase != "removed" {
		if err := a.checkMusicParents(merge.SourcePath); err != nil {
			return err
		}
		original := filepath.Join(a.musicDir, merge.SourcePath)
		if info, err := os.Lstat(original); err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("Original file is unsafe; no song removed")
			}
			if actual, err := hashFile(original); err != nil || actual != merge.SourceHash {
				return errors.New("Original file changed since merge review; no song removed")
			}
		} else if !os.IsNotExist(err) || phase != "prepared" {
			return errors.New("Original file is missing; no song removed")
		}
		expected := merge.KeptHash
		if merge.WriteMetadata {
			if err := a.DB.QueryRow("SELECT after_hash FROM metadata_files WHERE job_id=? AND song_id=?", jid, change.Song.ID).Scan(&expected); err != nil {
				return err
			}
		}
		if err := a.checkMusicParents(merge.KeptPath); err != nil {
			return err
		}
		kept := filepath.Join(a.musicDir, merge.KeptPath)
		info, err := os.Lstat(kept)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("Selected file is missing or unsafe; original retained")
		}
		if actual, err := hashFile(kept); err != nil || actual != expected {
			return errors.New("Selected file changed since merge review; original retained")
		}
		if err := a.replaceDeletionPlaylists(ctx, merge.RemovalID, merge.Source.ID, merge.SourcePath); err != nil {
			return err
		}
	}
	// Imports resolved to the removed entry should now point at the survivor.
	// Do this before removeLibrarySong clears references to intentional removals.
	if _, err := a.DB.Exec("UPDATE source_songs SET server_id=? WHERE server_id=?", change.Song.ID, merge.Source.ID); err != nil {
		return err
	}
	if err := a.removeLibrarySong(ctx, merge.RemovalID, owner, actor, merge.SourcePath, merge.SourceHash, merge.Source.ID, phase); err != nil {
		return err
	}
	if _, err := a.DB.Exec("UPDATE jobs SET state='completed',stage='Merged into selected library song; recovery copy retained' WHERE request_id=?", merge.RemovalID); err != nil {
		return err
	}
	slog.Info("Library song consolidation completed", "job_id", jid, "removed_song_id", merge.Source.ID, "removed_path", merge.SourcePath, "kept_song_id", change.Song.ID, "kept_path", merge.KeptPath)
	return nil
}

func consolidationDeletionConflict(tx *sql.Tx, changes []metadataChange) error {
	for _, change := range changes {
		ids := []string{change.Song.ID}
		if change.Target != nil {
			ids = append(ids, change.Target.ID)
		}
		if change.Consolidation != nil {
			ids = append(ids, change.Consolidation.Source.ID)
		}
		ownRemoval := ""
		if change.Consolidation != nil {
			ownRemoval = change.Consolidation.RemovalID
		}
		for _, song := range ids {
			var count int
			err := tx.QueryRow(`SELECT count(*) FROM song_deletions d JOIN requests r ON r.id=d.request_id LEFT JOIN jobs j ON j.request_id=r.id LEFT JOIN deletion_replacements k ON k.request_id=r.id WHERE r.decision IN ('approved','awaiting_approval') AND COALESCE(j.state,'') NOT IN ('completed','cancelled') AND (d.song_id=? OR k.keep_song_id=?) AND r.id<>?`, song, song, ownRemoval).Scan(&count)
			if err != nil {
				return err
			}
			if count > 0 {
				return errors.New("A pending song removal uses a merge entry; finish it before editing or merging")
			}
		}
	}
	return nil
}

// Verify the selected bytes before applying metadata. A retry may see either
// the original selected file or the verified output from the previous write.
func (a *App) verifyConsolidationSelected(jid string, change metadataChange) error {
	merge := change.Consolidation
	var decision string
	if err := a.DB.QueryRow("SELECT decision FROM requests WHERE id=?", merge.RemovalID).Scan(&decision); err != nil {
		return err
	}
	if decision != "approved" {
		return errors.New("Library merge removal is no longer approved; no song removed")
	}
	if err := a.checkMusicParents(merge.KeptPath); err != nil {
		return err
	}
	path := filepath.Join(a.musicDir, merge.KeptPath)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("Selected file is missing or unsafe; original retained")
	}
	var after string
	err = a.DB.QueryRow("SELECT after_hash FROM metadata_files WHERE job_id=? AND song_id=?", jid, change.Song.ID).Scan(&after)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	actual, err := hashFile(path)
	if err != nil || (actual != merge.KeptHash && (after == "" || actual != after)) {
		return errors.New("Selected file changed since merge review; original retained")
	}
	return nil
}
