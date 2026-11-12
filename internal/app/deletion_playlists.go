package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
)

func playlistIDs(playlist RemotePlaylist) []string {
	ids := make([]string, 0, len(playlist.Entries))
	for _, song := range playlist.Entries {
		ids = append(ids, song.ID)
	}
	return ids
}

// Playlist replacement is journaled before writing. A failed/uncertain response
// is reconciled against the remote list on retry, before any file is deleted.
func (a *App) replaceDeletionPlaylists(ctx context.Context, rid, source, sourcePath string) error {
	var keep, server string
	err := a.DB.QueryRow("SELECT keep_song_id,server_url FROM deletion_replacements WHERE request_id=?", rid).Scan(&keep, &server)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	shared, err := a.shared()
	if err != nil {
		return err
	}
	if shared.URL != server {
		return errors.New("Navidrome connection changed; playlist preservation stopped before deletion")
	}
	kept, err := a.nav(ctx, shared, "getSong", url.Values{"id": {keep}, "realPath": {"true"}})
	if err != nil || kept.Song.ID != keep {
		return errors.New("Kept song is unavailable; no duplicate file removed")
	}
	rel, _, err := a.deletionPath(ctx, shared, kept.Song)
	if err != nil || rel == sourcePath {
		return errors.New("Kept song file is unavailable or aliases the duplicate; no file removed")
	}
	keys := []string{"navidrome"}
	rows, err := a.DB.Query("SELECT name FROM secrets WHERE name LIKE 'navidrome:%' ORDER BY name")
	if err != nil {
		return err
	}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Do not silently forget a journal if its linked account was removed after a partial write.
	rows, err = a.DB.Query("SELECT DISTINCT account_key FROM deletion_playlist_updates WHERE request_id=?", rid)
	if err != nil {
		return err
	}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, key := range keys {
		var c Credentials
		if err = a.unseal(key, &c); err != nil {
			return errors.New("A linked playlist account is unavailable; reconnect it before retrying deletion")
		}
		c.URL = server
		if seen[c.Username] {
			continue
		}
		seen[c.Username] = true
		response, err := a.nav(ctx, c, "getPlaylists", nil)
		if err != nil {
			return fmt.Errorf("Cannot check playlists for %s; no duplicate file removed: %w", c.Username, err)
		}
		for _, listed := range response.Playlists.Items {
			// Public visibility does not grant write access. Smart/imported lists are readonly.
			if listed.Readonly || !strings.EqualFold(listed.Owner, c.Username) {
				continue
			}
			current, err := a.nav(ctx, c, "getPlaylist", url.Values{"id": {listed.ID}})
			if err != nil {
				return fmt.Errorf("Cannot read playlist %s: %w", listed.Name, err)
			}
			playlist := current.Playlist
			if playlist.Readonly || !strings.EqualFold(playlist.Owner, c.Username) {
				continue
			}
			ids := playlistIDs(playlist)
			var beforeRaw, afterRaw, state string
			err = a.DB.QueryRow("SELECT before_ids,after_ids,state FROM deletion_playlist_updates WHERE request_id=? AND account_key=? AND playlist_id=?", rid, key, listed.ID).Scan(&beforeRaw, &afterRaw, &state)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if err == sql.ErrNoRows {
				if !slices.Contains(ids, source) {
					continue
				}
				after := slices.Clone(ids)
				for i, id := range after {
					if id == source {
						after[i] = keep
					}
				}
				beforeRaw, afterRaw = mustJSON(ids), mustJSON(after)
				_, err = a.DB.Exec("INSERT INTO deletion_playlist_updates(request_id,account_key,playlist_id,before_ids,after_ids) VALUES(?,?,?,?,?)", rid, key, listed.ID, beforeRaw, afterRaw)
				if err != nil {
					return err
				}
			} else if state == "completed" && !slices.Contains(ids, source) {
				continue
			}
			var before, after []string
			if json.Unmarshal([]byte(beforeRaw), &before) != nil || json.Unmarshal([]byte(afterRaw), &after) != nil {
				return errors.New("Playlist replacement journal is unreadable")
			}
			if !slices.Equal(ids, after) {
				if !slices.Equal(ids, before) {
					return fmt.Errorf("Playlist %s changed during replacement; review it before retrying deletion", listed.Name)
				}
				// Ensure this account can actually see the kept song before inserting it.
				visible, err := a.nav(ctx, c, "getSong", url.Values{"id": {keep}})
				if err != nil || visible.Song.ID != keep {
					return fmt.Errorf("Kept song is not accessible to playlist owner %s", c.Username)
				}
				// Recheck before writing; never knowingly overwrite a concurrent playlist edit.
				latest, err := a.nav(ctx, c, "getPlaylist", url.Values{"id": {listed.ID}})
				if err != nil || !slices.Equal(playlistIDs(latest.Playlist), before) {
					return fmt.Errorf("Playlist %s changed before replacement; no file removed", listed.Name)
				}
				params := url.Values{"playlistId": {listed.ID}, "songId": after}
				if _, err = a.nav(ctx, c, "createPlaylist", params); err != nil {
					return fmt.Errorf("Could not update playlist %s; no file removed: %w", listed.Name, err)
				}
				verified, err := a.nav(ctx, c, "getPlaylist", url.Values{"id": {listed.ID}})
				if err != nil || !slices.Equal(playlistIDs(verified.Playlist), after) {
					return fmt.Errorf("Could not verify playlist %s; no file removed", listed.Name)
				}
			}
			if _, err = a.DB.Exec("UPDATE deletion_playlist_updates SET state='completed' WHERE request_id=? AND account_key=? AND playlist_id=?", rid, key, listed.ID); err != nil {
				return err
			}
			slog.Info("Duplicate song playlist references replaced", "request_id", rid, "playlist_id", listed.ID, "source_song_id", source, "kept_song_id", keep)
		}
	}
	// A long playlist pass must not delete a source if its survivor disappeared meanwhile.
	kept, err = a.nav(ctx, shared, "getSong", url.Values{"id": {keep}, "realPath": {"true"}})
	if err != nil || kept.Song.ID != keep {
		return errors.New("Kept song disappeared during playlist updates; no duplicate file removed")
	}
	if rel, _, err = a.deletionPath(ctx, shared, kept.Song); err != nil || rel == sourcePath {
		return errors.New("Kept file changed or disappeared; no duplicate file removed")
	}
	return nil
}
