package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"zensync/internal/apple"
)

const maxAudioUpload = 1 << 30

func uploadFormat(ext string) string {
	switch ext {
	case ".flac":
		return "flac"
	case ".mp3":
		return "mp3"
	case ".m4a":
		return "mov"
	case ".ogg", ".opus":
		return "ogg"
	case ".wav":
		return "wav"
	case ".aif", ".aiff":
		return "aiff"
	}
	return ""
}

type uploadProbeOutput struct{ bytes.Buffer }

func (output *uploadProbeOutput) Write(data []byte) (int, error) {
	if output.Len()+len(data) > 1<<20 {
		return 0, errors.New("Audio metadata exceeds 1 MiB")
	}
	return output.Buffer.Write(data)
}

func (a *App) probeUpload(path, format string) (probe, error) {
	var p probe
	switch format {
	case "flac", "mp3", "mov", "ogg", "wav", "aiff":
	default:
		return p, errors.New("Unsupported audio format")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 || info.Size() > maxAudioUpload {
		return p, errors.New("Audio must be a regular file between 1 KiB and 1 GiB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Force the expected demuxer; a renamed playlist cannot fetch external media.
	command := exec.CommandContext(ctx, a.ffprobePath, "-v", "error", "-protocol_whitelist", "file,pipe", "-f", format, "-show_format", "-show_streams", "-of", "json", path)
	var output uploadProbeOutput
	command.Stdout = &output
	if command.Run() != nil || json.Unmarshal(output.Bytes(), &p) != nil {
		return p, errors.New("Cannot read this audio file")
	}
	valid := false
	for _, stream := range p.Streams {
		if stream.Type == "audio" {
			valid = true
		}
	}
	duration, _ := strconv.ParseFloat(p.Format.Duration, 64)
	if !valid || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return p, errors.New("File has no valid audio duration")
	}
	return p, nil
}

func uploadTags(p probe) map[string]string {
	tags := map[string]string{}
	for key, value := range p.Format.Tags {
		tags[strings.ToLower(key)] = value
	}
	for _, stream := range p.Streams {
		if stream.Type == "audio" {
			for key, value := range stream.Tags {
				tags[strings.ToLower(key)] = value
			}
			break
		}
	}
	return tags
}

func (a *App) prepareAudioUpload(ctx context.Context, input, output, format string, source apple.Song) (Track, error) {
	var track Track
	p, err := a.probeUpload(input, format)
	if err != nil {
		return track, err
	}
	tags := uploadTags(p)
	fallback := func(values ...string) string {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
		return ""
	}
	track.Title = fallback(tags["title"], source.Title, "Untitled")
	track.Artist.Name = fallback(tags["artist"], source.Artist, "Unknown artist")
	track.Album.Title = fallback(tags["album"], source.Album, "Unknown album")
	track.Duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	if len(track.Title) > 300 || len(track.Artist.Name) > 300 || len(track.Album.Title) > 300 {
		return track, errors.New("Audio tags exceed 300 characters")
	}
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	// Copy the audio stream without re-encoding and fill missing tags from the
	// import. Uploaded versions keep their own title, artist, album and duration.
	args := []string{"-v", "error", "-nostdin", "-protocol_whitelist", "file,pipe", "-f", format, "-i", input, "-map", "0:a:0", "-map_metadata", "0", "-c:a", "copy", "-metadata", "title=" + track.Title, "-metadata", "artist=" + track.Artist.Name, "-metadata", "album=" + track.Album.Title}
	cover := uploadedCoverStream(p)
	if cover >= 0 && (format == "mp3" || format == "flac") {
		args = append(args, "-map", "0:"+strconv.Itoa(cover), "-c:v", "copy", "-disposition:v:0", "attached_pic")
	}
	if format == "aiff" {
		args = append(args, "-write_id3v2", "1")
	}
	args = append(args, output)
	command := exec.CommandContext(runCtx, "ffmpeg", args...)
	protectChild(command)
	if command.Run() != nil {
		return track, errors.New("Could not prepare this audio file; try FLAC, MP3 or M4A")
	}
	if cover >= 0 && (format == "mp3" || format == "flac") {
		a.extractUploadedCover(runCtx, output, format)
	}
	return track, a.validateUploadedAudio(output, track, format)
}

func uploadedCoverStream(p probe) int {
	selected := -1
	for _, stream := range p.Streams {
		if stream.Type != "video" || stream.Disposition.AttachedPic != 1 {
			continue
		}
		if selected < 0 {
			selected = stream.Index
		}
		for key, value := range stream.Tags {
			if strings.EqualFold(key, "comment") && strings.Contains(strings.ToLower(value), "front") {
				return stream.Index
			}
		}
	}
	return selected
}

// The embedded original remains in the audio. This bounded JPEG thumbnail is
// only a preview; unreadable artwork must not reject otherwise valid audio.
func (a *App) extractUploadedCover(ctx context.Context, audio, format string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output := audio + ".jpg"
	command := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-max_alloc", "67108864", "-protocol_whitelist", "file,pipe", "-f", format, "-i", audio, "-map", "0:v:0", "-frames:v", "1", "-vf", "scale=512:512:force_original_aspect_ratio=decrease", "-c:v", "mjpeg", "-threads", "1", "-fs", "1048576", output)
	protectChild(command)
	if command.Run() != nil {
		_ = os.Remove(output)
	}
}

func (a *App) uploadedCover(w http.ResponseWriter, r *http.Request, u User) {
	rid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v2/requests/"), "/cover")
	var target string
	if a.DB.QueryRow("SELECT r.target FROM requests r JOIN audio_uploads a ON a.target=r.target WHERE r.id=? AND (r.owner=? OR ?='admin')", rid, u.ID, u.Role).Scan(&target) != nil {
		fail(w, 404, "Cover not found")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(a.dataDir, "covers", digest(target)+".jpg"))
}

func (a *App) validateUploadedAudio(path string, track Track, format string) error {
	p, err := a.probeUpload(path, format)
	if err != nil {
		return err
	}
	duration, _ := strconv.ParseFloat(p.Format.Duration, 64)
	tags := uploadTags(p)
	if math.Abs(duration-track.Duration) > 3 || normalized(tags["title"]) != normalized(track.Title) || normalized(tags["artist"]) != normalized(track.Artist.Name) || normalized(tags["album"]) != normalized(track.Album.Title) {
		return errors.New("Uploaded audio no longer matches its verified metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-xerror", "-protocol_whitelist", "file,pipe", "-f", format, "-i", path, "-map", "0:a:0", "-f", "null", "-")
	protectChild(command)
	if command.Run() != nil {
		return errors.New("Full audio decoding failed; upload a complete audio file")
	}
	return nil
}

func (a *App) uploadSourceAudio(w http.ResponseWriter, r *http.Request, u User, mid, sid string) {
	var metadata, state string
	if err := a.DB.QueryRow("SELECT s.metadata,m.state FROM source_songs s JOIN migrations m ON m.id=s.migration_id WHERE s.id=? AND m.id=? AND m.owner=?", sid, mid, u.ID).Scan(&metadata, &state); err != nil {
		fail(w, 404, "Source song not found")
		return
	}
	if state != "open" {
		fail(w, 409, "Audio uploads require an open migration")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 128 {
		fail(w, 400, "An 8–128 character Idempotency-Key is required")
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
		fail(w, 400, "Choose a FLAC, MP3, M4A, OGG, OPUS, WAV or AIFF audio file")
		return
	}
	if !a.importMu.TryLock() {
		fail(w, 429, "Another import upload is being read; retry shortly")
		return
	}
	defer a.importMu.Unlock()
	// Large uploads need more time than normal API requests.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(5 * time.Minute))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(8 * time.Minute))
	stage := filepath.Join(a.dataDir, "uploads", id())
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
	if r.ContentLength > maxAudioUpload {
		fail(w, 413, "Audio exceeds 1 GiB")
		return
	}
	space := int64(maxAudioUpload*2 + 64<<20)
	if r.ContentLength > 0 {
		space = r.ContentLength*2 + 64<<20
	}
	if err = diskHeadroom(stage, uint64(space)); err != nil {
		fail(w, 507, err.Error())
		return
	}
	input := filepath.Join(stage, "source"+ext)
	file, err := os.OpenFile(input, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		a.internal(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioUpload)
	_, err = io.Copy(file, r.Body)
	if err == nil {
		err = file.Sync()
	}
	file.Close()
	if err != nil {
		fail(w, 413, "Audio exceeds 1 GiB or could not be read")
		return
	}
	var source apple.Song
	if json.Unmarshal([]byte(metadata), &source) != nil {
		a.internal(w, errors.New("Invalid source metadata"))
		return
	}
	output := filepath.Join(stage, "audio"+ext)
	track, err := a.prepareAudioUpload(r.Context(), input, output, format, source)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	validated, err := os.OpenFile(output, os.O_RDWR, 0600)
	if err != nil {
		a.internal(w, err)
		return
	}
	err = validated.Sync()
	validated.Close()
	if err != nil {
		a.internal(w, err)
		return
	}
	stageDir, err := os.Open(stage)
	if err != nil {
		a.internal(w, err)
		return
	}
	err = stageDir.Sync()
	stageDir.Close()
	if err != nil {
		a.internal(w, err)
		return
	}
	checksum, err := hashFile(output)
	if err != nil {
		a.internal(w, err)
		return
	}
	_ = os.Remove(input)
	target := "upload:" + digest(u.ID+":"+checksum)
	a.opMu.Lock()
	defer a.opMu.Unlock()
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	if err = tx.QueryRow("SELECT m.state FROM migrations m JOIN source_songs s ON s.migration_id=m.id WHERE m.id=? AND m.owner=? AND s.id=?", mid, u.ID, sid).Scan(&state); err != nil {
		fail(w, 404, "Source song no longer exists")
		return
	}
	if state != "open" {
		fail(w, 409, "Migration is no longer open")
		return
	}
	var existing, oldTarget string
	err = tx.QueryRow("SELECT r.id,r.target FROM requests r WHERE r.owner=? AND r.idem=?", u.ID, key).Scan(&existing, &oldTarget)
	if err == nil {
		var linked int
		if err = tx.QueryRow("SELECT count(*) FROM request_sources WHERE source_id=? AND request_id=?", sid, existing).Scan(&linked); err != nil {
			a.internal(w, err)
			return
		}
		if oldTarget != target || linked != 1 {
			fail(w, 409, "Upload key already used for another file or song")
			return
		}
		send(w, 200, map[string]string{"id": existing})
		return
	}
	if err != sql.ErrNoRows {
		a.internal(w, err)
		return
	}
	var pending int
	if err = tx.QueryRow(`SELECT count(*) FROM request_sources rs JOIN requests r ON r.id=rs.request_id LEFT JOIN jobs j ON j.request_id=r.id WHERE rs.source_id=? AND r.decision IN ('approved','awaiting_approval') AND COALESCE(j.state,'') NOT IN ('failed','cancelled','available')`, sid).Scan(&pending); err != nil {
		a.internal(w, err)
		return
	}
	if pending > 0 {
		fail(w, 409, "Withdraw or finish the existing request before uploading another recording")
		return
	}
	var role, quality string
	var direct bool
	if err = tx.QueryRow("SELECT role,direct,quality FROM users WHERE id=?", u.ID).Scan(&role, &direct, &quality); err != nil {
		a.internal(w, err)
		return
	}
	decision, reason := "awaiting_approval", ""
	if role == "admin" || direct {
		decision, reason = "approved", "Allowed without approval"
	}
	storedPath := output
	var previousPath string
	if tx.QueryRow("SELECT file_path FROM audio_uploads WHERE target=?", target).Scan(&previousPath) == nil && previousPath != "" {
		if previousChecksum, err := hashFile(previousPath); err == nil && previousChecksum == checksum {
			storedPath = previousPath
		}
	}
	rid := id()
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO audio_uploads(target,owner,file_path,metadata,quality,checksum) VALUES(?,?,?,?,?,?) ON CONFLICT(target) DO UPDATE SET file_path=excluded.file_path", []any{target, u.ID, storedPath, mustJSON(track), "uploaded:" + format, checksum}},
		{"INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision,reason) VALUES(?,?,?,?,?,?,0,?,?,?)", []any{rid, u.ID, key, target, track.Title, track.Artist.Name, quality, decision, reason}},
		{"INSERT INTO request_metadata(request_id,metadata) VALUES(?,?)", []any{rid, mustJSON(track)}},
		{"DELETE FROM request_sources WHERE source_id=?", []any{sid}},
		{"INSERT INTO request_sources(source_id,request_id) VALUES(?,?)", []any{sid, rid}},
		{"INSERT INTO source_resolutions(source_id,revision) VALUES(?,?) ON CONFLICT(source_id) DO UPDATE SET revision=excluded.revision", []any{sid, id()}},
		{"UPDATE source_songs SET server_id='',candidate='',match_state='manual',deferred=0,favorite_state='pending' WHERE id=?", []any{sid}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(statement.query, statement.args...); err != nil {
			a.internal(w, err)
			return
		}
	}
	if decision == "approved" {
		if _, err = tx.Exec("INSERT INTO jobs(id,request_id,state,stage) VALUES(?,?,'queued','Queued to import uploaded audio')", id(), rid); err != nil {
			a.internal(w, err)
			return
		}
	}
	if err = audit(tx, u.ID, u.ID, "source.audio_uploaded", sid); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	keep = storedPath == output
	// Covers stay available after staging is cleaned following indexing.
	if _, err := os.Stat(output + ".jpg"); err == nil {
		if os.MkdirAll(filepath.Join(a.dataDir, "covers"), 0700) == nil {
			_ = os.Chmod(output+".jpg", 0600)
			_ = os.Rename(output+".jpg", filepath.Join(a.dataDir, "covers", digest(target)+".jpg"))
		}
	}
	send(w, 201, map[string]string{"id": rid, "decision": decision})
}

func (a *App) placeUploadedAudio(jid, rid, target string) (string, string, string, error) {
	var file, metadata, quality, checksum, approved string
	err := a.DB.QueryRow("SELECT u.file_path,u.metadata,u.quality,u.checksum,rm.metadata FROM audio_uploads u JOIN requests r ON r.target=u.target AND r.owner=u.owner JOIN request_metadata rm ON rm.request_id=r.id WHERE r.id=? AND u.target=?", rid, target).Scan(&file, &metadata, &quality, &checksum, &approved)
	if err != nil || approved != metadata {
		return "", "", "", errors.New("Uploaded audio snapshot is missing or changed; upload it again")
	}
	actual, err := hashFile(file)
	if err != nil || actual != checksum {
		return "", "", "", errors.New("Uploaded audio is missing or changed; upload it again")
	}
	var track Track
	if json.Unmarshal([]byte(metadata), &track) != nil {
		return "", "", "", errors.New("Invalid uploaded recording metadata")
	}
	if err = a.validateAudio(file, track, quality); err != nil {
		return "", "", "", err
	}
	rel, err := a.placeAudio(jid, target, file, track, quality)
	return rel, quality, metadata, err
}

// Staged bytes are retained for retry until the request is completed or no
// longer approved/pending. Published audio has its own durable asset journal.
func (a *App) cleanAudioUpload(target string) {
	if !strings.HasPrefix(target, "upload:") {
		return
	}
	a.importMu.Lock()
	defer a.importMu.Unlock()
	var pending int
	if a.DB.QueryRow("SELECT count(*) FROM requests r LEFT JOIN jobs j ON j.request_id=r.id WHERE r.target=? AND (r.decision='awaiting_approval' OR (r.decision='approved' AND COALESCE(j.state,'')<>'available'))", target).Scan(&pending) != nil || pending > 0 {
		return
	}
	var file string
	if a.DB.QueryRow("SELECT file_path FROM audio_uploads WHERE target=?", target).Scan(&file) != nil || file == "" {
		return
	}
	rel, err := filepath.Rel(filepath.Join(a.dataDir, "uploads"), file)
	if err != nil || !safeRelativePath(rel) {
		return
	}
	_ = os.RemoveAll(filepath.Dir(file))
	_, _ = a.DB.Exec("UPDATE audio_uploads SET file_path='' WHERE target=? AND file_path=?", target, file)
}
