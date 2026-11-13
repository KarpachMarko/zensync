package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const previewAudioLimit = 32 << 20

type deezerPreview struct {
	ready   chan struct{}
	body    []byte
	err     error
	created time.Time
}

// Each playback session shares a small, short-lived 128 kbps buffer so byte
// range requests never start acquisition jobs or repeatedly fetch the recording.
func (a *App) deezerPreviewAudio(ctx context.Context, key, target, arl string) ([]byte, error) {
	a.previewMu.Lock()
	if a.deezerPreviews == nil {
		a.deezerPreviews = map[string]*deezerPreview{}
	}
	for k, p := range a.deezerPreviews {
		select {
		case <-p.ready:
			if time.Since(p.created) > 5*time.Minute {
				delete(a.deezerPreviews, k)
			}
		default:
		}
	}
	if p, ok := a.deezerPreviews[key]; ok {
		a.previewMu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.ready:
			return p.body, p.err
		}
	}
	if len(a.deezerPreviews) >= 4 {
		var oldest string
		for k, p := range a.deezerPreviews {
			select {
			case <-p.ready:
				if oldest == "" || p.created.Before(a.deezerPreviews[oldest].created) {
					oldest = k
				}
			default:
			}
		}
		if oldest == "" {
			a.previewMu.Unlock()
			return nil, errors.New("Preview buffers are busy; try again shortly")
		}
		delete(a.deezerPreviews, oldest)
	}
	p := &deezerPreview{ready: make(chan struct{}), created: time.Now()}
	a.deezerPreviews[key] = p
	a.previewMu.Unlock()
	body, err := a.bufferDeezerPreview(ctx, target, arl)
	a.previewMu.Lock()
	p.body, p.err = body, err
	if err != nil {
		delete(a.deezerPreviews, key)
	}
	close(p.ready)
	a.previewMu.Unlock()
	return body, err
}
func (a *App) bufferDeezerPreview(ctx context.Context, target, arl string) ([]byte, error) {
	track, err := a.reviewTrack(ctx, target)
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(a.dfiPath); err != nil {
		return nil, errors.New("Pinned d-fi executable is missing; full Deezer listening requires it")
	}
	stage, err := os.MkdirTemp(a.dataDir, "preview-")
	if err != nil {
		return nil, errors.New("Cannot prepare preview buffer")
	}
	defer os.RemoveAll(stage)
	if err = diskHeadroom(stage, 64<<20); err != nil {
		return nil, err
	}
	config := map[string]any{"concurrency": 1, "trackNumber": false, "fallbackTrack": false, "fallbackQuality": false, "cookies": map[string]string{"arl": arl}, "cover": map[string]string{"mode": "none"}}
	cfg := filepath.Join(stage, "d-fi.config.json")
	if err = os.WriteFile(cfg, []byte(mustJSON(config)), 0600); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, a.dfiPath, "--headless", "--quality", "128", "--url", fmt.Sprintf("https://www.deezer.com/track/%d", track.ID), "--output", "audio", "--config-file", cfg)
	protectChild(cmd)
	cmd.Dir = stage
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + stage, "TMPDIR=" + stage, "DFI_NO_PAUSE=1"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 5 * time.Second
	if err = cmd.Start(); err != nil {
		return nil, errors.New("Cannot start Deezer preview buffer")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err = <-done:
			if err != nil {
				return nil, errors.New("Deezer full-song playback failed; check the saved connection and account access")
			}
			paths, e := stagedAudio(stage)
			if e != nil || len(paths) != 1 {
				return nil, errors.New("Deezer did not return one playable song")
			}
			info, e := os.Stat(paths[0])
			if e != nil || info.Size() > previewAudioLimit {
				return nil, errors.New("Deezer preview exceeds the audio size limit")
			}
			if e = a.validateAudio(paths[0], track, "128"); e != nil {
				return nil, e
			}
			return os.ReadFile(paths[0])
		case <-ticker.C:
			var total int64
			e := filepath.WalkDir(stage, func(path string, d os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.Type()&os.ModeSymlink != 0 {
					return errors.New("Unsafe preview output")
				}
				if !d.IsDir() {
					info, e := d.Info()
					if e != nil {
						return e
					}
					total += info.Size()
				}
				return nil
			})
			if e != nil || total > previewAudioLimit {
				cancel()
				<-done
				return nil, errors.New("Deezer preview exceeds the audio size limit")
			}
		}
	}
}
func (a *App) streamDeezer(w http.ResponseWriter, r *http.Request, u User) {
	if r.Method != "GET" {
		fail(w, 405, "Method not allowed")
		return
	}
	id, err := deezerID(r.URL.Query().Get("target"))
	if err != nil {
		fail(w, 400, "Choose a valid Deezer recording")
		return
	}
	sid := r.URL.Query().Get("sid")
	if !streamSessionID.MatchString(sid) {
		fail(w, 400, "Invalid playback session")
		return
	}
	byteRange := r.Header.Get("Range")
	if len(byteRange) > 100 || (byteRange != "" && !streamRange.MatchString(byteRange)) {
		fail(w, 416, "Use a single audio byte range")
		return
	}
	var arl string
	if a.unseal("deezer", &arl) != nil || arl == "" {
		fail(w, 409, "Connect Deezer in Settings before full-song listening")
		return
	}
	target := "deezer:track:" + id
	key := digest(strings.Join([]string{u.ID, target, sid, digest(arl)}, ":"))
	body, err := a.deezerPreviewAudio(r.Context(), key, target, arl)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "preview.mp3", time.Time{}, bytes.NewReader(body))
}
