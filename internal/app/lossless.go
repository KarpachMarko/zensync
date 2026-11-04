package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Only endpoints observed in the supplied HAR are used. Search revalidates exact
// IDs because the capture does not establish a track-detail endpoint.
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
		if _, err = p.app.DB.Exec("INSERT INTO provider_tracks(target,metadata) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET metadata=excluded.metadata", trackTarget(t), mustJSON(t)); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}
func (p losslessProvider) Track(ctx context.Context, target string) (Track, error) {
	if !targetPattern.MatchString(target) || !strings.HasPrefix(target, "lossless:") {
		return Track{}, errors.New("Choose a Lossless.am song from Search")
	}
	var metadata string
	if p.app.DB.QueryRow("SELECT metadata FROM provider_tracks WHERE target=?", target).Scan(&metadata) != nil {
		return Track{}, errors.New("Search for this Lossless.am song again before requesting it")
	}
	var previous Track
	if json.Unmarshal([]byte(metadata), &previous) != nil {
		return Track{}, errors.New("Invalid saved provider metadata")
	}
	tracks, err := p.Search(ctx, previous.Artist.Name+" "+previous.Title)
	if err != nil {
		return Track{}, err
	}
	for _, t := range tracks {
		if trackTarget(t) == target {
			return t, nil
		}
	}
	return Track{}, errors.New("Lossless.am no longer returns the selected recording")
}
func (p losslessProvider) Acquire(ctx context.Context, job, target string, t Track, profile string) (string, string, error) {
	var qualities []string
	if json.Unmarshal([]byte(profile), &qualities) != nil {
		return "", "", errors.New("Invalid quality snapshot")
	}
	allowed := false
	for _, q := range qualities {
		if q == "lossless" {
			allowed = true
		}
	}
	if !allowed {
		return "", "", errors.New("Lossless.am only supports verified FLAC downloads; enable lossless quality and submit again")
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
	path := filepath.Join(stage, "audio.flac")
	// Retained validated files and explicit duration acceptances survive retries.
	if p.app.validateAudio(path, t, "lossless") == nil {
		rel, e := p.app.placeAudio(job, target, path, t, "lossless")
		return rel, "lossless", e
	}
	p.app.jobState(job, "downloading", "Acquiring FLAC audio from Lossless.am")
	resp, err := p.request(ctx, "GET", "/api/download/track?"+url.Values{"id": {fmt.Sprint(t.ID)}, "source": {t.Source}}.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	mime := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mime != "audio/flac" && mime != "audio/x-flac" {
		return "", "", errors.New("Lossless.am did not return FLAC audio")
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
	if err = os.Rename(f.Name(), path); err != nil {
		return "", "", err
	}
	p.app.jobState(job, "validating", "Verifying audio, tags, duration and quality")
	if err = p.app.validateAudio(path, t, "lossless"); err != nil {
		return "", "", err
	}
	rel, err := p.app.placeAudio(job, target, path, t, "lossless")
	return rel, "lossless", err
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
