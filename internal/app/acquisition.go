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
		Codec   string `json:"codec_name"`
		Type    string `json:"codec_type"`
		BitRate string `json:"bit_rate"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func (a *App) validateAudio(path string, t Track, quality string) error {
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
	if t.Duration <= 0 || math.Abs(duration-t.Duration) > 3 {
		return errors.New("Downloaded duration differs from the selected recording")
	}
	valid := false
	for _, s := range p.Streams {
		if s.Type != "audio" {
			continue
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
	if t.ISRC != "" && tags["isrc"] != t.ISRC && tags["tsrc"] != t.ISRC {
		return errors.New("Downloaded ISRC differs from the selected recording")
	}
	if normalized(tags["title"]) != normalized(trackLabel(t)) || !artistMatches(t, tags["artist"]) || normalized(tags["album"]) != normalized(t.Album.Title) {
		return errors.New("Downloaded tags differ from the selected track; review is required")
	}
	decoder := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-xerror", "-i", path, "-map", "0:a:0", "-f", "null", "-")
	protectChild(decoder)
	decoder.Stdout = io.Discard
	decoder.Stderr = io.Discard
	if e = decoder.Run(); e != nil {
		return errors.New("Full audio decoding failed; the file was not published")
	}
	return nil
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
	failures := []string{}
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
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + stage, "TMPDIR=" + stage, "DFI_NO_PAUSE=1"}
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		cmd.WaitDelay = 5 * time.Second
		started := time.Now()
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
			continue
		}
		paths, scanErr := stagedAudio(stage)
		e = scanErr
		if e != nil || len(paths) != 1 {
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
	var t Track
	if json.Unmarshal([]byte(metadata), &t) != nil {
		return "", errors.New("Invalid asset metadata")
	}
	songs := []Song{}
	for offset := 0; offset < 10000; offset += 100 {
		page, err := a.navSearch(ctx, c, t.Title, offset)
		if err != nil {
			return "", err
		}
		songs = append(songs, page...)
		if len(page) < 100 {
			break
		}
	}
	unique := len(requireUnique) > 0 && requireUnique[0]
	matchedIDs := []string{}
	slog.Info("Navidrome song lookup", "expected_path", rel, "candidate_count", len(songs))
	for _, s := range songs {
		path := strings.ReplaceAll(s.Path, "\\", "/")
		pathMatches := path == rel || strings.HasSuffix(path, "/"+rel)
		if pathMatches && !trackMatches(t, s) {
			slog.Warn("Navidrome file metadata differs from approved recording", "song_id", s.ID, "file_path", path)
		}
		// Navidrome normally reports virtual paths. Verify original bytes rather
		// than trusting a virtual filename or metadata-only coincidence.
		matched, err := a.sameRemoteAudio(ctx, c, s, rel)
		if err != nil {
			return "", err
		}
		if matched {
			if !trackMatches(t, s) {
				slog.Warn("Navidrome metadata differs but original audio bytes match", "song_id", s.ID)
			}
			slog.Info("Navidrome asset verified", "song_id", s.ID, "file_path", rel, "reported_path", path)
			if !unique {
				return s.ID, nil
			}
			matchedIDs = append(matchedIDs, s.ID)
			continue
		}
		slog.Warn("Navidrome candidate has different file content", "song_id", s.ID, "reported_path", path)

	}
	if len(matchedIDs) == 1 {
		return matchedIDs[0], nil
	}
	if len(matchedIDs) > 1 {
		return "", errors.New("Multiple Navidrome songs contain the same audio; relocation cannot safely identify the original file")
	}
	return "", nil
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
		return false, nil
	}
	checksum, e := hashFile(local)
	if e != nil {
		return false, e
	}
	resp, e := a.navRaw(ctx, c, "download", mapValues("id", s.ID))
	if e != nil {
		return false, e
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") {
		return false, errors.New("Navidrome returned an API error instead of original audio; check the service account's download permission")
	}
	if resp.StatusCode != 200 {
		return false, errors.New("Navidrome original-file verification failed; allow downloads for the service account")
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(resp.Body, info.Size()+1))
	if e != nil {
		return false, errors.New("Could not verify original audio from Navidrome; check download permission and connectivity")
	}
	return n == info.Size() && hex.EncodeToString(h.Sum(nil)) == checksum, nil
}
