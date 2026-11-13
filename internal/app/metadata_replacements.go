package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"zensync/internal/apple"
)

type recordingReplacement struct {
	AllowFormatChange bool   `json:"allowFormatChange"`
	ProviderTarget    string `json:"providerTarget,omitempty"`
	Track             Track  `json:"track,omitempty"`
	Quality           string `json:"quality,omitempty"`
	InputPath         string `json:"inputPath,omitempty"`
	Checksum          string `json:"checksum,omitempty"`
	UploadID          string `json:"uploadId,omitempty"`
}

func libraryReplacement(kind string, allow bool) *recordingReplacement {
	if kind != "song" {
		return nil
	}
	return &recordingReplacement{AllowFormatChange: allow}
}
func (a *App) replacementInputPath(input *recordingReplacement) (string, error) {
	root := filepath.Join(a.dataDir, "staging")
	if input.UploadID != "" {
		root = filepath.Join(a.dataDir, "metadata-inputs")
	}
	rel, err := filepath.Rel(root, input.InputPath)
	if err != nil || !safeRelativePath(rel) {
		return "", errors.New("Invalid replacement input path")
	}
	info, err := os.Lstat(input.InputPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("Replacement audio is missing or unsafe; select it again")
	}
	checksum, err := hashFile(input.InputPath)
	if err != nil || checksum != input.Checksum {
		return "", errors.New("Replacement audio changed; select it again")
	}
	return input.InputPath, nil
}
func (a *App) queueExternalReplacement(w http.ResponseWriter, r *http.Request, u User, source, providerTarget, uploadID string, allow bool) {
	if source == "" || (providerTarget == "") == (uploadID == "") {
		fail(w, 400, "Choose one replacement recording")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	current, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {source}, "realPath": {"true"}})
	if err != nil || current.Song.ID != source {
		fail(w, 404, "Source recording is unavailable")
		return
	}
	input := &recordingReplacement{AllowFormatChange: allow, Quality: string(u.Quality), UploadID: uploadID}
	if uploadID != "" {
		a.importMu.Lock()
		defer a.importMu.Unlock()
		var raw string
		err = a.DB.QueryRow("SELECT path,checksum,metadata FROM metadata_audio_inputs WHERE id=? AND owner=? AND source_id=?", uploadID, u.ID, source).Scan(&input.InputPath, &input.Checksum, &raw)
		if err != nil || json.Unmarshal([]byte(raw), &input.Track) != nil {
			fail(w, 404, "Uploaded replacement is unavailable")
			return
		}
		if _, err = a.replacementInputPath(input); err != nil {
			fail(w, 409, err.Error())
			return
		}
	} else {
		input.Track, err = a.track(r.Context(), providerTarget)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		p, err := a.mediaProvider(input.Track.Provider)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		if err = p.Ready(); err != nil {
			fail(w, 409, err.Error())
			return
		}
		input.ProviderTarget = trackTarget(input.Track)
	}
	// External native formats are measured after download. Confirmation covers
	// any native format change, rather than promising a provider's format.
	if !allow {
		fail(w, 409, "Confirm that the replacement may change the audio format and Navidrome ID")
		return
	}
	edit, err := a.existingMetadataEdit(current.Song)
	if err != nil {
		a.internal(w, err)
		return
	}
	jid, err := a.queueMetadata([]metadataChange{{Song: current.Song, Edit: edit, MergeKind: "song", Replacement: input}}, u)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	send(w, 202, map[string]any{"id": jid, "songs": 1})
}
func (a *App) prepareProviderReplacement(ctx context.Context, jid string, change *metadataChange) error {
	input := change.Replacement
	if input.InputPath != "" {
		if _, err := a.replacementInputPath(input); err == nil {
			return nil
		}
		// Native acquisition revalidates or redownloads private staging if
		// an interrupted transfer was lost. Never reuse changed bytes.
		input.InputPath, input.Checksum = "", ""
	}
	sink := func(path string, _ Track, _ string) (string, error) {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		err = file.Sync()
		file.Close()
		if err != nil {
			return "", err
		}
		if err = syncDir(filepath.Dir(path)); err != nil {
			return "", err
		}
		checksum, err := hashFile(path)
		if err != nil {
			return "", err
		}
		input.InputPath = path
		input.Checksum = checksum
		return path, nil
	}
	var err error
	switch input.Track.Provider {
	case "", "deezer":
		var arl string
		if a.unseal("deezer", &arl) != nil || arl == "" {
			return errors.New("Configure Deezer in Settings to acquire music")
		}
		_, _, err = a.acquire(ctx, jid, input.ProviderTarget, input.Track, input.Quality, arl, sink)
	case "lossless":
		p := losslessProvider{a}
		_, _, err = a.acquireNative(ctx, jid, input.ProviderTarget, input.Track, input.Quality, "lossless", "Lossless.am", func() (*http.Response, error) {
			return p.request(ctx, "GET", "/api/download/track?"+url.Values{"id": {fmt.Sprint(input.Track.ID)}, "source": {input.Track.Source}}.Encode(), nil)
		}, sink)
	case "hitmo":
		p := hitmoProvider{a}
		var recording hitmoRecording
		recording, err = p.recording(ctx, input.ProviderTarget)
		if err != nil {
			return err
		}
		if audioApprovalKey(input.Track) != audioApprovalKey(recording.track) {
			return errors.New("Hitmo recording changed since selection")
		}
		_, _, err = a.acquireNative(ctx, jid, input.ProviderTarget, input.Track, input.Quality, "hitmo", "Hitmo", func() (*http.Response, error) {
			return p.request(ctx, strings.TrimPrefix(recording.download, hitmoOrigin))
		}, sink)
	default:
		return errors.New("Unknown replacement provider")
	}
	return err
}

// Publish a format change without overwriting another library file. The new
// bytes are durable before the old path is removed. Reconciliation resumes a
// crash between these steps using the hashes already stored in metadata_files.
func (a *App) publishFormatChange(jid, song, oldRel, newRel, output string) error {
	if !safeRelativePath(newRel) {
		return errors.New("Unsafe replacement destination")
	}
	if err := a.checkMusicParents(newRel); err != nil {
		return err
	}
	newPath := filepath.Join(a.musicDir, newRel)
	if _, err := os.Lstat(newPath); !os.IsNotExist(err) {
		return errors.New("The new audio filename already exists; choose another recording or resolve the filename collision")
	}
	_, err := a.DB.Exec("INSERT INTO metadata_format_changes(job_id,song_id,old_path,new_path) VALUES(?,?,?,?) ON CONFLICT(job_id,song_id) DO UPDATE SET new_path=excluded.new_path", jid, song, oldRel, newRel)
	if err != nil {
		return err
	}
	// A hard link is atomic and fails if the destination appears concurrently.
	if err = os.Link(output, newPath); err != nil {
		return err
	}
	if err = syncDir(filepath.Dir(newPath)); err != nil {
		return err
	}
	_, err = a.reconcileFormatChange(jid, song)
	return err
}
func (a *App) reconcileFormatChange(jid, song string) (bool, error) {
	var oldRel, newRel, state, before, after string
	err := a.DB.QueryRow("SELECT f.old_path,f.new_path,f.state,m.before_hash,m.after_hash FROM metadata_format_changes f JOIN metadata_files m ON m.job_id=f.job_id AND m.song_id=f.song_id WHERE f.job_id=? AND f.song_id=?", jid, song).Scan(&oldRel, &newRel, &state, &before, &after)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !safeRelativePath(oldRel) || !safeRelativePath(newRel) {
		return false, errors.New("Unsafe format change journal")
	}
	if err = a.checkMusicParents(newRel); err != nil {
		return false, err
	}
	newPath, oldPath := filepath.Join(a.musicDir, newRel), filepath.Join(a.musicDir, oldRel)
	info, err := os.Lstat(newPath)
	if os.IsNotExist(err) && state == "prepared" {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return false, errors.New("Published replacement is missing or unsafe")
	}
	actual, err := hashFile(newPath)
	if err != nil || actual != after {
		return false, errors.New("Published replacement changed; review files before retrying")
	}
	if state != "completed" {
		if err = a.checkMusicParents(oldRel); err != nil {
			return false, err
		}
		if info, statErr := os.Lstat(oldPath); statErr == nil {
			if !info.Mode().IsRegular() {
				return false, errors.New("Original file is unsafe")
			}
			actual, err = hashFile(oldPath)
			if err != nil || actual != before {
				return false, errors.New("Original file changed; both recordings retained")
			}
			if err = os.Remove(oldPath); err != nil {
				return false, err
			}
		} else if !os.IsNotExist(statErr) {
			return false, statErr
		}
		if err = syncDir(filepath.Dir(oldPath)); err != nil {
			return false, err
		}
		tx, err := a.DB.Begin()
		if err != nil {
			return false, err
		}
		defer tx.Rollback()
		// Clear stale quality and server identity. The incremental scan reindexes
		// the new extension; an acquisition must not reuse an obsolete asset ID.
		if _, err = tx.Exec("UPDATE assets SET file_path=?,quality='',server_id='' WHERE file_path=?", newRel, oldRel); err != nil {
			return false, err
		}
		if _, err = tx.Exec("UPDATE metadata_format_changes SET state='completed' WHERE job_id=? AND song_id=?", jid, song); err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (a *App) replacementUpload(w http.ResponseWriter, r *http.Request, u User) {
	source := r.URL.Query().Get("source")
	if source == "" {
		fail(w, 400, "Choose a library song to replace")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	current, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {source}})
	if err != nil || current.Song.ID != source {
		fail(w, 404, "Source recording is unavailable")
		return
	}
	filename, err := url.QueryUnescape(r.Header.Get("X-File-Name"))
	if err != nil {
		fail(w, 400, "Invalid filename")
		return
	}
	ext := strings.ToLower(filepath.Ext(filename))
	format := uploadFormat(ext)
	if format == "" {
		fail(w, 400, "Choose FLAC, MP3, M4A, OGG, OPUS, WAV or AIFF audio")
		return
	}
	a.cleanReplacementInputs()
	if !a.importMu.TryLock() {
		fail(w, 429, "Another upload is being read; retry shortly")
		return
	}
	defer a.importMu.Unlock()
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(5 * time.Minute))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(8 * time.Minute))
	if r.ContentLength > maxAudioUpload {
		fail(w, 413, "Audio exceeds 1 GiB")
		return
	}
	uid := id()
	stage := filepath.Join(a.dataDir, "metadata-inputs", uid)
	if err = os.MkdirAll(stage, 0700); err != nil {
		a.internal(w, err)
		return
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	space := int64(maxAudioUpload*2 + 64<<20)
	if r.ContentLength > 0 {
		space = r.ContentLength*2 + 64<<20
	}
	if err = diskHeadroom(stage, uint64(space)); err != nil {
		fail(w, 507, err.Error())
		return
	}
	rawPath := filepath.Join(stage, "input"+ext)
	output := filepath.Join(stage, "audio"+ext)
	file, err := os.OpenFile(rawPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		a.internal(w, err)
		return
	}
	_, err = io.Copy(file, http.MaxBytesReader(w, r.Body, maxAudioUpload))
	file.Close()
	if err != nil {
		fail(w, 413, "Audio exceeds 1 GiB or could not be read")
		return
	}
	track, err := a.prepareAudioUpload(r.Context(), rawPath, output, format, apple.Song{Title: current.Song.Title, Artist: current.Song.Artist, Album: current.Song.Album})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	f, err := os.Open(output)
	if err != nil {
		a.internal(w, err)
		return
	}
	err = f.Sync()
	f.Close()
	if err == nil {
		err = syncDir(stage)
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	checksum, err := hashFile(output)
	if err != nil {
		a.internal(w, err)
		return
	}
	if _, err = a.DB.Exec("INSERT INTO metadata_audio_inputs(id,owner,source_id,path,checksum,metadata) VALUES(?,?,?,?,?,?)", uid, u.ID, source, output, checksum, mustJSON(track)); err != nil {
		a.internal(w, err)
		return
	}
	_ = os.Remove(rawPath)
	keep = true
	send(w, 201, map[string]string{"id": uid})
}
func (a *App) replacementInput(w http.ResponseWriter, r *http.Request, u User, uid, action string) {
	var file, raw, checksum string
	if a.DB.QueryRow("SELECT path,metadata,checksum FROM metadata_audio_inputs WHERE id=? AND owner=?", uid, u.ID).Scan(&file, &raw, &checksum) != nil {
		fail(w, 404, "Upload not found")
		return
	}
	if action == "audio" || action == "cover" {
		if _, err := a.replacementInputPath(&recordingReplacement{InputPath: file, Checksum: checksum, UploadID: uid}); err != nil {
			fail(w, 409, err.Error())
			return
		}
		if action == "cover" {
			file += ".jpg"
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, file)
		return
	}
	var track Track
	if json.Unmarshal([]byte(raw), &track) != nil {
		fail(w, 409, "Upload metadata is unavailable")
		return
	}
	_, coverErr := os.Stat(file + ".jpg")
	send(w, 200, map[string]any{"id": uid, "track": track, "format": strings.TrimPrefix(filepath.Ext(file), "."), "cover": coverErr == nil})
}

// Abandoned uploads expire after a day. Completed inputs can be removed sooner;
// queued, running and failed jobs retain bytes for a verified retry.
func (a *App) cleanReplacementInputs() {
	a.importMu.Lock()
	defer a.importMu.Unlock()
	rows, err := a.DB.Query(`SELECT i.id,i.path FROM metadata_audio_inputs i
 WHERE NOT EXISTS(SELECT 1 FROM metadata_jobs j,json_each(j.changes) c WHERE j.state<>'completed' AND json_extract(c.value,'$.replacement.uploadId')=i.id)
 AND (i.created<datetime('now','-1 day') OR EXISTS(SELECT 1 FROM metadata_jobs j,json_each(j.changes) c WHERE j.state='completed' AND json_extract(c.value,'$.replacement.uploadId')=i.id))`)
	if err != nil {
		return
	}
	type entry struct{ id, path string }
	var inputs []entry
	for rows.Next() {
		var input entry
		if rows.Scan(&input.id, &input.path) != nil {
			rows.Close()
			return
		}
		inputs = append(inputs, input)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	for _, input := range inputs {
		expected := filepath.Join(a.dataDir, "metadata-inputs", input.id)
		if filepath.Dir(input.path) != expected {
			continue
		}
		if os.RemoveAll(expected) == nil {
			_, _ = a.DB.Exec("DELETE FROM metadata_audio_inputs WHERE id=? AND path=?", input.id, input.path)
		}
	}
}
