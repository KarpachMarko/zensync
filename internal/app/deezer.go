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
)

type Artist struct {
	Name string `json:"name"`
	Role string `json:"role"`
}
type Track struct {
	Contributors []Artist `json:"contributors"`
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	TitleVersion string   `json:"title_version"`
	Duration     float64  `json:"duration"`
	ISRC         string   `json:"isrc"`
	Link         string   `json:"link"`
	Readable     *bool    `json:"readable"`
	Artist       struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
		Cover string `json:"cover_medium"`
	} `json:"album"`
}

var digits = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

func deezerID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "deezer:track:") {
		input = strings.TrimPrefix(input, "deezer:track:")
	}
	if digits.MatchString(input) {
		return input, nil
	}
	u, e := url.Parse(input)
	if e != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || (u.Host != "www.deezer.com" && u.Host != "deezer.com") {
		return "", errors.New("Paste a direct deezer.com/track/… song URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[len(parts)-2] == "track" && digits.MatchString(parts[len(parts)-1]) {
		return parts[len(parts)-1], nil
	}
	return "", errors.New("Use a Deezer song link, not an album, playlist or short link")
}
func (a *App) deezer(ctx context.Context, path string, out any) error {
	a.providerMu.Lock()
	defer a.providerMu.Unlock()
	wait := a.metadataInterval - time.Since(a.lastMetadata)
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	a.lastMetadata = time.Now()

	req, e := http.NewRequestWithContext(ctx, "GET", a.deezerURL+path, nil)
	if e != nil {
		return errors.New("invalid Deezer request")
	}
	resp, e := a.HTTP.Do(req)
	if e != nil {
		return errors.New("Deezer metadata is unavailable; try again later")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Deezer returned HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if e != nil {
		return errors.New("could not read Deezer response")
	}
	var envelope struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &envelope) != nil {
		return errors.New("invalid Deezer response")
	}
	if envelope.Error != nil {
		return fmt.Errorf("Deezer returned error %d", envelope.Error.Code)
	}
	return json.Unmarshal(b, out)
}
func (a *App) track(ctx context.Context, input string) (Track, error) {
	var t Track
	id, e := a.resolveDeezerID(ctx, input)
	if e != nil {
		return t, e
	}
	e = a.deezer(ctx, "/track/"+id, &t)
	if e == nil && (t.ID == 0 || strconv.FormatInt(t.ID, 10) != id || t.Title == "") {
		e = errors.New("Deezer did not return the requested recording")
	}
	return t, e
}
func (a *App) searchDeezer(ctx context.Context, q string) ([]Track, error) {
	if strings.TrimSpace(q) == "" {
		return []Track{}, nil
	}
	var out struct {
		Data []Track `json:"data"`
	}
	e := a.deezer(ctx, "/search/track?limit=25&q="+url.QueryEscape(q), &out)
	if out.Data == nil {
		out.Data = []Track{}
	}
	return out.Data, e
}
func (a *App) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 500 {
		fail(w, 400, "Search is too long")
		return
	}
	if strings.HasPrefix(q, "http://") || strings.HasPrefix(q, "https://") || strings.HasPrefix(q, "deezer:") {
		resource, err := a.resolveDeezerResource(r.Context(), q)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		if strings.HasPrefix(resource, "album:") {
			if r.URL.Query().Get("details") == "1" {
				album, e := a.albumSnapshot(r.Context(), strings.TrimPrefix(resource, "album:"))
				if e != nil {
					fail(w, 502, e.Error())
					return
				}
				send(w, 200, map[string]any{"tracks": album.Tracks, "album": map[string]any{"id": album.ID, "title": album.Title, "artist": album.Artist, "trackCount": len(album.Tracks)}})
				return
			}
			tracks, e := a.albumTracks(r.Context(), strings.TrimPrefix(resource, "album:"))
			if e != nil {
				fail(w, 502, e.Error())
				return
			}
			send(w, 200, tracks)
			return
		}
		t, e := a.track(r.Context(), q)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		send(w, 200, []Track{t})
		return
	}
	if r.URL.Query().Get("details") == "1" {
		tracks, trackErr := a.searchDeezer(r.Context(), q)
		albums, albumErr := a.searchAlbums(r.Context(), q)
		if trackErr != nil && albumErr != nil {
			fail(w, 502, "Deezer search is unavailable; try again later")
			return
		}
		warnings := []string{}
		if trackErr != nil {
			tracks = []Track{}
			warnings = append(warnings, "Song search is temporarily unavailable. Album results are shown.")
		}
		if albumErr != nil {
			albums = []AlbumSummary{}
			warnings = append(warnings, "Album search is temporarily unavailable. Song results are shown.")
		}
		send(w, 200, map[string]any{"tracks": tracks, "albums": albums, "warnings": warnings})
		return
	}
	out, e := a.searchDeezer(r.Context(), q)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	send(w, 200, out)
}
func trackLabel(t Track) string {
	if t.TitleVersion != "" && !strings.Contains(strings.ToLower(t.Title), strings.ToLower(t.TitleVersion)) {
		return t.Title + " " + t.TitleVersion
	}
	return t.Title
}

func artistMatches(t Track, artist string) bool {
	if normalized(t.Artist.Name) == normalized(artist) {
		return true
	}
	names := []string{}
	for _, a := range t.Contributors {
		names = append(names, a.Name)
	}
	if len(names) == 0 {
		return false
	}
	for _, sep := range []string{", ", "/", " / ", "; "} {
		if normalized(strings.Join(names, sep)) == normalized(artist) {
			return true
		}
	}
	return false
}

// Resolve only Deezer-owned HTTPS share redirects. Never fetch arbitrary URLs
// supplied in a redirect or a fallback query parameter.
func (a *App) resolveDeezerResource(ctx context.Context, input string) (string, error) {
	if id, e := deezerResourceID(input); e == nil {
		return id, nil
	}
	current, e := url.Parse(strings.TrimSpace(input))
	if e != nil {
		return "", errors.New("Invalid Deezer song URL")
	}
	client := *a.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for hop := 0; hop < 6; hop++ {
		if current.Scheme != "https" || current.User != nil || (current.Host != "link.deezer.com" && current.Host != "www.deezer.com" && current.Host != "deezer.com") {
			return "", errors.New("Deezer share link redirected outside supported Deezer hosts")
		}
		if id, e := deezerResourceID(current.String()); e == nil {
			return id, nil
		}
		for _, key := range []string{"dest", "awf", "gwf", "iwf"} {
			if target := current.Query().Get(key); target != "" {
				if id, e := deezerResourceID(target); e == nil {
					return id, nil
				}
			}
		}
		req, e := http.NewRequestWithContext(ctx, "GET", current.String(), nil)
		if e != nil {
			return "", errors.New("Invalid Deezer share link")
		}
		resp, e := client.Do(req)
		if e != nil {
			return "", errors.New("Could not resolve Deezer share link; try again later")
		}
		resp.Body.Close()
		if resp.StatusCode < 300 || resp.StatusCode > 399 {
			return "", errors.New("Deezer share link did not resolve to a song")
		}
		next, e := resp.Location()
		if e != nil {
			return "", errors.New("Deezer share link has no valid redirect")
		}
		current = next
	}
	return "", errors.New("Too many Deezer share-link redirects")
}

func deezerResourceID(input string) (string, error) {
	if strings.HasPrefix(input, "deezer:album:") && digits.MatchString(strings.TrimPrefix(input, "deezer:album:")) {
		return "album:" + strings.TrimPrefix(input, "deezer:album:"), nil
	}
	if id, e := deezerID(input); e == nil {
		return id, nil
	}
	u, e := url.Parse(strings.TrimSpace(input))
	if e == nil && u.User == nil && (u.Scheme == "https" || u.Scheme == "http") && (u.Host == "www.deezer.com" || u.Host == "deezer.com") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 && parts[len(parts)-2] == "album" && digits.MatchString(parts[len(parts)-1]) {
			return "album:" + parts[len(parts)-1], nil
		}
	}
	return "", errors.New("Use a Deezer song or album URL")
}
func (a *App) resolveDeezerID(ctx context.Context, input string) (string, error) {
	id, e := a.resolveDeezerResource(ctx, input)
	if e == nil && strings.HasPrefix(id, "album:") {
		return "", errors.New("Choose an individual song from the album results")
	}
	return id, e
}
func (a *App) albumTracks(ctx context.Context, id string) ([]Track, error) {
	var album struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	if e := a.deezer(ctx, "/album/"+id, &album); e != nil {
		return nil, e
	}
	if album.ID == 0 {
		return nil, errors.New("Album not found")
	}
	all := []Track{}
	for offset := 0; offset < 5000; offset += 100 {
		var page struct {
			Data  []Track `json:"data"`
			Total int     `json:"total"`
		}
		if e := a.deezer(ctx, "/album/"+id+"/tracks?limit=100&index="+strconv.Itoa(offset), &page); e != nil {
			return nil, e
		}
		for _, t := range page.Data {
			t.Album.Title = album.Title
			all = append(all, t)
		}
		if len(page.Data) < 100 || (page.Total > 0 && len(all) >= page.Total) {
			return all, nil
		}
	}
	return nil, errors.New("Album exceeds 5,000 tracks")
}

type AlbumSummary struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Artist     Artist `json:"artist"`
	TrackCount int    `json:"trackCount,omitempty"`
}

func (a *App) searchAlbums(ctx context.Context, q string) ([]AlbumSummary, error) {
	out := []AlbumSummary{}
	if strings.TrimSpace(q) == "" {
		return out, nil
	}
	var response struct {
		Data []struct {
			ID     int64  `json:"id"`
			Title  string `json:"title"`
			Artist Artist `json:"artist"`
			Count  int    `json:"nb_tracks"`
		} `json:"data"`
	}
	if e := a.deezer(ctx, "/search/album?limit=20&q="+url.QueryEscape(q), &response); e != nil {
		return nil, e
	}
	seen := map[int64]bool{}
	for _, a := range response.Data {
		if a.ID > 0 && a.Title != "" && !seen[a.ID] {
			seen[a.ID] = true
			out = append(out, AlbumSummary{ID: a.ID, Title: a.Title, Artist: a.Artist, TrackCount: a.Count})
		}
	}
	return out, nil
}
