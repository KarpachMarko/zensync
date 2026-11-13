package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Album IDs may be opaque (for example Qobuz), while legacy Deezer IDs stay numeric.
type losslessAlbumID string

func (id *losslessAlbumID) UnmarshalJSON(data []byte) error {
	var value string
	if string(data) == "null" {
		*id = ""
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
	} else {
		if !digits.MatchString(string(data)) {
			return errors.New("Invalid Lossless.am album ID")
		}
		value = string(data)
	}
	if value == "" {
		*id = ""
		return nil
	}
	if !losslessAlbumIDPattern.MatchString(value) || value == "0" {
		return errors.New("Invalid Lossless.am album ID")
	}
	*id = losslessAlbumID(value)
	return nil
}

var losslessAlbumIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type losslessPicture struct {
	Medium string `json:"medium"`
}
type losslessAlbumRecord struct {
	ID      losslessAlbumID `json:"id"`
	Title   string          `json:"title"`
	Source  string          `json:"source"`
	Artists []Artist        `json:"artists"`
	Count   int             `json:"number_of_tracks"`
	Picture losslessPicture `json:"picture"`
}
type losslessTrackRecord struct {
	ID        int64               `json:"id"`
	Title     string              `json:"title"`
	Duration  float64             `json:"duration"`
	Source    string              `json:"source"`
	ISRC      string              `json:"isrc"`
	Explicit  bool                `json:"explicit"`
	Available *bool               `json:"streaming_available"`
	Artists   []Artist            `json:"artists"`
	Album     losslessAlbumRecord `json:"album"`
}
type losslessCatalogResult struct {
	Tracks   []Track        `json:"tracks"`
	Albums   []AlbumSummary `json:"albums"`
	Album    *AlbumSummary  `json:"album,omitempty"`
	Warnings []string       `json:"warnings"`
}

func normalizeLosslessTrack(raw losslessTrackRecord) (Track, error) {
	if raw.ID <= 0 || strings.TrimSpace(raw.Title) == "" || len(raw.Artists) == 0 || strings.TrimSpace(raw.Artists[0].Name) == "" || !validDuration(raw.Duration) || !losslessSource.MatchString(raw.Source) {
		return Track{}, errors.New("Incomplete Lossless.am recording")
	}
	t := Track{Provider: "lossless", Source: raw.Source, ID: raw.ID, Title: raw.Title, Duration: raw.Duration, ISRC: raw.ISRC, Contributors: raw.Artists, ExplicitLyrics: &raw.Explicit, Readable: raw.Available}
	t.Artist.Name = raw.Artists[0].Name
	t.Album.Title = raw.Album.Title
	t.Album.Cover = raw.Album.Picture.Medium
	t.Album.ID, _ = strconv.ParseInt(string(raw.Album.ID), 10, 64)
	return t, nil
}
func (p losslessProvider) rememberTracks(tracks []Track) error {
	for _, t := range tracks {
		if _, err := p.app.DB.Exec("INSERT INTO provider_tracks(target,metadata) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET metadata=excluded.metadata", trackTarget(t), mustJSON(t)); err != nil {
			return err
		}
	}
	return nil
}
func losslessAlbumSummary(raw losslessAlbumRecord) (AlbumSummary, error) {
	if raw.ID == "" || raw.ID == "0" || !losslessAlbumIDPattern.MatchString(string(raw.ID)) || !losslessSource.MatchString(raw.Source) || strings.TrimSpace(raw.Title) == "" || len(raw.Artists) == 0 || strings.TrimSpace(raw.Artists[0].Name) == "" || raw.Count < 0 || raw.Count > 5000 {
		return AlbumSummary{}, errors.New("Incomplete Lossless.am album")
	}
	id, _ := strconv.ParseInt(string(raw.ID), 10, 64)
	return AlbumSummary{Provider: "lossless", Source: raw.Source, SourceID: string(raw.ID), ID: id, Title: raw.Title, Artist: raw.Artists[0], TrackCount: raw.Count}, nil
}
func losslessAlbumTarget(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "lossless:album:") && albumTargetPattern.MatchString(input) && strings.Split(input, ":")[3] != "0" {
		return input, nil
	}
	u, err := url.Parse(input)
	if err == nil && u.Scheme == "https" && u.Host == "lossless.am" && u.User == nil && u.Fragment == "" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		// Accept the provider's query-source links as well as captured source/id links.
		if len(parts) == 3 && parts[0] == "app" && parts[1] == "album" {
			query, err := url.ParseQuery(u.RawQuery)
			if err == nil && len(query) == 1 && len(query["source"]) == 1 {
				candidate := "lossless:album:" + query.Get("source") + ":" + parts[2]
				if albumTargetPattern.MatchString(candidate) && parts[2] != "0" {
					return candidate, nil
				}
			}
		}
		if len(parts) == 4 && parts[0] == "app" && parts[1] == "album" && u.RawQuery == "" {
			candidate := "lossless:album:" + parts[2] + ":" + parts[3]
			if albumTargetPattern.MatchString(candidate) && parts[3] != "0" {
				return candidate, nil
			}
		}
	}
	return "", errors.New("Use a direct lossless.am/app/album/source/id album URL")
}
func (p losslessProvider) searchCatalog(ctx context.Context, q string) (losslessCatalogResult, error) {
	result := losslessCatalogResult{Tracks: []Track{}, Albums: []AlbumSummary{}, Warnings: []string{}}
	q = strings.TrimSpace(q)
	if q == "" {
		return result, nil
	}
	if target, err := losslessAlbumTarget(q); err == nil {
		album, err := p.Album(ctx, target)
		if err != nil {
			return result, err
		}
		summary := AlbumSummary{Provider: album.Provider, Source: album.Source, SourceID: album.SourceID, ID: album.ID, Title: album.Title, Artist: album.Artist, TrackCount: len(album.Tracks)}
		result.Album = &summary
		result.Tracks = album.Tracks
		return result, nil
	}
	if strings.HasPrefix(q, "https://") || strings.HasPrefix(q, "lossless:") {
		track, err := p.Track(ctx, q)
		if err != nil {
			return result, err
		}
		result.Tracks = append(result.Tracks, track)
		return result, nil
	}
	body, _ := json.Marshal(map[string]string{"query": q})
	resp, err := p.request(ctx, "POST", "/api/search", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	var raw struct {
		Tracks []losslessTrackRecord `json:"tracks"`
		Albums []losslessAlbumRecord `json:"albums"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw) != nil {
		return result, errors.New("Invalid Lossless.am search response")
	}
	for _, record := range raw.Tracks {
		track, err := normalizeLosslessTrack(record)
		if err == nil {
			result.Tracks = append(result.Tracks, track)
		}
	}
	seen := map[string]bool{}
	for _, record := range raw.Albums {
		album, err := losslessAlbumSummary(record)
		if err == nil && album.TrackCount != 1 && !seen[albumTarget(album)] {
			seen[albumTarget(album)] = true
			result.Albums = append(result.Albums, album)
		}
	}
	if err = p.rememberTracks(result.Tracks); err != nil {
		return result, err
	}
	return result, nil
}
func (p losslessProvider) Album(ctx context.Context, input string) (AlbumSnapshot, error) {
	var album AlbumSnapshot
	target, err := losslessAlbumTarget(input)
	if err != nil {
		return album, err
	}
	parts := strings.Split(target, ":")
	resp, err := p.request(ctx, "GET", "/api/album/"+url.PathEscape(parts[3])+"?source="+url.QueryEscape(parts[2]), nil)
	if err != nil {
		return album, err
	}
	defer resp.Body.Close()
	var raw struct {
		Album  losslessAlbumRecord   `json:"album"`
		Tracks []losslessTrackRecord `json:"tracks"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&raw) != nil {
		return album, errors.New("Invalid Lossless.am album response")
	}
	if raw.Album.Source == "" {
		raw.Album.Source = parts[2]
	}
	summary, err := losslessAlbumSummary(raw.Album)
	if err != nil || albumTarget(summary) != target {
		return album, errors.New("Lossless.am did not return the requested album")
	}
	if len(raw.Tracks) == 0 || len(raw.Tracks) > 5000 || (raw.Album.Count > 0 && raw.Album.Count != len(raw.Tracks)) {
		return album, errors.New("Lossless.am album track list is empty, incomplete or exceeds 5,000 tracks")
	}
	album = AlbumSnapshot{Provider: "lossless", Source: summary.Source, SourceID: summary.SourceID, ID: summary.ID, Title: summary.Title, Artist: summary.Artist, Tracks: []Track{}}
	seen := map[string]bool{}
	for _, record := range raw.Tracks {
		if record.Source == "" {
			record.Source = summary.Source
		}
		if record.Album.ID != "" && record.Album.ID != raw.Album.ID {
			return AlbumSnapshot{}, errors.New("Lossless.am returned a track from a different album")
		}
		if record.Album.Source != "" && record.Album.Source != summary.Source {
			return AlbumSnapshot{}, errors.New("Lossless.am returned a track from a different catalog")
		}
		if record.Album.ID == "" {
			record.Album.ID = raw.Album.ID
		}
		if record.Album.Title == "" {
			record.Album.Title = summary.Title
		}
		if record.Album.Picture.Medium == "" {
			record.Album.Picture = raw.Album.Picture
		}
		track, err := normalizeLosslessTrack(record)
		if err != nil || track.Source != summary.Source || seen[trackTarget(track)] {
			return AlbumSnapshot{}, errors.New("Lossless.am album includes an incomplete, duplicate or mismatched recording")
		}
		seen[trackTarget(track)] = true
		album.Tracks = append(album.Tracks, track)
	}
	if err = p.rememberTracks(album.Tracks); err != nil {
		return AlbumSnapshot{}, err
	}
	return album, nil
}
