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
	Song      Song         `json:"song"`
	Edit      MetadataEdit `json:"edit"`
	MergeKind string       `json:"mergeKind,omitempty"`
	Target    *Song        `json:"target,omitempty"`
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
	var busy int
	if e = tx.QueryRow("SELECT count(*) FROM metadata_jobs WHERE state IN ('queued','running')").Scan(&busy); e != nil {
		return "", e
	}
	if busy > 0 {
		return "", errors.New("A metadata update is already queued or running; wait for it to finish")
	}
	jid := id()
	for _, change := range changes {
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
func (a *App) mergeMetadata(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Kind   string `json:"kind"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Kind != "artist" && in.Kind != "album") || in.Source == "" || in.Target == "" || in.Source == in.Target {
		fail(w, 400, "Choose distinct source and destination artists or albums")
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
		if in.Kind == "album" {
			key = s.AlbumID
		}
		if key == in.Target {
			target = s
		}
		if key == in.Source {
			source = append(source, s)
		}
	}
	if target.ID == "" || len(source) == 0 {
		fail(w, 404, "Source or destination no longer exists; refresh candidates")
		return
	}
	changes := []metadataChange{}
	for _, s := range source {
		edit := MetadataEdit{Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumArtist: s.AlbumArtist}
		var raw string
		e = a.DB.QueryRow("SELECT metadata FROM metadata_edits WHERE song_id=?", s.ID).Scan(&raw)
		if e == nil {
			if e = json.Unmarshal([]byte(raw), &edit); e != nil {
				a.internal(w, e)
				return
			}
		} else if e != sql.ErrNoRows {
			a.internal(w, e)
			return
		}
		if in.Kind == "artist" {
			edit.Artist = target.Artist
			// A track artist merge must not move compilation tracks into a
			// different album artist's discography.
			if edit.AlbumArtist == "" || metadataKey(edit.AlbumArtist) == metadataKey(s.Artist) {
				edit.AlbumArtist = target.Artist
			}
		} else {
			edit.Album = target.Album
			edit.AlbumArtist = target.AlbumArtist
			if edit.AlbumArtist == "" {
				edit.AlbumArtist = target.Artist
			}
		}
		changes = append(changes, metadataChange{Song: s, Edit: edit, MergeKind: in.Kind, Target: &target})
	}
	jid, e := a.queueMetadata(changes, u)
	if e != nil {
		fail(w, 409, e.Error())
		return
	}
	slog.Info("Metadata merge queued", "job_id", jid, "kind", in.Kind, "source_id", in.Source, "target_id", in.Target, "target_artist", target.Artist, "target_album", target.Album, "songs", len(changes))
	send(w, 202, map[string]any{"id": jid, "songs": len(changes)})
}
func (a *App) metadataJobRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, e := a.DB.Query("SELECT id,state,message,done,json_array_length(changes) FROM metadata_jobs ORDER BY rowid DESC LIMIT 20")
		if e != nil {
			a.internal(w, e)
			return
		}
		defer rows.Close()
		type job struct {
			ID      string `json:"id"`
			State   string `json:"state"`
			Message string `json:"message"`
			Done    int    `json:"done"`
			Total   int    `json:"total"`
		}
		jobs := []job{}
		for rows.Next() {
			var j job
			if e = rows.Scan(&j.ID, &j.State, &j.Message, &j.Done, &j.Total); e != nil {
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
		result, e := a.DB.Exec("UPDATE metadata_jobs SET state='queued',message='Retry queued' WHERE id=? AND state='failed' AND NOT EXISTS (SELECT 1 FROM metadata_jobs WHERE state IN ('queued','running'))", in.ID)
		if e != nil {
			a.internal(w, e)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			fail(w, 409, "Only a failed operation can be retried, with no other metadata operation active")
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
		logger.Info("Metadata song update started", "song_id", change.Song.ID, "kind", change.MergeKind, "artist_before", change.Song.Artist, "artist_after", change.Edit.Artist, "album_artist_before", change.Song.AlbumArtist, "album_artist_after", change.Edit.AlbumArtist)
		e = a.publishMetadata(ctx, c, jid, change)
		if e != nil {
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
		_, e = a.nav(ctx, c, "startScan", mapValues("fullScan", "true"))
		if e == nil {
			logger.Info("Metadata full library scan requested")
		}
	}
	state, message := "completed", "Tags updated; Navidrome scan requested. Refresh the library after scanning."
	if e != nil {
		state = "failed"
		message = fmt.Sprintf("%d/%d songs updated: %s. Recovery copies retained; retry resumes remaining work.", done, len(changes), e.Error())
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
	key := jid + "-" + digest(change.Song.ID)
	dir := filepath.Join(a.dataDir, "metadata-backups", key)
	music, e := filepath.EvalSymlinks(a.musicDir)
	if e != nil {
		return e
	}
	data, e := filepath.EvalSymlinks(a.dataDir)
	if e != nil {
		return e
	}
	if inside, _ := filepath.Rel(music, data); inside == "." || (!strings.HasPrefix(inside, ".."+string(filepath.Separator)) && inside != "..") {
		return errors.New("Metadata recovery storage must be outside the music library")
	}
	var rel, before, after string
	e = a.DB.QueryRow("SELECT file_path,before_hash,after_hash FROM metadata_files WHERE job_id=? AND song_id=?", jid, change.Song.ID).Scan(&rel, &before, &after)
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
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	backup := filepath.Join(dir, "original"+filepath.Ext(path))
	if _, e = os.Stat(backup); os.IsNotExist(e) {
		if e = a.backupMetadataFile(path, backup, before); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	if hash, e = hashFile(backup); e != nil || hash != before {
		return errors.New("Recovery copy verification failed")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".flac", ".m4a", ".ogg", ".opus", ".wav", ".aiff":
	default:
		return errors.New("Unsupported tag format; use MP3, FLAC, M4A, OGG, OPUS, WAV or AIFF")
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".zensync-tags-*"+ext)
	if e != nil {
		return e
	}
	output := tmp.Name()
	tmp.Close()
	os.Remove(output)
	defer os.Remove(output)
	args := []string{"-v", "error", "-nostdin", "-protocol_whitelist", "file,pipe", "-i", backup}
	useCover := change.Edit.CoverURL != ""
	if useCover {
		if ext != ".mp3" && ext != ".flac" && ext != ".m4a" {
			return errors.New("Cover embedding currently supports MP3, FLAC and M4A")
		}
		cover := filepath.Join(dir, "selected-cover")
		if e = a.downloadMetadataCover(ctx, change.Edit.CoverURL, cover); e != nil {
			return e
		}
		args = append(args, "-i", cover, "-map", "0:a", "-map", "1:v:0", "-c:v", "mjpeg", "-disposition:v:0", "attached_pic")
	} else {
		args = append(args, "-map", "0", "-c:v", "copy")
	}
	args = append(args, "-map_metadata", "0", "-c:a", "copy")
	tags := map[string]string{"title": change.Edit.Title, "artist": change.Edit.Artist, "album": change.Edit.Album}
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
	if change.Edit.Artist != change.Song.Artist {
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
	if change.MergeKind != "artist" && (change.Edit.Album != change.Song.Album || change.Edit.Artist != change.Song.Artist) {
		tags["musicbrainz_albumid"] = ""
		tags["musicbrainz_releasegroupid"] = ""
	}
	if change.MergeKind != "" && change.Target != nil {
		canonical, e := a.metadataCanonicalTags(ctx, c, *change.Target)
		if e != nil {
			return e
		}
		if change.MergeKind == "album" {
			for _, key := range []string{"musicbrainz_albumid", "musicbrainz album id", "musicbrainz_releasegroupid", "musicbrainz release group id", "musicbrainz_albumartistid", "musicbrainz album artist id", "albumartist", "album_artist", "albumartists", "albumversion", "album_version", "releasedate", "date", "year", "originaldate", "originalyear", "compilation"} {
				tags[key] = canonical[key]
			}
			tags["album_artist"] = change.Edit.AlbumArtist
		} else {
			tags["artists"] = change.Edit.Artist
			for _, key := range []string{"musicbrainz_artistid", "musicbrainz artist id"} {
				tags[key] = canonical[key]
			}
			if change.Edit.AlbumArtist == change.Edit.Artist {
				tags["albumartist"] = change.Edit.Artist
				tags["album_artist"] = change.Edit.Artist
				tags["albumartists"] = change.Edit.Artist
				// The destination track may itself belong to a compilation.
				// Use its track artist identity, not its album artist identity.
				tags["musicbrainz_albumartistid"] = canonical["musicbrainz_artistid"]
				tags["musicbrainz album artist id"] = canonical["musicbrainz artist id"]
			} else {
				tags["albumartists"] = change.Edit.AlbumArtist
				// Restore album artist identity cleared by the general edit path.
				original, err := a.metadataCanonicalTags(ctx, c, change.Song)
				if err != nil {
					return err
				}
				for _, key := range []string{"albumartist", "musicbrainz_albumartistid", "musicbrainz album artist id"} {
					tags[key] = original[key]
				}
			}
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
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Error("Metadata tag writer failed", "job_id", jid, "song_id", change.Song.ID, "error", err, "detail", string(output[:min(len(output), 2000)]))
		return errors.New("Could not write audio tags; original file retained")
	}
	if change.MergeKind != "" {
		p, err := a.probeUpload(output, strings.TrimPrefix(ext, "."))
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
		if written["artist"] != change.Edit.Artist || written["album"] != change.Edit.Album {
			return errors.New("Written artist or album did not match; original file retained")
		}
	}
	// Compare encoded audio packets, rather than container bytes altered by tags.
	oldAudio, e := metadataAudioHash(ctx, backup)
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
	if e = os.Rename(output, path); e != nil {
		return e
	}
	return syncDir(filepath.Dir(path))
}
func (a *App) backupMetadataFile(source, dest, checksum string) error {
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	info, e := in.Stat()
	if e != nil {
		return e
	}
	if e = diskHeadroom(filepath.Dir(dest), uint64(info.Size())*2+(32<<20)); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(dest), ".backup-")
	if e != nil {
		return e
	}
	defer tmp.Close()
	defer os.Remove(tmp.Name())
	if _, e = io.Copy(tmp, in); e != nil {
		return e
	}
	if e = tmp.Sync(); e != nil {
		return e
	}
	if hash, e := hashFile(tmp.Name()); e != nil || hash != checksum {
		return errors.New("Source changed while creating recovery copy")
	}
	if e = os.Link(tmp.Name(), dest); e != nil {
		return e
	}
	return syncDir(filepath.Dir(dest))
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
	if e != nil || !metadataCoverHost(u) {
		return errors.New("Choose an HTTPS cover from Cover Art Archive or archive.org")
	}
	client := *a.HTTP
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !metadataCoverHost(r.URL) {
			return errors.New("Unsafe cover redirect")
		}
		return nil
	}
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

func (a *App) metadataCanonicalTags(ctx context.Context, c Credentials, target Song) (map[string]string, error) {
	remote, e := a.nav(ctx, c, "getSong", url.Values{"id": {target.ID}, "realPath": {"true"}})
	if e != nil {
		return nil, e
	}
	if remote.Song.ID != target.ID {
		return nil, errors.New("Merge destination no longer exists")
	}
	rel, _, e := a.deletionPath(ctx, c, remote.Song)
	if e != nil {
		return nil, e
	}
	out, e := exec.CommandContext(ctx, a.ffprobePath, "-v", "error", "-show_format", "-show_streams", "-of", "json", filepath.Join(a.musicDir, rel)).Output()
	if e != nil {
		return nil, errors.New("Cannot read destination grouping tags")
	}
	var p probe
	if e = json.Unmarshal(out, &p); e != nil {
		return nil, e
	}
	return uploadTags(p), nil
}
