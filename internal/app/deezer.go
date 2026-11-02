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
	id, e := deezerID(input)
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
		t, e := a.track(r.Context(), q)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		send(w, 200, []Track{t})
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
