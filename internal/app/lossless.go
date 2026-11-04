package app

import (
	"bytes"
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
	tracks := []Track{}
	if strings.TrimSpace(q) == "" {
		return tracks, nil
	}
	body, _ := json.Marshal(map[string]string{"query": q})
	resp, err := p.request(ctx, "POST", "/api/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return p.decodeTracks(resp)
}
func (p losslessProvider) decodeTracks(resp *http.Response) ([]Track, error) {
	tracks := []Track{}
	var result struct {
		Tracks []struct {
			ID        int64    `json:"id"`
			Title     string   `json:"title"`
			Duration  float64  `json:"duration"`
			Source    string   `json:"source"`
			ISRC      string   `json:"isrc"`
			Explicit  bool     `json:"explicit"`
			Available *bool    `json:"streaming_available"`
			Artists   []Artist `json:"artists"`
			Album     struct {
				Title   string `json:"title"`
				Picture struct {
					Medium string `json:"medium"`
				} `json:"picture"`
			} `json:"album"`
		} `json:"tracks"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result) != nil {
		return nil, errors.New("Invalid Lossless.am search response")
	}
	for _, r := range result.Tracks {
		if r.ID <= 0 || r.Title == "" || len(r.Artists) == 0 || !validDuration(r.Duration) || !losslessSource.MatchString(r.Source) {
			continue
		}
		t := Track{Provider: "lossless", Source: r.Source, ID: r.ID, Title: r.Title, Duration: r.Duration, ISRC: r.ISRC, Contributors: r.Artists, ExplicitLyrics: &r.Explicit, Readable: r.Available}
		t.Artist.Name = r.Artists[0].Name
		t.Album.Title = r.Album.Title
		t.Album.Cover = r.Album.Picture.Medium
		// Unverified third-party preview URLs from the service are intentionally omitted.
		if _, err := p.app.DB.Exec("INSERT INTO provider_tracks(target,metadata) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET metadata=excluded.metadata", trackTarget(t), mustJSON(t)); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
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
	if !losslessSource.MatchString(t.Source) || trackTarget(t) != target {
		return "", "", errors.New("Invalid provider recording identity")
	}
	root, err := os.Lstat(p.app.musicDir)
	if err != nil || !root.IsDir() {
		return "", "", errors.New("Music mount is missing")
	}
	stage := filepath.Join(p.app.dataDir, "staging", job, "lossless")
	if err = os.MkdirAll(stage, 0700); err != nil {
		return "", "", err
	}
	if err = diskHeadroom(stage, 64<<20); err != nil {
		return "", "", err
	}
	if err = diskHeadroom(p.app.musicDir, 64<<20); err != nil {
		return "", "", err
	}
	defer p.app.clearTransfer(job)
	existing, err := stagedAudio(stage)
	if err != nil {
		return "", "", err
	}
	for _, path := range existing {
		actual, _, probeErr := p.app.nativeAudioQuality(path)
		if probeErr == nil && nativeQualityAllowed(qualities, actual) && p.app.validateAudio(path, t, actual) == nil {
			rel, e := p.app.placeAudio(job, target, path, t, actual)
			return rel, actual, e
		}
	}
	p.app.jobState(job, "downloading", "Acquiring best available audio from Lossless.am")
	resp, err := p.request(ctx, "GET", "/api/download/track?"+url.Values{"id": {fmt.Sprint(t.ID)}, "source": {t.Source}}.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	contentType := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(contentType, "audio/") && contentType != "application/octet-stream" && contentType != "video/mp4" {
		return "", "", errors.New("Lossless.am did not return audio")
	}
	if resp.ContentLength > 1<<30 {
		return "", "", errors.New("Lossless.am audio exceeds 1 GiB")
	}
	f, err := os.CreateTemp(stage, "download-*.partial")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(f.Name())
	writer := &losslessDownloadWriter{file: f, app: p.app, job: job, total: resp.ContentLength}
	n, err := io.Copy(writer, io.LimitReader(resp.Body, (1<<30)+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return "", "", errors.New("Lossless.am audio transfer failed")
	}
	if n < 1024 || n > 1<<30 || (resp.ContentLength >= 0 && n != resp.ContentLength) {
		return "", "", errors.New("Lossless.am audio is incomplete or exceeds 1 GiB")
	}
	actual, extension, err := p.app.nativeAudioQuality(f.Name())
	if err != nil {
		return "", "", err
	}
	if !nativeQualityAllowed(qualities, actual) {
		return "", "", fmt.Errorf("Lossless.am returned %s, which is outside the allowed quality profile", actual)
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
	p.app.jobState(job, "validating", "Verifying audio, tags, duration and actual quality")
	if err = p.app.validateAudio(path, t, actual); err != nil {
		return "", "", err
	}
	rel, err := p.app.placeAudio(job, target, path, t, actual)
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
