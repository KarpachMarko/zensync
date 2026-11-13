package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// Provider IDs are namespaced; a numeric ID alone always retains its legacy Deezer meaning.
type mediaProvider interface {
	Ready() error
	Search(context.Context, string) ([]Track, error)
	Track(context.Context, string) (Track, error)
	Acquire(context.Context, string, string, Track, string) (string, string, error)
}
type deezerProvider struct{ app *App }

func (p deezerProvider) Ready() error {
	var arl string
	if p.app.unseal("deezer", &arl) != nil || arl == "" {
		return errors.New("Configure Deezer in Settings to acquire music")
	}
	return nil
}
func (p deezerProvider) Search(ctx context.Context, q string) ([]Track, error) {
	return p.app.searchDeezer(ctx, q)
}
func (p deezerProvider) Track(ctx context.Context, target string) (Track, error) {
	return p.app.deezerTrack(ctx, target)
}
func (p deezerProvider) Acquire(ctx context.Context, job, target string, t Track, profile string) (string, string, error) {
	var arl string
	if p.app.unseal("deezer", &arl) != nil || arl == "" {
		return "", "", errors.New("Configure Deezer in Settings to acquire music")
	}
	return p.app.acquire(ctx, job, target, t, profile, arl)
}
func (a *App) mediaProvider(name string) (mediaProvider, error) {
	switch name {
	case "", "deezer":
		return deezerProvider{a}, nil
	case "hitmo":
		return hitmoProvider{a}, nil
	case "lossless":
		return losslessProvider{a}, nil
	}
	return nil, errors.New("Unknown media provider")
}
func trackTarget(t Track) string {
	if t.Provider == "hitmo" {
		return "hitmo:track:" + strconv.FormatInt(t.ID, 10)
	}
	if t.Provider == "lossless" {
		return "lossless:track:" + t.Source + ":" + strconv.FormatInt(t.ID, 10)
	}
	return "deezer:track:" + strconv.FormatInt(t.ID, 10)
}
func (a *App) track(ctx context.Context, target string) (Track, error) {
	name := "deezer"
	if strings.HasPrefix(target, "lossless:") || strings.HasPrefix(target, "https://lossless.am/") {
		name = "lossless"
	}
	if strings.HasPrefix(target, "hitmo:") || strings.HasPrefix(target, "https://eu.hitmoz.com/") {
		name = "hitmo"
	}
	p, err := a.mediaProvider(name)
	if err != nil {
		return Track{}, err
	}
	return p.Track(ctx, target)
}
func (a *App) acquireProvider(ctx context.Context, job, target string, t Track, profile string) (string, string, error) {
	p, err := a.mediaProvider(t.Provider)
	if err != nil {
		return "", "", err
	}
	return p.Acquire(ctx, job, target, t, profile)
}
func (a *App) searchProvider(w http.ResponseWriter, r *http.Request, name string) {
	p, err := a.mediaProvider(name)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 500 {
		fail(w, 400, "Search is too long")
		return
	}
	tracks, err := p.Search(r.Context(), q)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if r.URL.Query().Get("details") == "1" {
		providerName := "Lossless.am"
		if name == "hitmo" {
			providerName = "Hitmo"
		}
		warning := providerName + " supports individual song requests. Album requests and previews are unavailable."
		if name == "hitmo" {
			warning = "Hitmo supports individual MP3 songs and full-song listening. Album requests are unavailable."
		}
		send(w, 200, map[string]any{"tracks": tracks, "albums": []AlbumSummary{}, "warnings": []string{warning}})
		return
	}
	send(w, 200, tracks)
}
