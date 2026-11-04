package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"zensync/internal/apple"
)

func mustJSON(v any) string { return apple.JSON(v) }
func trackMatches(t Track, s Song) bool {
	return normalized(trackLabel(t)) == normalized(s.Title) && artistMatches(t, s.Artist) && normalized(t.Album.Title) == normalized(s.Album) && t.Duration > 0 && math.Abs(t.Duration-s.Duration) <= 2
}

type probe struct {
	Streams []struct {
		Index       int `json:"index"`
		Disposition struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
		Codec    string            `json:"codec_name"`
		Duration string            `json:"duration"`
		Type     string            `json:"codec_type"`
		BitRate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func (a *App) validateAudio(path string, t Track, quality string) error {
	if strings.HasPrefix(quality, "uploaded:") {
		return a.validateUploadedAudio(path, t, strings.TrimPrefix(quality, "uploaded:"))
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() < 1024 || info.Size() > 1<<30 {
		return errors.New("Downloaded audio is missing, unsafe or exceeds 1 GiB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.ffprobePath, "-v", "error", "-show_format", "-show_streams", "-of", "json", path)
	data, e := cmd.Output()
	if e != nil {
		return errors.New("Audio validation failed; ffprobe could not read the file")
	}
	var p probe
	if json.Unmarshal(data, &p) != nil {
		return errors.New("Invalid audio probe output")
	}
	duration, _ := strconv.ParseFloat(p.Format.Duration, 64)
	valid := false
	for _, s := range p.Streams {
		if s.Type != "audio" {
			continue
		}
		if seconds, err := strconv.ParseFloat(s.Duration, 64); err == nil && validDuration(seconds) {
			duration = seconds
		}
		if quality == "lossless" {
			valid = s.Codec == "flac"
		} else {
			rate, _ := strconv.Atoi(s.BitRate)
			wanted, _ := strconv.Atoi(quality)
			valid = s.Codec == "mp3" && math.Abs(float64(rate-wanted*1000)) < 10000
		}
		break
	}
	if !valid {
		return errors.New("Downloaded codec/bitrate does not satisfy the selected quality")
	}
	tags := map[string]string{}
	for k, v := range p.Format.Tags {
		tags[strings.ToLower(k)] = v
	}
	if tags["sourceid"] != strconv.FormatInt(t.ID, 10) {
		return errors.New("Downloaded source ID does not match the selected Deezer track")
	}
	if t.ISRC != "" && !strings.EqualFold(tags["isrc"], t.ISRC) && !strings.EqualFold(tags["tsrc"], t.ISRC) {
		return errors.New("Downloaded ISRC differs from the selected recording")
	}
	if normalized(tags["title"]) != normalized(trackLabel(t)) || !artistMatches(t, tags["artist"]) || normalized(tags["album"]) != normalized(t.Album.Title) {
		return errors.New("Downloaded tags differ from the selected track; review is required")
	}
	tolerance := verifiedDurationTolerance(t.Duration, t.ISRC != "")
	if !validDuration(duration) || !validDuration(t.Duration) {
		return errors.New("Downloaded recording has an invalid duration")
	}
	var mismatch *audioDurationMismatch
	if math.Abs(duration-t.Duration) > tolerance {
		mismatch = &audioDurationMismatch{Expected: t.Duration, Measured: duration, Tolerance: tolerance}
		if a.DB != nil {
			checksum, err := hashFile(path)
			if err != nil {
				return err
			}
			var accepted float64
			if a.DB.QueryRow("SELECT duration FROM audio_duration_approvals WHERE checksum=? AND recording=? AND quality=?", checksum, audioApprovalKey(t), quality).Scan(&accepted) == nil && math.Abs(accepted-duration) < 0.01 {
				mismatch = nil
			}
		}
	}
	decoder := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-xerror", "-i", path, "-map", "0:a:0", "-f", "null", "-")
	protectChild(decoder)
	decoder.Stdout = io.Discard
	decoder.Stderr = io.Discard
	if e = decoder.Run(); e != nil {
		return errors.New("Full audio decoding failed; the file was not published")
	}
	if mismatch != nil {
		return mismatch
	}
	return nil
}

func validDuration(seconds float64) bool {
	return seconds > 0 && !math.IsNaN(seconds) && !math.IsInf(seconds, 0)
}

func verifiedDurationTolerance(seconds float64, isrcVerified bool) float64 {
	limit := 3.0
	if isrcVerified {
		limit = 5
	}
	return math.Min(limit, math.Max(1, seconds*0.05))
}

func sameApprovedRecording(approved, current Track) bool {
	if approved.ID != current.ID || (approved.ISRC != "" && !strings.EqualFold(approved.ISRC, current.ISRC)) {
		return false
	}
	tolerance := verifiedDurationTolerance(approved.Duration, approved.ISRC != "")
	if !validDuration(approved.Duration) || !validDuration(current.Duration) || math.Abs(approved.Duration-current.Duration) > tolerance {
		return false
	}
	// Compare identity/versions with the same normalization used for review;
	// duration was checked explicitly above against the approved snapshot.
	source := apple.Song{Title: trackLabel(approved), Artist: approved.Artist.Name, Album: approved.Album.Title, Duration: current.Duration}
	return assessRecording(source, current).Confident
}
func (a *App) acquire(ctx context.Context, jid, target string, t Track, profile, arl string) (string, string, error) {
	root, e := os.Lstat(a.musicDir)
	if e != nil || !root.IsDir() {
		return "", "", errors.New("Music mount is missing; mount the directory Navidrome scans")
	}
	if _, e = os.Stat(a.dfiPath); e != nil {
		return "", "", errors.New("Pinned d-fi executable is missing")
	}
	if _, e = exec.LookPath(a.ffprobePath); e != nil {
		return "", "", errors.New("ffprobe is missing")
	}
	// A deterministic managed directory permits crash reconciliation without overwriting library files.
	var qualities []string
	if json.Unmarshal([]byte(profile), &qualities) != nil || len(qualities) == 0 {
		return "", "", errors.New("Invalid quality snapshot")
	}
	defer a.clearTransfer(jid)
	failures := []string{}
	// An explicit file acceptance wins over quality fallback retries. Otherwise
	// accepting the retained 320 file could start a fresh lossless download first.
	if a.DB != nil {
		for _, quality := range qualities {
			if quality != "lossless" && quality != "320" && quality != "128" {
				continue
			}
			paths, err := stagedAudio(filepath.Join(a.dataDir, "staging", jid, quality))
			if err != nil || len(paths) != 1 {
				continue
			}
			checksum, err := hashFile(paths[0])
			if err != nil {
				continue
			}
			var accepted int
			if a.DB.QueryRow("SELECT count(*) FROM audio_duration_approvals WHERE checksum=? AND recording=? AND quality=?", checksum, audioApprovalKey(t), quality).Scan(&accepted) != nil || accepted == 0 {
				continue
			}
			if err := a.validateAudio(paths[0], t, quality); err != nil {
				return "", "", err
			}
			rel, err := a.placeAudio(jid, target, paths[0], t, quality)
			return rel, quality, err
		}
	}
	for _, quality := range qualities {
		if quality != "lossless" && quality != "320" && quality != "128" {
			return "", "", errors.New("Unsupported quality in request")
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		default:
		}
		stage := filepath.Join(a.dataDir, "staging", jid, quality)
		if e = os.MkdirAll(stage, 0700); e != nil {
			return "", "", errors.New("Unable to create staging directory")
		}
		if e = diskHeadroom(stage, 64<<20); e != nil {
			return "", "", e
		}
		if e = diskHeadroom(a.musicDir, 64<<20); e != nil {
			return "", "", e
		}
		existing, err := stagedAudio(stage)
		if err != nil {
			return "", "", err
		}
		for _, path := range existing {
			if a.validateAudio(path, t, quality) == nil {
				rel, err := a.placeAudio(jid, target, path, t, quality)
				return rel, quality, err
			}
			if e = os.Remove(path); e != nil {
				return "", "", errors.New("Cannot remove invalid staged output before retry")
			}
		}
		a.clearTransfer(jid)
		a.jobState(jid, "downloading", "Acquiring "+quality+" audio with d-fi 2.3.5")
		config := map[string]any{"concurrency": 1, "trackNumber": false, "fallbackTrack": false, "fallbackQuality": false, "cookies": map[string]string{"arl": arl}, "cover": map[string]string{"mode": "embed"}}
		cfg := filepath.Join(stage, "d-fi.config.json")
		if e = os.WriteFile(cfg, []byte(mustJSON(config)), 0600); e != nil {
			return "", "", errors.New("Unable to create downloader configuration")
		}
		dfiQuality := quality
		if quality == "lossless" {
			dfiQuality = "flac"
		}
		runCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		cmd := exec.CommandContext(runCtx, a.dfiPath, "--headless", "--quality", dfiQuality, "--url", fmt.Sprintf("https://www.deezer.com/track/%d", t.ID), "--output", "audio", "--config-file", cfg)
		protectChild(cmd)
		cmd.Dir = stage
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + stage, "TMPDIR=" + stage, "DFI_NO_PAUSE=1", "ZENSYNC_PROGRESS=1"}
		output := &transferOutput{app: a, job: jid}
		cmd.Stdout = output
		cmd.Stderr = io.Discard
		cmd.WaitDelay = 5 * time.Second
		started := time.Now()
		monitorFailure := ""
		e = cmd.Start()
		if e == nil {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			ticker := time.NewTicker(2 * time.Second)
		monitor:
			for {
				select {
				case e = <-done:
					break monitor
				case <-ticker.C:
					_, limitErr := stagedAudio(stage)
					if limitErr == nil {
						limitErr = diskHeadroom(stage, 32<<20)
					}
					if limitErr != nil {
						monitorFailure = "Downloader stopped because staging safety or free-storage limits were exceeded"
						cancel()
						<-done
						e = limitErr
						break monitor
					}
				}
			}
			ticker.Stop()
		}

		slog.Info("d-fi attempt finished", "job_id", jid, "quality", quality, "success", e == nil, "duration_ms", time.Since(started).Milliseconds())
		cancel()
		_ = os.Remove(cfg)
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		if e != nil {
			reason := output.failure
			if reason == "" {
				var exit *exec.ExitError
				if errors.As(e, &exit) {
					reason = fmt.Sprintf("d-fi exited with code %d; check the configured ARL/account and connectivity", exit.ExitCode())
				} else {
					reason = "d-fi could not run; check its executable and configuration"
				}
			}
			if monitorFailure != "" {
				reason = monitorFailure
			}
			if runCtx.Err() == context.DeadlineExceeded {
				reason = "Downloader timed out"
			}
			failures = append(failures, quality+": "+reason)
			continue
		}
		paths, scanErr := stagedAudio(stage)
		e = scanErr
		if e != nil || len(paths) != 1 {
			reason := output.failure
			if reason == "" {
				if scanErr != nil {
					reason = "Downloader output failed staging safety checks"
				} else if len(paths) > 1 {
					reason = "d-fi produced multiple audio files; none was accepted"
				} else {
					reason = "d-fi produced no audio file; check account access to this exact recording"
				}
			}
			failures = append(failures, quality+": "+reason)
			continue
		}
		a.jobState(jid, "validating", "Verifying audio, tags, duration and quality")
		if e = a.validateAudio(paths[0], t, quality); e != nil {
			failures = append(failures, quality+": "+e.Error())
			continue
		}
		rel, e := a.placeAudio(jid, target, paths[0], t, quality)
		if e != nil {
			return "", "", e
		}
		return rel, quality, nil
	}
	if len(failures) > 0 {
		return "", "", errors.New(strings.Join(failures, "; "))
	}
	return "", "", errors.New("d-fi could not acquire a valid recording in the allowed qualities. Check Deezer ARL/account access and retry; no alternate recording was accepted")
}
func hashFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (a *App) placeAudio(jid, target, source string, t Track, quality string) (string, error) {
	a.placementMu.Lock()
	defer a.placementMu.Unlock()
	checksum, e := hashFile(source)
	if e != nil {
		return "", e
	}
	rel, e := a.libraryPath(source, t, checksum)
	if e != nil {
		return "", e
	}
	destination := filepath.Join(a.musicDir, rel)
	dir := filepath.Dir(destination)
	if e = a.ensureMusicDirs(filepath.Dir(rel)); e != nil {
		return "", e
	}
	a.jobState(jid, "importing", "Copying validated audio into the shared music library")
	if info, statErr := os.Lstat(destination); statErr == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("Destination is not a regular file")
		}
		e = nil
		existing, err := hashFile(destination)
		if err != nil || existing != checksum {
			return "", errors.New("Destination collision; existing audio was preserved")
		}
	} else if os.IsNotExist(statErr) {
		// Copy within the destination filesystem, fsync, then hard-link without overwrite.
		tmp, e := os.CreateTemp(dir, ".zensync-*.partial")
		if e != nil {
			return "", e
		}
		tempName := tmp.Name()
		defer os.Remove(tempName)
		input, e := os.Open(source)
		if e != nil {
			tmp.Close()
			return "", e
		}
		_, e = io.Copy(tmp, input)
		input.Close()
		if e == nil {
			e = tmp.Chmod(0644)
		}
		if e == nil {
			e = tmp.Sync()
		}
		tmp.Close()
		if e != nil {
			return "", e
		}
		if e = a.validateAudio(tempName, t, quality); e != nil {
			return "", e
		}
		if e = os.Link(tempName, destination); e != nil {
			return "", errors.New("Atomic placement failed; existing files were not overwritten")
		}
		df, e := os.Open(dir)
		if e == nil {
			e = df.Sync()
			df.Close()
		}
		if e != nil {
			return "", e
		}
	} else {
		return "", statErr
	}
	if _, e = a.DB.Exec("INSERT INTO assets(target,file_path,quality,metadata) VALUES(?,?,?,?) ON CONFLICT(target) DO NOTHING", target, filepath.ToSlash(rel), quality, mustJSON(t)); e != nil {
		return "", e
	}
	return filepath.ToSlash(rel), nil
}
func (a *App) verifyPlaced(rel, metadata, quality string) error {
	if !safeRelativePath(rel) {
		return errors.New("Invalid placed asset path")
	}
	var t Track
	if json.Unmarshal([]byte(metadata), &t) != nil {
		return errors.New("Invalid asset metadata")
	}
	return a.validateAudio(filepath.Join(a.musicDir, filepath.FromSlash(rel)), t, quality)
}
func (a *App) findPlaced(ctx context.Context, c Credentials, rel, metadata string, requireUnique ...bool) (string, error) {
	unique := len(requireUnique) > 0 && requireUnique[0]
	id, _, err := a.findPlacedWithReport(ctx, c, rel, metadata, unique)
	return id, err
}

func stagedAudio(stage string) ([]string, error) {
	paths := []string{}
	var total int64
	count := 0
	e := filepath.WalkDir(stage, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		if count > 1000 {
			return errors.New("Downloader output file limit exceeded")
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("Symlink output rejected")
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
			if total > 1<<30 {
				return errors.New("Staging limit of 1 GiB exceeded")
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".flac" || ext == ".mp3" {
				paths = append(paths, path)
			}
		}
		return nil
	})
	return paths, e
}
func diskHeadroom(path string, minimum uint64) error {
	var stat syscall.Statfs_t
	if e := syscall.Statfs(path, &stat); e != nil {
		return errors.New("Cannot check available storage")
	}
	if uint64(stat.Bavail)*uint64(stat.Bsize) < minimum {
		return errors.New("Insufficient free disk space; free storage before retrying")
	}
	return nil
}

func (a *App) sameRemoteAudio(ctx context.Context, c Credentials, s Song, rel string) (bool, error) {
	local := filepath.Join(a.musicDir, filepath.FromSlash(rel))
	info, e := os.Stat(local)
	if e != nil {
		return false, errors.New("Validated local file is missing")
	}
	if s.Size > 0 && s.Size != info.Size() {
		slog.Warn("Navidrome indexed size differs; checking original bytes", "song_id", s.ID, "indexed_size", s.Size, "local_size", info.Size())
	}
	checksum, e := hashFile(local)
	if e != nil {
		return false, e
	}
	return a.sameRemoteAudioHash(ctx, c, s.ID, info.Size(), checksum)
}

func (a *App) sameRemoteAudioHash(ctx context.Context, c Credentials, sid string, size int64, checksum string) (bool, error) {
	resp, e := a.navRaw(ctx, c, "download", mapValues("id", sid))
	if e != nil {
		return false, e
	}
	defer resp.Body.Close()
	// An index entry can outlive its file during a scan. It is a failed
	// candidate, not evidence that the account cannot download another entry.
	if resp.StatusCode == 404 || resp.StatusCode == 410 {
		return false, nil
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") {
		if strings.Contains(contentType, "json") {
			var envelope struct {
				Response NavResponse `json:"subsonic-response"`
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope) == nil && envelope.Response.Error != nil && envelope.Response.Error.Code == 70 {
				return false, nil
			}
		}
		return false, errors.New("Navidrome returned an API error instead of original audio; check the service account's download permission")
	}
	if resp.StatusCode != 200 {
		return false, errors.New("Navidrome original-file verification failed; allow downloads for the service account")
	}
	if resp.ContentLength >= 0 && resp.ContentLength != size {
		return false, nil
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(resp.Body, size+1))
	if e != nil {
		return false, errors.New("Could not verify original audio from Navidrome; check download permission and connectivity")
	}
	return n == size && hex.EncodeToString(h.Sum(nil)) == checksum, nil
}
