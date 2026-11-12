package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type metadataChange struct {
	Enrichment    string               `json:"enrichment,omitempty"`
	Resolved      bool                 `json:"resolved,omitempty"`
	Skip          bool                 `json:"skip,omitempty"`
	CreditRepairs []creditRepairPlan   `json:"creditRepairs,omitempty"`
	UploadedCover string               `json:"uploadedCover,omitempty"`
	CreditRepair  *creditRepairPlan    `json:"creditRepair,omitempty"`
	ArtistMerge   *artistMergeIdentity `json:"artistMerge,omitempty"`
	Song          Song                 `json:"song"`
	Edit          MetadataEdit         `json:"edit"`
	MergeKind     string               `json:"mergeKind,omitempty"`
	Target        *Song                `json:"target,omitempty"`
}

func (a *App) queueMetadata(changes []metadataChange, u User) (string, error) {
	credentials, e := a.shared()
	if e != nil {
		return "", e
	}
	tx, e := a.DB.Begin()
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	if e = metadataQueueConflict(tx, changes); e != nil {
		return "", e
	}
	jid := id()
	for _, change := range changes {
		if change.Enrichment != "" {
			continue
		}
		if _, e = tx.Exec("INSERT INTO metadata_edits(song_id,metadata,actor) VALUES(?,?,?) ON CONFLICT(song_id) DO UPDATE SET metadata=excluded.metadata,actor=excluded.actor,updated=CURRENT_TIMESTAMP", change.Song.ID, mustJSON(change.Edit), u.ID); e != nil {
			return "", e
		}
	}
	if _, e = tx.Exec("INSERT INTO metadata_jobs(id,actor,changes,server_url) VALUES(?,?,?,?)", jid, u.ID, mustJSON(changes), credentials.URL); e != nil {
		return "", e
	}
	if e = audit(tx, u.ID, u.ID, "metadata.queued", jid); e != nil {
		return "", e
	}
	if e = tx.Commit(); e != nil {
		return "", e
	}
	slog.Info("Metadata job queued", "job_id", jid, "user_id", u.ID, "songs", len(changes))
	return jid, nil
}

// Jobs execute serially, but snapshots for overlapping files cannot safely be
// prepared in advance. Shared read-only destinations are allowed.
func metadataQueueConflict(tx *sql.Tx, changes []metadataChange) error {
	writes, reads := map[string]bool{}, map[string]bool{}
	for _, change := range changes {
		writes[change.Song.ID] = true
		if change.Target != nil {
			reads[change.Target.ID] = true
		}
	}
	rows, err := tx.Query("SELECT changes FROM metadata_jobs WHERE state IN ('queued','running')")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var previous []metadataChange
		if err := json.Unmarshal([]byte(raw), &previous); err != nil {
			return errors.New("An active metadata job is unreadable; review Activity before queuing changes")
		}
		for _, change := range previous {
			if writes[change.Song.ID] || reads[change.Song.ID] || (change.Target != nil && writes[change.Target.ID]) {
				return errors.New("A queued or running metadata update uses the same song or merge destination; wait for that operation and refresh before editing these files")
			}
		}
	}
	return rows.Err()
}

func (a *App) existingMetadataEdit(s Song) (MetadataEdit, error) {
	edit := MetadataEdit{Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumArtist: s.AlbumArtist}
	var raw string
	err := a.DB.QueryRow("SELECT metadata FROM metadata_edits WHERE song_id=?", s.ID).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &edit)
	} else if err == sql.ErrNoRows {
		err = nil
	}
	return edit, err
}
func (a *App) mergeMetadata(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Kind   string `json:"kind"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Kind != "artist" && in.Kind != "album" && in.Kind != "song") || in.Source == "" || in.Target == "" || in.Source == in.Target {
		fail(w, 400, "Choose distinct source and destination artists, albums or songs")
		return
	}
	c, e := a.shared()
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	index, e := a.indexedNavSongs(r.Context(), c)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	if index.limited {
		fail(w, 409, "Merge requires a complete library scan; this library exceeds the scan limit")
		return
	}
	var target Song
	source := []Song{}
	for _, s := range index.songs {
		key := s.ArtistID
		if in.Kind == "song" {
			key = s.ID
		}
		if in.Kind == "album" {
			key = s.AlbumID
		}
		if key == in.Target || (in.Kind == "artist" && creditNameForID(s, in.Target) != "") {
			target = s
		}
		if key == in.Source || (in.Kind == "artist" && creditNameForID(s, in.Source) != "") {
			source = append(source, s)
		}
	}
	if target.ID == "" || len(source) == 0 {
		fail(w, 404, "Source or destination no longer exists; refresh candidates")
		return
	}
	changes := []metadataChange{}
	for _, s := range source {
		edit, e := a.existingMetadataEdit(s)
		if e != nil {
			a.internal(w, e)
			return
		}
		var identity *artistMergeIdentity
		if in.Kind == "artist" {
			identity = &artistMergeIdentity{
				SourceName: creditNameForID(s, in.Source), TargetName: creditNameForID(target, in.Target),
				ReplaceTrack: creditHasID(s.Artists, in.Source) || (len(s.Artists) == 0 && s.ArtistID == in.Source),
				ReplaceAlbum: creditHasID(s.AlbumArtists, in.Source),
			}
			if len(s.AlbumArtists) == 0 && (s.AlbumArtist != "" && strings.EqualFold(s.AlbumArtist, identity.SourceName)) {
				identity.ReplaceAlbum = true
			}
			if identity.ReplaceTrack {
				edit.Artist = replaceCreditDisplay(s.Artists, in.Source, identity.TargetName, s.Artist)
			}
			if identity.ReplaceAlbum {
				edit.AlbumArtist = replaceCreditDisplay(s.AlbumArtists, in.Source, identity.TargetName, s.AlbumArtist)
			}
		} else if in.Kind == "album" {
			edit.Album = target.Album
			edit.AlbumArtist = target.AlbumArtist
			if edit.AlbumArtist == "" {
				edit.AlbumArtist = target.Artist
			}
		}
		changes = append(changes, metadataChange{Song: s, Edit: edit, MergeKind: in.Kind, Target: &target, ArtistMerge: identity})
	}
	jid, e := a.queueMetadata(changes, u)
	if e != nil {
		fail(w, 409, e.Error())
		return
	}
	destinationArtist := target.Artist
	if in.Kind == "artist" {
		destinationArtist = creditNameForID(target, in.Target)
	}
	slog.Info("Metadata merge queued", "job_id", jid, "kind", in.Kind, "source_id", in.Source, "target_id", in.Target, "target_artist", destinationArtist, "target_album", target.Album, "songs", len(changes))
	send(w, 202, map[string]any{"id": jid, "songs": len(changes)})
}
func (a *App) metadataJobRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, e := a.DB.Query("SELECT id,state,message,done," + metadataActivityTotalSQL + "," + metadataActivityTitleSQL + " FROM metadata_jobs ORDER BY " + executionOrderSQL("state") + ",rowid DESC LIMIT 20")
		if e != nil {
			a.internal(w, e)
			return
		}
		defer rows.Close()
		type job struct {
			Title   string `json:"title"`
			ID      string `json:"id"`
			State   string `json:"state"`
			Message string `json:"message"`
			Done    int    `json:"done"`
			Total   int    `json:"total"`
		}
		jobs := []job{}
		for rows.Next() {
			var j job
			if e = rows.Scan(&j.ID, &j.State, &j.Message, &j.Done, &j.Total, &j.Title); e != nil {
				a.internal(w, e)
				return
			}
			jobs = append(jobs, j)
		}
		if e = rows.Err(); e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, jobs)
		return
	}
	if r.Method == "POST" {
		var in struct {
			ID string `json:"id"`
		}
		if !decode(w, r, &in) {
			return
		}
		tx, e := a.DB.Begin()
		if e != nil {
			a.internal(w, e)
			return
		}
		defer tx.Rollback()
		var raw string
		if e = tx.QueryRow("SELECT changes FROM metadata_jobs WHERE id=? AND state='failed'", in.ID).Scan(&raw); e != nil {
			if e == sql.ErrNoRows {
				fail(w, 409, "Only a failed operation can be retried")
			} else {
				a.internal(w, e)
			}
			return
		}
		var changes []metadataChange
		if e = json.Unmarshal([]byte(raw), &changes); e != nil {
			fail(w, 409, "The failed operation is unreadable")
			return
		}
		if e = metadataQueueConflict(tx, changes); e != nil {
			fail(w, 409, e.Error())
			return
		}
		if _, e = tx.Exec("UPDATE metadata_jobs SET state='queued',message='Retry queued' WHERE id=? AND state='failed'", in.ID); e != nil {
			a.internal(w, e)
			return
		}
		if e = tx.Commit(); e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	fail(w, 405, "Method not allowed")
}
func (a *App) runMetadata(ctx context.Context) {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.workMu.Lock()
	defer a.workMu.Unlock()
	if len(a.activeJobs) > 0 {
		return
	}
	var jid, raw, serverURL string
	var done int
	if a.DB.QueryRow("SELECT id,changes,done,server_url FROM metadata_jobs WHERE state IN ('queued','running') ORDER BY rowid LIMIT 1").Scan(&jid, &raw, &done, &serverURL) != nil {
		return
	}
	_, _ = a.DB.Exec("UPDATE metadata_jobs SET state='running',message='Updating file tags' WHERE id=?", jid)
	var changes []metadataChange
	e := json.Unmarshal([]byte(raw), &changes)
	logger := slog.With("job_id", jid, "songs", len(changes))
	logger.Info("Metadata job started", "done", done)
	c, ce := a.shared()
	if ce == nil && c.URL != serverURL {
		ce = errors.New("Navidrome connection changed since this operation was queued")
	}
	if e == nil {
		e = ce
	}
	for e == nil && done < len(changes) {
		change := changes[done]
		if change.Enrichment != "" && !change.Resolved {
			change, e = a.resolveEnrichment(ctx, c, change)
			if e != nil {
				break
			}
			change.Resolved = true
			changes[done] = change
			_, e = a.DB.Exec("UPDATE metadata_jobs SET changes=? WHERE id=?", mustJSON(changes), jid)
			if e != nil {
				break
			}
		}
		if change.Skip {
			done++
			_, e = a.DB.Exec("UPDATE metadata_jobs SET done=? WHERE id=?", done, jid)
			continue
		}
		if change.MergeKind == "credits" {
			var skip bool
			change, skip, e = a.filterDismissedCreditRepairs(c, change)
			if e != nil {
				break
			}
			if skip {
				logger.Info("Metadata credit repair skipped; artist marked as indivisible", "song_id", change.Song.ID)
				done++
				_, e = a.DB.Exec("UPDATE metadata_jobs SET done=? WHERE id=?", done, jid)
				continue
			}
		}
		logger.Info("Metadata song update started", "song_id", change.Song.ID, "kind", change.MergeKind, "artist_before", change.Song.Artist, "artist_after", change.Edit.Artist, "album_artist_before", change.Song.AlbumArtist, "album_artist_after", change.Edit.AlbumArtist)
		e = a.publishMetadata(ctx, c, jid, change)
		if errors.Is(e, errCreditRepairDismissed) {
			logger.Info("Metadata credit repair skipped; artist marked as indivisible", "song_id", change.Song.ID)
			e = nil
		} else if e != nil {
			logger.Error("Metadata song update failed", "song_id", change.Song.ID, "error", e)
		} else {
			logger.Info("Metadata song tags verified and published", "song_id", change.Song.ID)
		}
		if e == nil {
			done++
			_, e = a.DB.Exec("UPDATE metadata_jobs SET done=? WHERE id=?", done, jid)
		}
	}
	if e == nil {
		e = a.scanMetadataChanges(ctx, c, jid)
		if e == nil {
			logger.Info("Metadata incremental scan requested")
		}
	}
	if errors.Is(e, errMetadataScanBusy) {
		_, _ = a.DB.Exec("UPDATE metadata_jobs SET state='queued',message=? WHERE id=?", e.Error(), jid)
		return
	}
	skipped := 0
	for _, change := range changes {
		if change.Skip {
			skipped++
		}
	}
	state, message := "completed", fmt.Sprintf("%d updated, %d skipped (existing metadata, no confident match or unsupported format); Navidrome scan requested.", len(changes)-skipped, skipped)
	if e != nil {
		state = "failed"
		message = fmt.Sprintf("%d/%d songs updated: %s. Original files are unchanged for failed writes; retry resumes remaining work.", done, len(changes), e.Error())
	}
	if _, err := a.DB.Exec("UPDATE metadata_jobs SET state=?,message=? WHERE id=?", state, message, jid); err != nil {
		logger.Error("Metadata job completion persistence failed", "error", err)
	}
	if e != nil {
		logger.Error("Metadata job failed", "done", done, "error", e)
	} else {
		logger.Info("Metadata job completed; Navidrome scan pending", "done", done)
	}
	a.navIndexMu.Lock()
	clear(a.navIndexes)
	a.navIndexMu.Unlock()
}

// Each remux is prepared and journaled before replacing the source. A restart
// recognizes either checksum, avoiding replay against an uncertain file.
func (a *App) publishMetadata(ctx context.Context, c Credentials, jid string, change metadataChange) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var rel, before, after string
	e := a.DB.QueryRow("SELECT file_path,before_hash,after_hash FROM metadata_files WHERE job_id=? AND song_id=?", jid, change.Song.ID).Scan(&rel, &before, &after)
	if e == sql.ErrNoRows {
		current, ne := a.nav(ctx, c, "getSong", url.Values{"id": {change.Song.ID}, "realPath": {"true"}})
		if ne != nil {
			return ne
		}
		if current.Song.ID != change.Song.ID {
			return errors.New("Song no longer exists")
		}
		if current.Song.Title != change.Song.Title || current.Song.Artist != change.Song.Artist || current.Song.Album != change.Song.Album {
			return errors.New("Song metadata changed since this operation was queued; refresh before editing")
		}
		rel, before, e = a.deletionPath(ctx, c, current.Song)
		if e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	if e = a.checkMusicParents(rel); e != nil {
		return e
	}
	path := filepath.Join(a.musicDir, rel)
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		return errors.New("Source audio is missing or unsafe")
	}
	hash, e := hashFile(path)
	if e != nil {
		return e
	}
	if after != "" && hash == after {
		return nil
	}
	if hash != before {
		return errors.New("Source changed since this update was prepared")
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".flac", ".m4a", ".ogg", ".opus", ".wav", ".aiff":
	default:
		return errors.New("Unsupported tag format; use MP3, FLAC, M4A, OGG, OPUS, WAV or AIFF")
	}
	requiredSpace := uint64(info.Size()) + (256 << 20)
	if change.MergeKind == "song" && change.Target != nil {
		_, targetPath, err := a.metadataCanonicalTags(ctx, c, *change.Target)
		if err != nil {
			return err
		}
		targetInfo, err := os.Stat(targetPath)
		if err != nil {
			return err
		}
		requiredSpace += uint64(targetInfo.Size())
	}
	if change.Edit.CoverURL != "" || change.UploadedCover != "" {
		requiredSpace += 2 * maxMetadataCover
	}
	if err := diskHeadroom(filepath.Dir(path), requiredSpace); err != nil {
		return errors.New("Not enough free space for metadata working copy plus 256 MiB headroom")
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".zensync-tags-"+jid+"-"+digest(change.Song.ID)+"-*"+ext)
	if e != nil {
		return e
	}
	output := tmp.Name()
	tmp.Close()
	os.Remove(output)
	defer os.Remove(output)
	audioPath := path
	if change.MergeKind == "song" && change.Target != nil {
		_, audioPath, e = a.metadataCanonicalTags(ctx, c, *change.Target)
		if e != nil {
			return e
		}
		if strings.ToLower(filepath.Ext(audioPath)) != ext {
			return errors.New("Recording replacement requires matching file formats; no lossy conversion is performed")
		}
	}
	args := []string{"-v", "error", "-nostdin", "-protocol_whitelist", "file,pipe", "-i", path}
	if change.MergeKind == "song" {
		args = append(args, "-i", audioPath, "-map", "1:a", "-map", "0:v?", "-c:v", "copy")
	}
	useCover := change.Edit.CoverURL != "" || change.UploadedCover != ""
	if change.MergeKind == "song" {
		// Keep the original location and metadata; replace only encoded audio.
	} else if change.Edit.RemoveCover {
		args = append(args, "-map", "0:a")
	} else if useCover {
		if ext != ".mp3" && ext != ".flac" && ext != ".m4a" {
			return errors.New("Cover embedding currently supports MP3, FLAC and M4A")
		}
		coverFile, err := os.CreateTemp(filepath.Dir(path), ".zensync-cover-"+jid+"-"+digest(change.Song.ID)+"-*")
		if err != nil {
			return err
		}
		cover := coverFile.Name()
		coverFile.Close()
		defer os.Remove(cover)
		if change.UploadedCover != "" {
			data, err := a.readCoverUpload(change.UploadedCover)
			if err != nil {
				return err
			}
			if err := os.WriteFile(cover, data, 0600); err != nil {
				return err
			}
		} else if e = a.downloadMetadataCover(ctx, change.Edit.CoverURL, cover); e != nil {
			return e
		}
		args = append(args, "-i", cover, "-map", "0:a", "-map", "1:v:0", "-c:v", "mjpeg", "-disposition:v:0", "attached_pic")
	} else {
		args = append(args, "-map", "0", "-c:v", "copy")
	}
	args = append(args, "-map_metadata", "0", "-c:a", "copy")
	tags := map[string]string{"title": change.Edit.Title, "artist": change.Edit.Artist, "album": change.Edit.Album}
	if change.Edit.TrackNumber > 0 {
		tags["track"] = fmt.Sprint(change.Edit.TrackNumber)
	}
	if change.Edit.DiscNumber > 0 {
		tags["disc"] = fmt.Sprint(change.Edit.DiscNumber)
	}
	if change.MergeKind == "credits" {
		tags = map[string]string{}
	}
	if change.Edit.Lyrics != "" {
		tags["lyrics"] = change.Edit.Lyrics
	}
	if change.Edit.SyncedLyrics != "" {
		tags["syncedlyrics"] = change.Edit.SyncedLyrics
		tags["lyrics"] = change.Edit.SyncedLyrics
	}
	if change.Edit.AlbumArtist != "" {
		tags["album_artist"] = change.Edit.AlbumArtist
	}
	if change.MergeKind == "" && metadataTrackArtistsChanged(change) {
		tags["album_artist"] = change.Edit.Artist
		tags["artists"] = change.Edit.Artist
		tags["albumartists"] = change.Edit.Artist
		if change.Edit.AlbumArtist != "" && change.Edit.AlbumArtist != change.Song.Artist {
			tags["album_artist"] = change.Edit.AlbumArtist
			tags["albumartists"] = change.Edit.AlbumArtist
		}
		for _, key := range []string{"musicbrainz_artistid", "musicbrainz_albumartistid"} {
			tags[key] = ""
		}
	}
	if (change.MergeKind == "" || change.MergeKind == "album") && (change.Edit.Album != change.Song.Album || metadataTrackArtistsChanged(change)) {
		tags["musicbrainz_albumid"] = ""
		tags["musicbrainz_releasegroupid"] = ""
	}
	canonicalPath := path
	if change.MergeKind != "" && change.MergeKind != "credits" && change.MergeKind != "song" && change.Target != nil {
		canonical, targetPath, e := a.metadataCanonicalTags(ctx, c, *change.Target)
		if e != nil {
			return e
		}
		canonicalPath = targetPath
		if change.MergeKind == "album" {
			for _, key := range []string{"musicbrainz_albumid", "musicbrainz album id", "musicbrainz_releasegroupid", "musicbrainz release group id", "musicbrainz_albumartistid", "musicbrainz album artist id", "albumartist", "album_artist", "albumartists", "albumversion", "album_version", "releasedate", "date", "year", "originaldate", "originalyear", "compilation"} {
				tags[key] = canonical[key]
			}
			tags["album_artist"] = change.Edit.AlbumArtist

		}
	}
	// Navidrome recognises several aliases and prefers the plural tags for
	// grouping. Updating just album_artist leaves existing aliases authoritative.
	if change.MergeKind != "" {
		if value, ok := tags["album_artist"]; ok {
			tags["albumartist"] = value
			tags["album artist"] = value
		}
		if value, ok := tags["albumartists"]; ok {
			tags["album artists"] = value
		}
	}
	for key, value := range tags {
		args = append(args, "-metadata", key+"="+value)
	}
	if ext == ".aiff" {
		args = append(args, "-write_id3v2", "1")
	}
	args = append(args, output)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	protectChild(cmd)
	if change.MergeKind == "credits" {
		if err := prepareCreditRepairFile(path, output); err != nil {
			return err
		}
	} else if output, err := cmd.CombinedOutput(); err != nil {
		slog.Error("Metadata tag writer failed", "job_id", jid, "song_id", change.Song.ID, "error", err, "detail", string(output[:min(len(output), 2000)]))
		return errors.New("Could not write audio tags; original file retained")
	}
	if change.MergeKind == "" || change.MergeKind == "song" {
		if err := preserveEditedMetadata(ctx, &change, path, output); err != nil {
			return err
		}
	}
	if change.Edit.RemoveCover {
		if err := removeMetadataArtwork(ctx, output); err != nil {
			return err
		}
	}
	if change.MergeKind != "" && change.MergeKind != "song" {
		if err := preserveMergeCredits(ctx, &change, path, canonicalPath, output); err != nil {
			return err
		}
		if change.MergeKind != "credits" {
			// The native writer verifies individual list values. ffprobe flattens
			// native lists, so only compare the single-valued display fields here.
			for _, key := range []string{"artists", "albumartists", "album artists", "albumartist", "album artist", "album_artist"} {
				delete(tags, key)
			}
			tags["artist"] = change.Edit.Artist
			p, err := a.probeUpload(output, uploadFormat(ext))
			if err != nil {
				return fmt.Errorf("Cannot verify written grouping tags: %w", err)
			}
			written := uploadTags(p)
			for _, key := range []string{"artist", "artists", "album", "albumartist", "album artist", "album_artist", "albumartists", "album artists"} {
				expected, changed := tags[key]
				actual, present := written[key]
				if changed && present && actual != expected {
					return fmt.Errorf("Grouping tag %s did not update; original file retained", key)
				}
			}
			expectedAlbum := change.Edit.Album

			if written["artist"] != change.Edit.Artist || written["album"] != expectedAlbum {
				return fmt.Errorf("Written tags did not match: artist expected %q, found %q; album expected %q, found %q; original file retained", change.Edit.Artist, written["artist"], expectedAlbum, written["album"])
			}
		}
	}
	// Compare encoded audio packets, rather than container bytes altered by tags.
	oldAudio, e := metadataAudioHash(ctx, audioPath)
	if e != nil {
		return e
	}
	newAudio, e := metadataAudioHash(ctx, output)
	if e != nil || oldAudio != newAudio {
		return errors.New("Audio verification failed; original file retained")
	}
	if e = os.Chmod(output, info.Mode().Perm()); e != nil {
		return e
	}
	f, e := os.Open(output)
	if e != nil {
		return e
	}
	e = f.Sync()
	f.Close()
	if e != nil {
		return e
	}
	after, e = hashFile(output)
	if e != nil {
		return e
	}
	if _, e = a.DB.Exec("INSERT INTO metadata_files(job_id,song_id,file_path,before_hash,after_hash) VALUES(?,?,?,?,?) ON CONFLICT(job_id,song_id) DO UPDATE SET after_hash=excluded.after_hash", jid, change.Song.ID, rel, before, after); e != nil {
		return e
	}
	if e = a.checkMusicParents(rel); e != nil {
		return e
	}
	check, e := hashFile(path)
	if e != nil || check != before {
		return errors.New("Source changed during tag writing; original file retained")
	}
	if change.MergeKind == "credits" {
		// Serialize the final decision with dismissals. A staged write must
		// not publish a split after the user has marked that name indivisible.
		a.metadataDecisionMu.Lock()
		defer a.metadataDecisionMu.Unlock()
		latest, skip, err := a.filterDismissedCreditRepairs(c, change)
		if err != nil {
			return err
		}
		planned := len(change.CreditRepairs)
		if planned == 0 && change.CreditRepair != nil {
			planned = 1
		}
		if skip || len(latest.CreditRepairs) != planned {
			return errCreditRepairDismissed
		}
	}
	if e = os.Rename(output, path); e != nil {
		return e
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return err
	}
	{
		change.Edit.RemoveCover = false
		change.Edit.CoverUploadID = ""
		var err error
		if change.Enrichment != "" {
			_, err = a.DB.Exec("INSERT INTO metadata_edits(song_id,metadata,actor) SELECT ?,?,actor FROM metadata_jobs WHERE id=? ON CONFLICT(song_id) DO UPDATE SET metadata=excluded.metadata,actor=excluded.actor,updated=CURRENT_TIMESTAMP", change.Song.ID, mustJSON(change.Edit), jid)
		} else {
			_, err = a.DB.Exec("UPDATE metadata_edits SET metadata=? WHERE song_id=?", mustJSON(change.Edit), change.Song.ID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func metadataAudioHash(ctx context.Context, path string) (string, error) {
	out, e := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-protocol_whitelist", "file,pipe", "-i", path, "-map", "0:a:0", "-c:a", "copy", "-f", "hash", "-hash", "sha256", "-").Output()
	if e != nil {
		return "", errors.New("Cannot verify encoded audio")
	}
	return strings.TrimSpace(string(out)), nil
}
func metadataCoverHost(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") && (u.Hostname() == "archive.org" || strings.HasSuffix(u.Hostname(), ".archive.org") || u.Hostname() == "coverartarchive.org")
}
func (a *App) downloadMetadataCover(ctx context.Context, raw, path string) error {
	u, e := url.Parse(raw)
	if e != nil {
		return errors.New("Use a public HTTPS image URL")
	}
	if err := validateArtworkURL(ctx, u); err != nil {
		return err
	}
	client := a.artworkClient()
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return e
	}
	res, e := client.Do(req)
	if e != nil {
		return errors.New("Could not download cover")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("Cover is unavailable")
	}
	bytes, e := io.ReadAll(io.LimitReader(res.Body, (20<<20)+1))
	if e != nil || len(bytes) > 20<<20 {
		return errors.New("Cover exceeds 20 MiB")
	}
	config, _, e := image.DecodeConfig(strings.NewReader(string(bytes)))
	if e != nil || int64(config.Width)*int64(config.Height) > 50000000 {
		return errors.New("Cover must be a JPEG or PNG under 50 megapixels")
	}
	return os.WriteFile(path, bytes, 0600)
}

func (a *App) metadataCanonicalTags(ctx context.Context, c Credentials, target Song) (map[string]string, string, error) {
	remote, e := a.nav(ctx, c, "getSong", url.Values{"id": {target.ID}, "realPath": {"true"}})
	if e != nil {
		return nil, "", e
	}
	if remote.Song.ID != target.ID {
		return nil, "", errors.New("Merge destination no longer exists")
	}
	rel, _, e := a.deletionPath(ctx, c, remote.Song)
	if e != nil {
		return nil, "", e
	}
	out, e := exec.CommandContext(ctx, a.ffprobePath, "-v", "error", "-show_format", "-show_streams", "-of", "json", filepath.Join(a.musicDir, rel)).Output()
	if e != nil {
		return nil, "", errors.New("Cannot read destination grouping tags")
	}
	var p probe
	if e = json.Unmarshal(out, &p); e != nil {
		return nil, "", e
	}
	return uploadTags(p), filepath.Join(a.musicDir, rel), nil
}
