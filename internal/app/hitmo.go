package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const hitmoOrigin = "https://eu.hitmoz.com"

var hitmoAudioCDN = regexp.MustCompile(`^(cdn[0-9]{1,3}\.deliciousoranges\.com|s[0-9]{1,3}\.deliciouspeaches\.com)$`)

type hitmoProvider struct{ app *App }
type hitmoRecording struct {
	track    Track
	download string
	stream   string
}

func (p hitmoProvider) Ready() error { return nil }

func hitmoTarget(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "hitmo:track:") && targetPattern.MatchString(input) {
		return input, nil
	}
	u, err := url.Parse(input)
	if err == nil && u.Scheme == "https" && u.Host == "eu.hitmoz.com" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		id := strings.TrimPrefix(u.Path, "/song/")
		if digits.MatchString(id) {
			return "hitmo:track:" + id, nil
		}
	}
	return "", errors.New("Use a direct https://eu.hitmoz.com/song/id song URL")
}

// Only the observed HTTPS origin and full music paths may be fetched. Never
// follow a page-supplied external link or forward another provider's cookies.
func hitmoDownloadURL(raw string) (string, error) {
	return hitmoAudioURL(raw, false)
}

func hitmoAudioURL(raw string, streaming bool) (string, error) {
	base, _ := url.Parse(hitmoOrigin)
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("Invalid Hitmo audio URL")
	}
	u = base.ResolveReference(u)
	if u.Scheme != "https" || u.Host != base.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !(strings.HasPrefix(u.Path, "/get/music/") || (streaming && strings.HasPrefix(u.Path, "/get/cuts/"))) || !strings.HasSuffix(u.Path, ".mp3") || strings.Contains(u.Path, "..") {
		return "", errors.New("Untrusted Hitmo audio URL")
	}
	return u.String(), nil
}

func (p hitmoProvider) request(ctx context.Context, path string) (*http.Response, error) {
	return p.fetch(ctx, path, "", false)
}

func (p hitmoProvider) fetch(ctx context.Context, path, byteRange string, streaming bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", hitmoOrigin+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	client := *p.app.HTTP
	client.Jar = nil
	client.Timeout = 2 * time.Minute
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		// Full MP3 links redirect once to Hitmo's observed delivery CDN. Keep the
		// selected file path intact and reject all other destinations and chains.
		u := next.URL
		if (strings.HasPrefix(path, "/get/music/") || strings.HasPrefix(path, "/get/cuts/")) && len(via) == 1 && u.Scheme == "https" && u.Port() == "" && u.User == nil && hitmoAudioCDN.MatchString(u.Host) && (u.Path == "/s1"+via[0].URL.Path || u.Path == via[0].URL.Path) && u.RawQuery == "" && u.Fragment == "" {
			next.Header.Del("Cookie")
			next.Header.Del("Authorization")
			return nil
		}
		return errors.New("Untrusted Hitmo redirect")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Hitmo request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK && !(streaming && (resp.StatusCode == http.StatusPartialContent || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable)) {
		resp.Body.Close()
		return nil, fmt.Errorf("Hitmo returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func hitmoAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}
func hitmoClass(n *html.Node, class string) bool {
	for _, value := range strings.Fields(hitmoAttr(n, "class")) {
		if value == class {
			return true
		}
	}
	return false
}
func hitmoText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(hitmoText(c))
	}
	return b.String()
}
func parseHitmo(body io.Reader) ([]hitmoRecording, error) {
	data, err := io.ReadAll(io.LimitReader(body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, errors.New("Hitmo page is unreadable or too large")
	}
	root, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, errors.New("Invalid Hitmo page")
	}
	out := []hitmoRecording{}
	seen := map[int64]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if raw := hitmoAttr(n, "data-musmeta"); raw != "" {
			var meta struct{ ID, Title, Artist, Img, URL string }
			if json.Unmarshal([]byte(raw), &meta) == nil && strings.HasPrefix(meta.ID, "track-id-") {
				id, _ := strconv.ParseInt(strings.TrimPrefix(meta.ID, "track-id-"), 10, 64)
				var duration float64
				var download string
				var inspect func(*html.Node)
				inspect = func(child *html.Node) {
					if hitmoClass(child, "track__fulltime") {
						parts := strings.Split(strings.TrimSpace(hitmoText(child)), ":")
						if len(parts) == 2 {
							m, e1 := strconv.Atoi(parts[0])
							s, e2 := strconv.Atoi(parts[1])
							if e1 == nil && e2 == nil && m >= 0 && s >= 0 && s < 60 {
								duration = float64(m*60 + s)
							}
						}
					}
					if child.Data == "a" && hitmoClass(child, "track__download-btn") {
						download, _ = hitmoDownloadURL(hitmoAttr(child, "href"))
					}
					for c := child.FirstChild; c != nil; c = c.NextSibling {
						inspect(c)
					}
				}
				inspect(n)
				if id > 0 && !seen[id] && strings.TrimSpace(meta.Title) != "" && strings.TrimSpace(meta.Artist) != "" && validDuration(duration) && download != "" {
					t := Track{Provider: "hitmo", ID: id, Title: meta.Title, Duration: duration, Link: fmt.Sprintf("%s/song/%d", hitmoOrigin, id)}
					t.Artist.Name = meta.Artist
					t.Album.Cover = meta.Img
					stream, _ := hitmoAudioURL(meta.URL, true)
					if stream == "" {
						stream = download
					}
					out = append(out, hitmoRecording{track: t, download: download, stream: stream})
					seen[id] = true
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	// A challenge/login page is not an empty search result.
	if len(out) == 0 && !strings.Contains(string(data), "search-form") {
		return nil, errors.New("Hitmo returned an unexpected page")
	}
	return out, nil
}
func (p hitmoProvider) recordings(ctx context.Context, path string) ([]hitmoRecording, error) {
	resp, err := p.request(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	recordings, err := parseHitmo(resp.Body)
	if err != nil {
		return nil, err
	}
	for _, recording := range recordings {
		if _, err := p.app.DB.ExecContext(ctx, "INSERT INTO provider_tracks(target,metadata) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET metadata=excluded.metadata", trackTarget(recording.track), mustJSON(recording.track)); err != nil {
			return nil, err
		}
	}
	return recordings, nil
}
func (p hitmoProvider) Search(ctx context.Context, query string) ([]Track, error) {
	tracks := []Track{}
	query = strings.TrimSpace(query)
	if query == "" {
		return tracks, nil
	}
	if strings.HasPrefix(query, "https://") || strings.HasPrefix(query, "hitmo:") {
		t, err := p.Track(ctx, query)
		if err != nil {
			return nil, err
		}
		return []Track{t}, nil
	}
	recordings, err := p.recordings(ctx, "/search?q="+url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	for _, r := range recordings {
		tracks = append(tracks, r.track)
	}
	return tracks, nil
}
func (p hitmoProvider) recording(ctx context.Context, input string) (hitmoRecording, error) {
	target, err := hitmoTarget(input)
	if err != nil {
		return hitmoRecording{}, err
	}
	recordings, err := p.recordings(ctx, "/song/"+strings.TrimPrefix(target, "hitmo:track:"))
	if err != nil {
		return hitmoRecording{}, err
	}
	for _, r := range recordings {
		if trackTarget(r.track) == target {
			return r, nil
		}
	}
	return hitmoRecording{}, errors.New("Hitmo did not return the requested recording")
}
func (p hitmoProvider) Track(ctx context.Context, input string) (Track, error) {
	r, err := p.recording(ctx, input)
	return r.track, err
}
func (p hitmoProvider) Acquire(ctx context.Context, job, target string, t Track, profile string) (string, string, error) {
	// Resolve the current download URL from the exact song page, never from a client.
	r, err := p.recording(ctx, target)
	if err != nil {
		return "", "", err
	}
	if audioApprovalKey(t) != audioApprovalKey(r.track) {
		return "", "", errors.New("Hitmo recording changed since approval")
	}
	return p.app.acquireNative(ctx, job, target, t, profile, "hitmo", "Hitmo", func() (*http.Response, error) { return p.request(ctx, strings.TrimPrefix(r.download, hitmoOrigin)) })
}
