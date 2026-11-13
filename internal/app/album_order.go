package app

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"time"
)

type albumOrderTrack struct {
	Track
	Disc     int `json:"disk_number"`
	Position int `json:"track_position"`
}
type albumOrderReference struct {
	ID            int64
	Title, Artist string
	Tracks        []albumOrderTrack
}

func primaryOrderArtist(s Song) string {
	if len(s.Artists) > 0 && s.Artists[0].Name != "" {
		return s.Artists[0].Name
	}
	return s.Artist
}

func (a *App) lookupAlbumOrder(ctx context.Context, c Credentials, s Song) (albumOrderReference, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	var reference albumOrderReference
	album, err := a.nav(ctx, c, "getAlbum", url.Values{"id": {s.AlbumID}})
	if err != nil {
		return reference, err
	}
	if album.Album.ID != s.AlbumID || strings.TrimSpace(album.Album.Name) == "" {
		return reference, fmt.Errorf("Current library album is unavailable")
	}
	title, artist := album.Album.Name, album.Album.Artist
	if artist == "" {
		artist = s.AlbumArtist
	}
	if artist == "" {
		artist = primaryOrderArtist(s)
	}
	var search struct {
		Data []struct {
			ID     int64  `json:"id"`
			Title  string `json:"title"`
			Artist Artist `json:"artist"`
		} `json:"data"`
	}
	if err = a.deezer(ctx, "/search/album?limit=100&q="+url.QueryEscape(artist+" "+title), &search); err != nil {
		return reference, err
	}
	matches := map[int64]bool{}
	for _, candidate := range search.Data {
		if candidate.ID > 0 && metadataIdentity(title, candidate.Title) && metadataIdentity(artist, candidate.Artist.Name) {
			matches[candidate.ID] = true
			reference = albumOrderReference{ID: candidate.ID, Title: title, Artist: artist}
		}
	}
	if len(matches) != 1 {
		return reference, fmt.Errorf("No unique reference album for %s / %s", artist, title)
	}
	counters := map[int]int{}
	for offset := 0; offset < 5000; offset += 100 {
		var page struct {
			Data  []albumOrderTrack `json:"data"`
			Total int               `json:"total"`
		}
		if err = a.deezer(ctx, fmt.Sprintf("/album/%d/tracks?limit=100&index=%d", reference.ID, offset), &page); err != nil {
			return reference, err
		}
		for _, track := range page.Data {
			if track.Disc <= 0 {
				track.Disc = 1
			}
			counters[track.Disc]++
			if track.Position <= 0 {
				track.Position = counters[track.Disc]
			} else {
				counters[track.Disc] = track.Position
			}
			reference.Tracks = append(reference.Tracks, track)
		}
		if len(page.Data) < 100 || page.Total > 0 && len(reference.Tracks) >= page.Total {
			return reference, nil
		}
	}
	return reference, fmt.Errorf("Reference album exceeds 5,000 tracks")
}

func matchAlbumOrder(change metadataChange, reference albumOrderReference) metadataChange {
	change.Skip = true
	change.Edit.TrackNumber = 0
	change.Edit.DiscNumber = 0
	matches := []albumOrderTrack{}
	for _, track := range reference.Tracks {
		if metadataIdentity(change.Song.Title, track.Title) && metadataIdentity(primaryOrderArtist(change.Song), track.Artist.Name) && change.Song.Duration > 0 && track.Duration > 0 && math.Abs(change.Song.Duration-track.Duration) <= 2 {
			matches = append(matches, track)
		}
	}
	if len(matches) == 1 {
		track := matches[0]
		change.Skip = false
		change.OrderAlbumID = reference.ID
		change.Edit.TrackNumber = track.Position
		change.Edit.DiscNumber = track.Disc
	}
	change.Resolved = true
	change.OrderVersion = 2
	return change
}

// Reference identity comes from the current library album, never the files' origins.
func (a *App) prepareAlbumOrder(ctx context.Context, c Credentials, jid string, changes []metadataChange) error {
	groups := map[string][]int{}
	order := []string{}
	for i, change := range changes {
		if _, ok := groups[change.Song.AlbumID]; !ok {
			order = append(order, change.Song.AlbumID)
		}
		groups[change.Song.AlbumID] = append(groups[change.Song.AlbumID], i)
	}
	for _, albumID := range order {
		indices := groups[albumID]
		pending := false
		for _, i := range indices {
			if !changes[i].Resolved || changes[i].OrderVersion != 2 {
				pending = true
			}
		}
		if !pending {
			continue
		}
		reference, lookupErr := a.lookupAlbumOrder(ctx, c, changes[indices[0]].Song)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if lookupErr != nil {
			slog.Warn("Album reference lookup skipped", "job_id", jid, "album_id", albumID, "error", lookupErr)
		} else {
			slog.Info("Album order reference selected", "job_id", jid, "album_id", albumID, "provider_album_id", reference.ID, "album", reference.Title)
		}
		for _, i := range indices {
			if changes[i].Resolved && changes[i].OrderVersion == 2 {
				continue
			}
			if lookupErr != nil {
				changes[i].Skip = true
				changes[i].Resolved = true
				changes[i].OrderVersion = 2
			} else {
				current, err := a.nav(ctx, c, "getSong", url.Values{"id": {changes[i].Song.ID}})
				original := changes[i].Song
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err != nil || current.Song.ID != original.ID || current.Song.Title != original.Title || current.Song.Artist != original.Artist || current.Song.Album != original.Album {
					changes[i].Skip = true
					changes[i].Resolved = true
					changes[i].OrderVersion = 2
				} else {
					changes[i] = matchAlbumOrder(changes[i], reference)
				}
			}
			change := changes[i]
			slog.Info("Album order song matched", "job_id", jid, "song_id", change.Song.ID, "provider_album_id", reference.ID, "skipped", change.Skip, "track_number", change.Edit.TrackNumber, "disc_number", change.Edit.DiscNumber)
			if _, err := a.DB.Exec("UPDATE metadata_jobs SET changes=? WHERE id=?", mustJSON(changes), jid); err != nil {
				return err
			}
		}
	}
	return nil
}
