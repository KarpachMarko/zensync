package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The captured client establishes search and exact track lookup endpoints.
type losslessProvider struct{ app *App }

func (p losslessProvider) Ready() error { return nil }

var losslessSource = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,40}$`)

func (p losslessProvider) Search(ctx context.Context, q string) ([]Track, error) {
	result, err := p.searchCatalog(ctx, q)
	return result.Tracks, err
}
func (p losslessProvider) decodeTracks(resp *http.Response) ([]Track, error) {
	var result struct {
		Tracks []losslessTrackRecord `json:"tracks"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result) != nil {
		return nil, errors.New("Invalid Lossless.am track response")
	}
	tracks := []Track{}
	for _, raw := range result.Tracks {
		track, err := normalizeLosslessTrack(raw)
		if err != nil {
			continue
		}
		tracks = append(tracks, track)
	}
	if err := p.rememberTracks(tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}
func losslessTarget(input string) (string, error) {
	if strings.HasPrefix(input, "lossless:track:") && targetPattern.MatchString(input) {
		return input, nil
	}
	u, err := url.Parse(strings.TrimSpace(input))
	if err == nil && u.Scheme == "https" && u.Host == "lossless.am" && u.User == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 4 && parts[0] == "app" && parts[1] == "track" && losslessSource.MatchString(parts[2]) && digits.MatchString(parts[3]) {
			return "lossless:track:" + parts[2] + ":" + parts[3], nil
		}
	}
	return "", errors.New("Use a direct lossless.am/app/track/source/id song URL")
}
func (p losslessProvider) Track(ctx context.Context, input string) (Track, error) {
	target, err := losslessTarget(input)
	if err != nil {
		return Track{}, err
	}
	parts := strings.Split(target, ":")
	resp, err := p.request(ctx, "GET", "/api/track/"+parts[3]+"?source="+url.QueryEscape(parts[2]), nil)
	if err != nil {
		return Track{}, err
	}
	defer resp.Body.Close()
	tracks, err := p.decodeTracks(resp)
	if err != nil {
		return Track{}, err
	}
	for _, t := range tracks {
		if trackTarget(t) == target {
			return t, nil
		}
	}
	return Track{}, errors.New("Lossless.am did not return the requested recording")
}
func (p losslessProvider) Acquire(ctx context.Context, job, target string, t Track, profile string) (string, string, error) {
	if !losslessSource.MatchString(t.Source) {
		return "", "", errors.New("Invalid provider recording identity")
	}
	return p.app.acquireNative(ctx, job, target, t, profile, "lossless", "Lossless.am", func() (*http.Response, error) {
		return p.request(ctx, "GET", "/api/download/track?"+url.Values{"id": {fmt.Sprint(t.ID)}, "source": {t.Source}}.Encode(), nil)
	})
}

// Native providers share staging, measured quality, validation and publication.
func (a *App) acquireNative(ctx context.Context, job, target string, t Track, profile, stageName, providerName string, download func() (*http.Response, error)) (string, string, error) {
	var qualities []string
	if json.Unmarshal([]byte(profile), &qualities) != nil {
		return "", "", errors.New("Invalid quality snapshot")
	}
	if len(qualities) == 0 {
		return "", "", errors.New("Invalid quality snapshot")
	}
	for _, q := range qualities {
		if q != "lossless" && q != "320" && q != "128" {
			return "", "", errors.New("Unsupported quality in request")
		}
	}
	if trackTarget(t) != target {
		return "", "", errors.New("Invalid provider recording identity")
	}
	root, err := os.Lstat(a.musicDir)
	if err != nil || !root.IsDir() {
		return "", "", errors.New("Music mount is missing")
	}
	stage := filepath.Join(a.dataDir, "staging", job, stageName)
	if err = os.MkdirAll(stage, 0700); err != nil {
		return "", "", err
	}
	if err = diskHeadroom(stage, 64<<20); err != nil {
		return "", "", err
	}
	if err = diskHeadroom(a.musicDir, 64<<20); err != nil {
		return "", "", err
	}
	defer a.clearTransfer(job)
	existing, err := stagedAudio(stage)
	if err != nil {
		return "", "", err
	}
	for _, path := range existing {
		actual, _, probeErr := a.nativeAudioQuality(path)
		if probeErr == nil && nativeQualityAllowed(qualities, actual) && a.validateAudio(path, t, actual) == nil {
			rel, e := a.placeAudio(job, target, path, t, actual)
			return rel, actual, e
		}
	}
	a.jobState(job, "downloading", "Acquiring best available audio from "+providerName)
	resp, err := download()
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	contentType := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(contentType, "audio/") && contentType != "application/octet-stream" && contentType != "video/mp4" {
		return "", "", errors.New(providerName + " did not return audio")
	}
	if resp.ContentLength > 1<<30 {
		return "", "", errors.New(providerName + " audio exceeds 1 GiB")
	}
	f, err := os.CreateTemp(stage, "download-*.partial")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(f.Name())
	writer := &losslessDownloadWriter{file: f, app: a, job: job, total: resp.ContentLength}
	n, err := io.Copy(writer, io.LimitReader(resp.Body, (1<<30)+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return "", "", errors.New(providerName + " audio transfer failed")
	}
	if n < 1024 || n > 1<<30 || (resp.ContentLength >= 0 && n != resp.ContentLength) {
		return "", "", errors.New(providerName + " audio is incomplete or exceeds 1 GiB")
	}
	actual, extension, err := a.nativeAudioQuality(f.Name())
	if err != nil {
		return "", "", err
	}
	if !nativeQualityAllowed(qualities, actual) {
		return "", "", fmt.Errorf(providerName+" returned %s, which is outside the allowed quality profile", actual)
	}
	path := filepath.Join(stage, "audio"+extension)
	for _, old := range existing {
		if old != path {
			if err = os.Remove(old); err != nil {
				return "", "", errors.New("Cannot replace invalid staged audio")
			}
		}
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return "", "", err
	}
	a.jobState(job, "validating", "Verifying audio, tags, duration and actual quality")
	if err = a.validateAudio(path, t, actual); err != nil {
		return "", "", err
	}
	rel, err := a.placeAudio(job, target, path, t, actual)
	return rel, actual, err
}

// Keep disk checks and progress bounded while an HTTP download is in flight.
type losslessDownloadWriter struct {
	file              *os.File
	app               *App
	job               string
	total, downloaded int64
	checked           int64
	updated           time.Time
}

func (w *losslessDownloadWriter) Write(data []byte) (int, error) {
	if w.downloaded-w.checked >= 8<<20 {
		if err := diskHeadroom(filepath.Dir(w.file.Name()), 32<<20); err != nil {
			return 0, err
		}
		w.checked = w.downloaded
	}
	n, err := w.file.Write(data)
	w.downloaded += int64(n)
	now := time.Now()
	if now.Sub(w.updated) >= 500*time.Millisecond {
		progress := TransferProgress{DownloadedBytes: w.downloaded, TotalBytes: max(0, w.total), UpdatedAt: now}
		if w.total > 0 {
			percent := min(100, float64(w.downloaded)*100/float64(w.total))
			progress.Percent = &percent
		}
		w.app.transferMu.Lock()
		if w.app.transfers == nil {
			w.app.transfers = map[string]TransferProgress{}
		}
		w.app.transfers[w.job] = progress
		w.app.transferMu.Unlock()
		w.updated = now
	}
	return n, err
}
