package app

import (
	"bytes"
	"encoding/json"
	"time"
)

// TransferProgress describes only network transfer, never overall job completion.
// It is ephemeral: restarting/retrying cannot expose an old transfer's counters.
type TransferProgress struct {
	Percent         *float64  `json:"percent,omitempty"`
	DownloadedBytes int64     `json:"downloadedBytes"`
	TotalBytes      int64     `json:"totalBytes"`
	BytesPerSecond  *float64  `json:"bytesPerSecond,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (a *App) clearTransfer(jid string) {
	a.transferMu.Lock()
	defer a.transferMu.Unlock()
	delete(a.transfers, jid)
}
func (a *App) transfer(jid, state string) *TransferProgress {
	if state != "downloading" {
		return nil
	}
	a.transferMu.Lock()
	defer a.transferMu.Unlock()
	p, ok := a.transfers[jid]
	if !ok {
		return nil
	}
	if time.Since(p.UpdatedAt) > 4*time.Second {
		zero := float64(0)
		p.BytesPerSecond = &zero
	}
	return &p
}

// stdout is untrusted human-oriented output except for our explicit numeric
// protocol. Oversized/noisy lines are discarded without logging or retaining them.
type transferOutput struct {
	app       *App
	job       string
	line      []byte
	overflow  bool
	lastBytes int64
	lastAt    time.Time
	lastTotal int64
	failure   string
}

func (s *transferOutput) Write(p []byte) (int, error) {
	n := len(p)
	for _, c := range p {
		if c == '\n' || c == '\r' {
			if !s.overflow {
				s.event(s.line, time.Now())
			}
			s.line = s.line[:0]
			s.overflow = false
		} else if !s.overflow {
			if len(s.line) >= 1024 {
				s.line = s.line[:0]
				s.overflow = true
			} else {
				s.line = append(s.line, c)
			}
		}
	}
	return n, nil
}
func (s *transferOutput) event(line []byte, now time.Time) {
	if bytes.HasPrefix(line, []byte("ZENSYNC_RESULT ")) {
		var result struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(line[len("ZENSYNC_RESULT "):], &result) != nil {
			return
		}
		switch result.Code {
		case "geo_blocked":
			s.failure = "Deezer denied this recording for the configured account's country"
		case "license_required":
			s.failure = "The configured Deezer account cannot stream this quality"
		case "unavailable":
			s.failure = "Deezer returned no downloadable recording for the configured account"
		case "timeout":
			s.failure = "Downloader timed out"
		case "provider_error":
			s.failure = "Deezer/downloader failed before a valid recording was produced"
		}
		return
	}
	prefix := []byte("ZENSYNC_TRANSFER ")
	if !bytes.HasPrefix(line, prefix) {
		return
	}
	var e struct {
		Phase      string `json:"phase"`
		Downloaded int64  `json:"downloaded"`
		Total      int64  `json:"total"`
	}
	if json.Unmarshal(line[len(prefix):], &e) != nil {
		return
	}
	switch e.Phase {
	case "decrypting", "tagging", "saving":
		s.app.clearTransfer(s.job)
		s.lastAt = time.Time{}
		messages := map[string]string{"decrypting": "Decrypting downloaded audio", "tagging": "Adding audio tags and artwork", "saving": "Saving downloaded audio"}
		s.app.jobState(s.job, "downloading", messages[e.Phase])
		return
	case "download":
	default:
		return
	}
	if e.Downloaded < 0 || e.Total < 0 || e.Total > 1<<30 || e.Downloaded > 1<<30 || (e.Total > 0 && e.Downloaded > e.Total) {
		return
	}
	progress := TransferProgress{DownloadedBytes: e.Downloaded, TotalBytes: e.Total, UpdatedAt: now}
	if e.Total > 0 {
		p := 100 * float64(e.Downloaded) / float64(e.Total)
		progress.Percent = &p
	}
	if !s.lastAt.IsZero() && now.After(s.lastAt) && e.Downloaded >= s.lastBytes && e.Total == s.lastTotal {
		rate := float64(e.Downloaded-s.lastBytes) / now.Sub(s.lastAt).Seconds()
		progress.BytesPerSecond = &rate
	}
	s.lastAt, s.lastBytes, s.lastTotal = now, e.Downloaded, e.Total
	s.app.transferMu.Lock()
	if s.app.transfers == nil {
		s.app.transfers = map[string]TransferProgress{}
	}
	s.app.transfers[s.job] = progress
	s.app.transferMu.Unlock()
}
