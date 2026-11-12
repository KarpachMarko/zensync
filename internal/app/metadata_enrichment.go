package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Enrichment resolves providers in the durable metadata worker, never in the HTTP request.
func (a *App) metadataEnrichment(w http.ResponseWriter, r *http.Request, u User) {
	c, err := a.shared()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if r.Method == "GET" {
		index, err := a.indexedNavSongs(r.Context(), c)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		songs := append([]Song{}, index.songs...)
		for i := range songs {
			songs[i].Path = ""
		}
		send(w, 200, map[string]any{"songs": songs, "limited": index.limited})
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "Method not allowed")
		return
	}
	var in struct {
		Mode     string   `json:"mode"`
		SongIDs  []string `json:"songIds"`
		AlbumIDs []string `json:"albumIds"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Mode != "missing-covers" && in.Mode != "force-covers" && in.Mode != "missing-lyrics" && in.Mode != "album-order" {
		fail(w, 400, "Choose a metadata operation")
		return
	}
	index, err := a.indexedNavSongs(r.Context(), c)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if index.limited {
		fail(w, 409, "Library index is incomplete; refusing a partial metadata operation")
		return
	}
	selected := map[string]bool{}
	albums := map[string]bool{}
	for _, sid := range in.SongIDs {
		selected[sid] = true
	}
	for _, aid := range in.AlbumIDs {
		albums[aid] = true
	}
	if in.Mode == "album-order" {
		for _, song := range index.songs {
			if selected[song.ID] && song.AlbumID != "" {
				albums[song.AlbumID] = true
			}
		}
	}
	changes := []metadataChange{}
	for _, s := range index.songs {
		if (in.Mode == "force-covers" || in.Mode == "album-order") && !selected[s.ID] && !albums[s.AlbumID] {
			continue
		}
		if in.Mode == "missing-covers" && s.CoverArt != "" {
			continue
		}
		// Start from live library values, not an unrelated previously saved editor record.
		edit := MetadataEdit{Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumArtist: s.AlbumArtist}
		changes = append(changes, metadataChange{Song: s, Edit: edit, Enrichment: in.Mode})
	}
	if len(changes) == 0 {
		fail(w, 400, "No eligible songs; select songs or albums for a forced update")
		return
	}
	jid, err := a.queueMetadata(changes, u)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	send(w, 202, map[string]any{"id": jid, "songs": len(changes)})
}

func metadataIdentity(a, b string) bool {
	return strings.TrimSpace(a) != "" && metadataKey(a) == metadataKey(b)
}
func enrichmentTrackMatch(s Song, title, artist, album string, duration float64) bool {
	return metadataIdentity(s.Title, title) && metadataIdentity(s.Artist, artist) && metadataIdentity(s.Album, album) && s.Duration > 0 && duration > 0 && math.Abs(s.Duration-duration) <= 2
}

func (a *App) resolveEnrichment(ctx context.Context, c Credentials, change metadataChange) (metadataChange, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	current, err := a.nav(ctx, c, "getSong", url.Values{"id": {change.Song.ID}, "realPath": {"true"}})
	if err != nil {
		return change, err
	}
	s := current.Song
	if s.ID != change.Song.ID || s.Title != change.Song.Title || s.Artist != change.Song.Artist || s.Album != change.Song.Album {
		return change, errors.New("Song changed since enrichment was queued")
	}
	// Inspect actual file tags before deciding that lyrics or embedded artwork are absent.
	rel, _, err := a.deletionPath(ctx, c, s)
	if err != nil {
		return change, err
	}
	ext := strings.ToLower(filepath.Ext(rel))
	if strings.Contains(change.Enrichment, "covers") && ext != ".mp3" && ext != ".flac" && ext != ".m4a" {
		change.Skip = true
		return change, nil
	}
	out, err := exec.CommandContext(ctx, a.ffprobePath, "-v", "error", "-show_format", "-show_streams", "-of", "json", filepath.Join(a.musicDir, rel)).Output()
	if err != nil {
		return change, errors.New("Cannot inspect current file metadata")
	}
	var p probe
	if err = json.Unmarshal(out, &p); err != nil {
		return change, err
	}
	tags := uploadTags(p)
	if change.Enrichment == "missing-covers" {
		if s.CoverArt != "" {
			change.Skip = true
			return change, nil
		}
		for _, stream := range p.Streams {
			if stream.Disposition.AttachedPic == 1 {
				change.Skip = true
				return change, nil
			}
		}
	}
	if change.Enrichment == "missing-lyrics" {
		for key, value := range tags {
			if strings.Contains(strings.ToLower(key), "lyric") && strings.TrimSpace(value) != "" {
				change.Skip = true
				return change, nil
			}
		}
		var rows []lyricCandidate
		err = a.metadataJSON(ctx, "https://lrclib.net/api/search?"+url.Values{"track_name": {s.Title}, "artist_name": {s.Artist}, "album_name": {s.Album}}.Encode(), &rows)
		if err != nil {
			return change, err
		}
		matches := []lyricCandidate{}
		for _, row := range rows {
			if enrichmentTrackMatch(s, row.TrackName, row.ArtistName, row.AlbumName, row.Duration) && !row.Instrumental && (row.PlainLyrics != "" || row.SyncedLyrics != "") {
				matches = append(matches, row)
			}
		}
		if len(matches) != 1 {
			change.Skip = true
			return change, nil
		}
		row := matches[0]
		if len(row.PlainLyrics) > 200000 || len(row.SyncedLyrics) > 200000 {
			change.Skip = true
			return change, nil
		}
		change.Edit.Lyrics = row.PlainLyrics
		change.Edit.SyncedLyrics = row.SyncedLyrics
		change.Edit.LyricsSource = fmt.Sprintf("LRCLIB:%d", row.ID)
		return change, nil
	}
	tracks, err := a.searchDeezer(ctx, s.Artist+" "+s.Title+" "+s.Album)
	if err != nil {
		return change, err
	}
	matches := []Track{}
	for _, track := range tracks {
		if enrichmentTrackMatch(s, track.Title, track.Artist.Name, track.Album.Title, track.Duration) {
			matches = append(matches, track)
		}
	}
	if len(matches) != 1 {
		change.Skip = true
		return change, nil
	}
	var detail struct {
		Track
		Position    int `json:"track_position"`
		Disc        int `json:"disk_number"`
		AlbumDetail struct {
			Cover string `json:"cover_xl"`
			ID    int64  `json:"id"`
		} `json:"album"`
	}
	if err = a.deezer(ctx, fmt.Sprintf("/track/%d", matches[0].ID), &detail); err != nil {
		return change, err
	}
	if detail.ID != matches[0].ID {
		return change, errors.New("Deezer returned a different recording")
	}
	if change.Enrichment == "album-order" {
		if detail.Position <= 0 || detail.Disc <= 0 || detail.AlbumDetail.ID <= 0 || detail.AlbumDetail.ID != matches[0].Album.ID {
			change.Skip = true
			return change, nil
		}
		change.OrderAlbumID = detail.AlbumDetail.ID
		change.Edit.TrackNumber = detail.Position
		change.Edit.DiscNumber = detail.Disc
	} else {
		image, err := url.Parse(detail.AlbumDetail.Cover)
		if err != nil || image.Scheme != "https" || image.User != nil || !strings.HasSuffix(image.Hostname(), ".dzcdn.net") {
			change.Skip = true
			return change, nil
		}
		change.Edit.CoverURL = image.String()
	}
	slog.Info("Metadata provider match resolved", "song_id", s.ID, "operation", change.Enrichment, "provider_track_id", detail.ID, "provider_album_id", detail.AlbumDetail.ID, "track_number", change.Edit.TrackNumber, "disc_number", change.Edit.DiscNumber)
	return change, nil
}

// Resolve the entire selected album before publishing any track numbers. A partial
// per-song match can otherwise mix editions or leave duplicate positions behind.
func (a *App) prepareAlbumOrder(ctx context.Context, c Credentials, jid string, changes []metadataChange) error {
	for i := range changes {
		if !changes[i].Resolved {
			resolved, err := a.resolveEnrichment(ctx, c, changes[i])
			if err != nil {
				return err
			}
			resolved.Resolved = true
			changes[i] = resolved
		}
	}
	groups := map[string][]int{}
	for i, change := range changes {
		groups[change.Song.AlbumID] = append(groups[change.Song.AlbumID], i)
	}
	for _, indices := range groups {
		release := int64(0)
		positions := map[string]bool{}
		valid := true
		for _, i := range indices {
			change := changes[i]
			key := fmt.Sprintf("%d:%d", change.Edit.DiscNumber, change.Edit.TrackNumber)
			if change.Song.AlbumID == "" || change.Skip || change.OrderAlbumID <= 0 || release != 0 && release != change.OrderAlbumID || positions[key] {
				valid = false
			}
			release = change.OrderAlbumID
			positions[key] = true
		}
		if !valid {
			slog.Info("Album order skipped; incomplete matches, mixed releases or duplicate positions", "job_id", jid, "album_id", changes[indices[0]].Song.AlbumID, "songs", len(indices))
			for _, i := range indices {
				changes[i].Skip = true
			}
		}
	}
	_, err := a.DB.Exec("UPDATE metadata_jobs SET changes=? WHERE id=?", mustJSON(changes), jid)
	return err
}
