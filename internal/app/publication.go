package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"zensync/internal/apple"
)

func mapValues(k, v string) url.Values { return url.Values{k: {v}} }
func normalized(s string) string       { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
func sameRecording(s apple.Song, title, artist, album string, duration float64) bool {
	t := Track{Title: title, Duration: duration}
	t.Artist.Name, t.Album.Title = artist, album
	return assessRecording(s, t).Confident
}
func (a *App) matchMigration(ctx context.Context, owner, mid string) error {
	sources, e := a.sources(mid)
	if e != nil {
		return e
	}
	c, e := a.shared()
	if e != nil {
		return e
	}
	for index, s := range sources {
		m, err := a.getMigration(mid, owner)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if m.State != "open" {
			return nil
		}
		var hasCandidate bool
		err = a.DB.QueryRow("SELECT server_id,candidate<>'',match_state,deferred FROM source_songs WHERE id=? AND migration_id=?", s.ID, mid).Scan(&s.ServerID, &hasCandidate, &s.MatchState, &s.Deferred)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		if index%10 == 0 {
			_, _ = a.DB.Exec("UPDATE migration_tasks SET message=? WHERE migration_id=? AND action='match' AND state='running'", fmt.Sprintf("Matching song %d of %d", index+1, len(sources)), mid)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.ServerID != "" || s.Metadata.Missing || s.MatchState == "manual" {
			continue
		}
		local, e := a.navSearch(ctx, c, s.Metadata.Title, 0)
		if e != nil {
			return e
		}
		matches := []Song{}
		for _, song := range local {
			if sameRecording(s.Metadata, song.Title, song.Artist, song.Album, song.Duration) {
				matches = append(matches, song)
			}
		}
		if len(matches) == 1 {
			if _, e = a.DB.Exec("UPDATE source_songs SET server_id=?,match_state='confident' WHERE id=? AND server_id='' AND match_state<>'manual' AND EXISTS(SELECT 1 FROM migrations WHERE id=source_songs.migration_id AND state='open')", matches[0].ID, s.ID); e != nil {
				return e
			}
			continue
		}
		if hasCandidate || s.Deferred {
			continue
		}
		tracks, e := a.searchImportRecordings(ctx, s.Metadata, s.Metadata.Artist+" "+s.Metadata.Title)
		if e != nil {
			return e
		}
		if candidate := confidentRecording(s.Metadata, tracks); candidate != nil {
			if _, e = a.DB.Exec("UPDATE source_songs SET candidate=?,match_state='confident' WHERE id=? AND candidate='' AND server_id='' AND deferred=0 AND match_state<>'manual' AND EXISTS(SELECT 1 FROM migrations WHERE id=source_songs.migration_id AND state='open')", apple.JSON(candidate), s.ID); e != nil {
				return e
			}
		} else {
			state := "unresolved"
			if len(tracks) > 0 || len(matches) > 1 {
				state = "ambiguous"
			}
			if _, e = a.DB.Exec("UPDATE source_songs SET match_state=? WHERE id=? AND candidate='' AND server_id='' AND deferred=0 AND match_state<>'manual' AND EXISTS(SELECT 1 FROM migrations WHERE id=source_songs.migration_id AND state='open')", state, s.ID); e != nil {
				return e
			}
		}
	}
	return nil
}

type playlistSnapshot struct {
	Name    string   `json:"name"`
	Owner   string   `json:"owner"`
	Comment string   `json:"comment"`
	Public  bool     `json:"public"`
	IDs     []string `json:"ids"`
}

func snapshot(p RemotePlaylist) playlistSnapshot {
	ids := []string{}
	for _, s := range p.Entries {
		ids = append(ids, s.ID)
	}
	return playlistSnapshot{Name: p.Name, Owner: p.Owner, Comment: p.Comment, Public: p.Public, IDs: ids}
}
func sameSnapshot(a, b string) bool {
	var x, y playlistSnapshot
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return apple.JSON(x) == apple.JSON(y)
}
func (a *App) pausePlaylist(pid, message, observed string) error {
	_, e := a.DB.Exec("UPDATE migration_playlists SET state='paused',error=?,desired=? WHERE id=?", message, observed, pid)
	return e
}
func (a *App) publishMigration(ctx context.Context, owner, mid string) error {
	m, e := a.getMigration(mid, owner)
	if e != nil {
		return e
	}
	if m.State != "open" {
		return nil
	}
	c, e := a.personal(owner)
	if e != nil {
		return e
	}
	if m.TargetUser == "" || m.TargetUser != c.Username {
		return errors.New("Publish the migration to bind your personal Navidrome account first")
	}
	sources, e := a.sources(mid)
	if e != nil {
		return e
	}
	available := map[string]Song{}
	verified := map[string]Song{}
	for index, s := range sources {
		if index%10 == 0 {
			current, err := a.getMigration(mid, owner)
			if err != nil || current.State != "open" {
				return nil
			}
		}
		if song, ok := verified[s.ServerID]; ok {
			available[s.ID] = song
			continue
		}
		if s.ServerID == "" {
			continue
		}
		out, err := a.nav(ctx, c, "getSong", mapValues("id", s.ServerID))
		if err != nil {
			return err
		}
		if out.Song.ID != s.ServerID {
			return errors.New("Navidrome returned an invalid song mapping")
		}
		available[s.ID] = out.Song
		verified[s.ServerID] = out.Song
	}
	playlists, e := a.playlists(mid)
	if e != nil {
		return e
	}
	hadPause := false
	for _, p := range playlists {
		if !p.Selected || p.State == "handed_over" {
			continue
		}
		if p.State == "paused" {
			hadPause = true
			continue
		}
		if e = a.publishCurrentPlaylist(ctx, owner, mid, c, p.ID, available); e != nil {
			return e
		}
	}
	if m.Favorites {
		for _, source := range sources {
			if song, ok := available[source.ID]; ok && source.Metadata.Favorite && source.FavoriteState != "applied" && source.FavoriteState != "review" {
				if e = a.publishCurrentFavorite(ctx, owner, mid, c, source.ID, song); e != nil {
					return e
				}
			}
		}
	}
	var pauses int
	if e = a.DB.QueryRow("SELECT count(*) FROM migration_playlists WHERE migration_id=? AND selected=1 AND state='paused'", mid).Scan(&pauses); e != nil {
		return e
	}
	if hadPause || pauses > 0 {
		return errors.New("Some playlists are paused after external changes; review them before resuming")
	}
	return nil
}

// Publication holds the mutation lock only while applying one collection or
// favorite, never while verifying thousands of existing library songs.
func (a *App) publishCurrentPlaylist(ctx context.Context, owner, mid string, c Credentials, pid string, available map[string]Song) error {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	m, err := a.getMigration(mid, owner)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if m.State != "open" || m.TargetUser != c.Username {
		return nil
	}
	playlists, err := a.playlists(mid)
	if err != nil {
		return err
	}
	var current *Playlist
	for i := range playlists {
		if playlists[i].ID == pid {
			current = &playlists[i]
			break
		}
	}
	if current == nil || !current.Selected || current.State == "paused" || current.State == "handed_over" {
		return nil
	}
	// Resolutions can change while getSong runs. Use only mappings that were
	// verified and still point at the same recording at publication time.
	rows, err := a.DB.Query("SELECT id,server_id FROM source_songs WHERE migration_id=?", mid)
	if err != nil {
		return err
	}
	mappings := map[string]string{}
	for rows.Next() {
		var sid, server string
		if err = rows.Scan(&sid, &server); err != nil {
			rows.Close()
			return err
		}
		mappings[sid] = server
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	ids := []string{}
	for _, sid := range current.Entries {
		if song, ok := available[sid]; ok && mappings[sid] == song.ID {
			ids = append(ids, song.ID)
		}
	}
	if len(ids) == 0 && current.RemoteID == "" {
		return nil
	}
	return a.publishPlaylist(ctx, c, *current, ids)
}

func (a *App) publishCurrentFavorite(ctx context.Context, owner, mid string, c Credentials, sid string, song Song) error {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	m, err := a.getMigration(mid, owner)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if m.State != "open" || !m.Favorites || m.TargetUser != c.Username {
		return nil
	}
	sources, err := a.sources(mid, sid)
	if err != nil {
		return err
	}
	if len(sources) != 1 {
		return nil
	}
	s := sources[0]
	if s.ServerID != song.ID || !s.Metadata.Favorite || s.FavoriteState == "applied" || s.FavoriteState == "review" {
		return nil
	}
	var e error
	if s.FavoriteState == "writing" {
		if song.Starred != "" {
			_, e = a.DB.Exec("UPDATE source_songs SET favorite_state='applied' WHERE id=?", s.ID)
		} else {
			_, e = a.DB.Exec("UPDATE source_songs SET favorite_state='review' WHERE id=?", s.ID)
		}
		if e != nil {
			return e
		}
		return nil
	}
	if song.Starred == "" {
		if _, e = a.DB.Exec("UPDATE source_songs SET favorite_state='writing' WHERE id=?", s.ID); e != nil {
			return e
		}
		if _, e = a.nav(ctx, c, "star", mapValues("id", song.ID)); e != nil {
			return e
		}
		check, err := a.nav(ctx, c, "getSong", mapValues("id", song.ID))
		if err != nil {
			return err
		}
		if check.Song.Starred == "" {
			_, _ = a.DB.Exec("UPDATE source_songs SET favorite_state='review' WHERE id=?", s.ID)
			return nil
		}
	}
	if _, e = a.DB.Exec("UPDATE source_songs SET favorite_state='applied' WHERE id=?", s.ID); e != nil {
		return e
	}

	return nil
}

func (a *App) publishPlaylist(ctx context.Context, c Credentials, p Playlist, ids []string) error {
	name := p.Name + " · ZenSync " + p.ID[:8]
	desired := playlistSnapshot{Name: name, Owner: c.Username, IDs: ids}
	if p.RemoteID == "" {
		if p.State == "creating" {
			out, e := a.nav(ctx, c, "getPlaylists", nil)
			if e != nil {
				return e
			}
			matches := []RemotePlaylist{}
			for _, remote := range out.Playlists.Items {
				if remote.Name == name && remote.Owner == c.Username {
					matches = append(matches, remote)
				}
			}
			if len(matches) != 1 {
				return a.pausePlaylist(p.ID, "A prior create has an uncertain outcome. Inspect Navidrome; this playlist will not be recreated automatically.", "")
			}
			p.RemoteID = matches[0].ID
			out, e = a.nav(ctx, c, "getPlaylist", mapValues("id", p.RemoteID))
			if e != nil {
				return e
			}
			observed := apple.JSON(snapshot(out.Playlist))
			if !sameSnapshot(observed, p.Desired) {
				return a.pausePlaylist(p.ID, "Recovered playlist differs from the pending write. Review external changes.", observed)
			}
			_, e = a.DB.Exec("UPDATE migration_playlists SET remote_id=?,baseline=?,state='active',error='' WHERE id=?", p.RemoteID, observed, p.ID)
			if e != nil {
				return e
			}
			p.Baseline = observed
			p.State = "active"
		} else {
			if _, e := a.DB.Exec("UPDATE migration_playlists SET state='creating',desired=? WHERE id=?", apple.JSON(desired), p.ID); e != nil {
				return e
			}
			params := url.Values{"name": {name}, "songId": ids}
			out, e := a.nav(ctx, c, "createPlaylist", params)
			if e != nil {
				return e
			}
			if out.Playlist.ID == "" {
				return errors.New("Navidrome did not return the created playlist ID")
			}
			p.RemoteID = out.Playlist.ID
			if _, e = a.DB.Exec("UPDATE migration_playlists SET remote_id=?,state='writing' WHERE id=?", p.RemoteID, p.ID); e != nil {
				return e
			}
			p.Desired = apple.JSON(desired)
			p.State = "writing"
		}
	}
	out, e := a.nav(ctx, c, "getPlaylist", mapValues("id", p.RemoteID))
	if e != nil {
		return a.pausePlaylist(p.ID, "Playlist is missing or inaccessible. Restore access or hand it over; it will not be recreated automatically.", "")
	}
	observed := apple.JSON(snapshot(out.Playlist))
	if out.Playlist.Owner != c.Username {
		return a.pausePlaylist(p.ID, "Remote playlist belongs to a different account.", observed)
	}
	if p.State == "writing" {
		if sameSnapshot(observed, p.Desired) {
			if _, e = a.DB.Exec("UPDATE migration_playlists SET baseline=?,state='active',error='' WHERE id=?", observed, p.ID); e != nil {
				return e
			}
			p.Baseline = observed
		} else {
			return a.pausePlaylist(p.ID, "Prior write outcome is uncertain or differs from the proposed membership.", observed)
		}
	}
	if !sameSnapshot(observed, p.Baseline) {
		return a.pausePlaylist(p.ID, "External playlist changes detected. Review before replacing them.", observed)
	}
	base := snapshot(out.Playlist)
	desired.Name = base.Name
	desired.Comment = base.Comment
	desired.Public = base.Public
	if sameSnapshot(observed, apple.JSON(desired)) {
		return nil
	}
	if _, e = a.DB.Exec("UPDATE migration_playlists SET state='writing',desired=? WHERE id=?", apple.JSON(desired), p.ID); e != nil {
		return e
	}
	// createPlaylist with playlistId replaces ordered membership, including duplicates.
	if _, e = a.nav(ctx, c, "createPlaylist", url.Values{"playlistId": {p.RemoteID}, "songId": ids}); e != nil {
		return e
	}
	check, e := a.nav(ctx, c, "getPlaylist", mapValues("id", p.RemoteID))
	if e != nil {
		return e
	}
	verified := apple.JSON(snapshot(check.Playlist))
	if !sameSnapshot(verified, apple.JSON(desired)) {
		return a.pausePlaylist(p.ID, "Navidrome read-back differs from the proposed playlist.", verified)
	}
	_, e = a.DB.Exec("UPDATE migration_playlists SET baseline=?,state='active',error='' WHERE id=?", verified, p.ID)
	return e
}
func (a *App) resumePlaylist(ctx context.Context, owner, pid, reviewed string) error {
	var remoteID, observed string
	if e := a.DB.QueryRow("SELECT remote_id,desired FROM migration_playlists WHERE id=?", pid).Scan(&remoteID, &observed); e != nil {
		return e
	}
	if remoteID == "" {
		return errors.New("Uncertain creation has no verified remote ID; hand over this playlist after inspecting Navidrome")
	}
	if reviewed == "" || !sameSnapshot(reviewed, observed) {
		return errors.New("Review the current external snapshot before resuming")
	}
	c, e := a.personal(owner)
	if e != nil {
		return e
	}
	out, e := a.nav(ctx, c, "getPlaylist", mapValues("id", remoteID))
	if e != nil {
		return e
	}
	current := apple.JSON(snapshot(out.Playlist))
	if !sameSnapshot(reviewed, current) {
		_ = a.pausePlaylist(pid, "Playlist changed again; review the latest snapshot", current)
		return errors.New("Playlist changed again; reload and review")
	}
	if out.Playlist.Owner != c.Username {
		return errors.New("Playlist ownership changed")
	}
	_, e = a.DB.Exec("UPDATE migration_playlists SET state='active',baseline=?,error='' WHERE id=?", current, pid)
	return e
}
