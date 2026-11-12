package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxMetadataCover = 20 << 20

func validateMetadataCover(data []byte) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 50000000 {
		return errors.New("Choose a JPEG or PNG under 50 megapixels")
	}
	if len(data) > maxMetadataCover {
		return errors.New("Cover exceeds 20 MiB")
	}
	return nil
}
func coverUploadRelative(actor, song, token string) (string, error) {
	if len(token) != 64 || strings.Trim(token, "0123456789abcdef") != "" {
		return "", errors.New("Invalid uploaded cover")
	}
	return filepath.Join(digest(actor+"|"+song), token), nil
}
func (a *App) readCoverUpload(relative string) ([]byte, error) {
	if !safeRelativePath(relative) {
		return nil, errors.New("Invalid uploaded cover")
	}
	root := filepath.Join(a.dataDir, "metadata-cover-uploads")
	path := filepath.Join(root, relative)
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Uploaded cover is unavailable; upload it again")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxMetadataCover {
		return nil, errors.New("Uploaded cover is unavailable; upload it again")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, validateMetadataCover(data)
}
func (a *App) metadataCoverUpload(w http.ResponseWriter, r *http.Request, u User) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v2/metadata/songs/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] != "cover-upload" {
		fail(w, 400, "Choose a song")
		return
	}
	sid := parts[0]
	if len(parts) == 3 && r.Method == "GET" {
		relative, err := coverUploadRelative(u.ID, sid, parts[2])
		if err != nil {
			fail(w, 404, "Uploaded cover not found")
			return
		}
		data, err := a.readCoverUpload(relative)
		if err != nil {
			fail(w, 404, "Uploaded cover not found")
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(data))
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
		return
	}
	if len(parts) != 2 || r.Method != "POST" {
		fail(w, 405, "Method not allowed")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	remote, err := a.nav(r.Context(), c, "getSong", mapValues("id", sid))
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if remote.Song.ID != sid {
		fail(w, 404, "Song no longer exists")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxMetadataCover+1))
	if err != nil || len(data) > maxMetadataCover {
		fail(w, 413, "Choose a cover under 20 MiB")
		return
	}
	if err := validateMetadataCover(data); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := a.cleanupCoverUploads(); err != nil {
		slog.Warn("Temporary cover cleanup deferred", "error", err)
	}
	token := id()
	relative, _ := coverUploadRelative(u.ID, sid, token)
	path := filepath.Join(a.dataDir, "metadata-cover-uploads", relative)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		a.internal(w, err)
		return
	}
	if err := diskHeadroom(filepath.Dir(path), uint64(len(data))+(1<<20)); err != nil {
		fail(w, 507, "Not enough space for the uploaded cover")
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		a.internal(w, err)
		return
	}
	send(w, 201, map[string]string{"id": token})
}

func publicArtworkIP(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range []string{"100.64.0.0/10", "198.18.0.0/15", "192.0.0.0/24", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(block).Contains(address) {
			return false
		}
	}
	return true
}
func validateArtworkURL(ctx context.Context, u *url.URL) error {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || (u.Port() != "" && u.Port() != "443") {
		return errors.New("Use a public HTTPS image URL")
	}
	// Existing provider hosts remain compatible with mocked provider transports.
	// Real requests are also checked and pinned in DialContext below.
	if metadataCoverHost(u) {
		return nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil || len(ips) == 0 {
		return errors.New("Image host could not be resolved")
	}
	for _, ip := range ips {
		if !publicArtworkIP(ip) {
			return errors.New("Use an image URL on a public host")
		}
	}
	return nil
}
func (a *App) artworkClient() *http.Client {
	client := *a.HTTP
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		copy := transport.Clone()
		copy.Proxy = nil
		copy.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !publicArtworkIP(ip) {
					return nil, errors.New("Image host is not public")
				}
			}
			var last error
			for _, ip := range ips {
				conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			if last == nil {
				last = errors.New("Image host has no public address")
			}
			return nil, last
		}
		client.Transport = copy
	}
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("Too many image redirects")
		}
		return validateArtworkURL(r.Context(), r.URL)
	}
	return &client
}

// Abandoned picker uploads expire; files needed by queued/running/failed jobs
// remain available for retries. Completed jobs already embed the image.
func (a *App) cleanupCoverUploads() error {
	rows, err := a.DB.Query("SELECT changes FROM metadata_jobs WHERE state IN ('queued','running','failed')")
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var changes []metadataChange
		if err := json.Unmarshal([]byte(raw), &changes); err != nil {
			rows.Close()
			return err
		}
		for _, change := range changes {
			if change.UploadedCover != "" {
				keep[change.UploadedCover] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	root := filepath.Join(a.dataDir, "metadata-cover-uploads")
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Invalid upload storage directory")
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 2 || len(parts[0]) != 64 || len(parts[1]) != 64 || strings.Trim(parts[0]+parts[1], "0123456789abcdef") != "" || keep[relative] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if time.Since(info.ModTime()) > 24*time.Hour {
			return os.Remove(path)
		}
		return nil
	})
}
