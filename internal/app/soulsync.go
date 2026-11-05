package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func soulSend(w http.ResponseWriter, data any) {
	send(w, 200, map[string]any{"success": true, "data": data, "error": nil, "pagination": nil})
}
func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

// SoulSync profile 1 is the default profile *within the key owner's account*,
// never a lookup of ZenSync user 1. Other profiles cannot select another user.
func (a *App) soulSyncRoutes(w http.ResponseWriter, r *http.Request, u User, scopes []string) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	required := ""
	switch {
	case path == "/system/status" && r.Method == "GET":
		a.soulStatus(w)
		return
	case (path == "/search/tracks" || path == "/search/artists") && r.Method == "POST":
		required = "search:read"
	case (path == "/downloads" || path == "/wishlist") && r.Method == "GET":
		required = "requests:read"
	case (path == "/wishlist" || path == "/wishlist/process") && r.Method == "POST":
		required = "requests:create"
	case strings.HasPrefix(path, "/wishlist/") && r.Method == "DELETE":
		required = "requests:create"
	case path == "/library/recently-added" && r.Method == "GET":
		required = "library:read"
	case path == "/watchlist" || strings.HasPrefix(path, "/watchlist/"):
		fail(w, 501, "Artist following and watchlist scans are not available in ZenSync yet")
		return
	default:
		fail(w, 404, "SoulSync compatibility endpoint not found")
		return
	}
	if !hasScope(scopes, required) {
		fail(w, 403, "API key requires the "+required+" scope")
		return
	}
	switch {
	case strings.HasPrefix(path, "/search/"):
		a.soulSearch(w, r, path)
	case path == "/downloads":
		a.soulDownloads(w, r, u)
	case path == "/library/recently-added":
		a.soulRecent(w, r)
	case path == "/wishlist" && r.Method == "GET":
		a.soulWishlist(w, u)
	default:
		a.compatMu.Lock()
		defer a.compatMu.Unlock()
		switch {
		case path == "/wishlist" && r.Method == "POST":
			a.soulAddWishlist(w, r, u)
		case path == "/wishlist/process":
			a.soulProcessWishlist(w, r, u)
		default:
			target := strings.TrimPrefix(path, "/wishlist/")
			result, e := a.DB.Exec("DELETE FROM compatibility_wishlist WHERE owner=? AND target=?", u.ID, target)
			if e != nil {
				a.internal(w, e)
				return
			}
			count, _ := result.RowsAffected()
			if count == 0 {
				fail(w, 404, "Wishlist track not found")
				return
			}
			soulSend(w, map[string]string{"message": "Removed from wishlist; existing acquisition tasks are unchanged"})
		}
	}
}

func (a *App) soulStatus(w http.ResponseWriter) {
	_, binaryErr := exec.LookPath(a.dfiPath)
	music, musicErr := os.Stat(a.musicDir)
	_, navErr := a.shared()
	provider := deezerProvider{a}
	ready := binaryErr == nil && musicErr == nil && music.IsDir() && navErr == nil && provider.Ready() == nil
	seconds := int64(time.Since(a.started).Seconds())
	soulSend(w, map[string]any{
		"uptime": fmt.Sprintf("%ds", seconds), "uptime_seconds": seconds,
		"services":    map[string]bool{"spotify": false, "soulseek": ready, "hydrabase": false, "deezer": ready},
		"application": "ZenSync", "profile_id": 1,
		"capabilities": map[string]bool{"search": true, "wishlist": true, "downloads": true, "watchlist": false},
	})
}

type soulTrack struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Artists    []string `json:"artists"`
	Album      string   `json:"album"`
	DurationMS int64    `json:"duration_ms"`
	Popularity int      `json:"popularity"`
	PreviewURL string   `json:"preview_url"`
	ImageURL   string   `json:"image_url"`
}

func (a *App) soulSearch(w http.ResponseWriter, r *http.Request, path string) {
	var in struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Source string `json:"source"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" || len(in.Query) > 500 {
		fail(w, 400, "Enter a search query of 1 to 500 characters")
		return
	}
	if in.Source != "" && in.Source != "auto" && in.Source != "deezer" {
		fail(w, 400, "This integration supports the Deezer search source")
		return
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 100 {
		in.Limit = 100
	}
	if path == "/search/artists" {
		type artist struct {
			ID      json.Number `json:"id"`
			Name    string      `json:"name"`
			Picture string      `json:"picture_medium"`
			Fans    int         `json:"nb_fan"`
		}
		var result struct {
			Data []artist `json:"data"`
		}
		if e := a.deezer(r.Context(), "/search/artist?limit="+strconv.Itoa(in.Limit)+"&q="+url.QueryEscape(in.Query), &result); e != nil {
			fail(w, 502, e.Error())
			return
		}
		artists := []map[string]any{}
		for _, item := range result.Data {
			artists = append(artists, map[string]any{"id": item.ID.String(), "name": item.Name, "popularity": 0, "genres": []string{}, "followers": item.Fans, "image_url": item.Picture})
		}
		soulSend(w, map[string]any{"artists": artists, "source": "deezer"})
		return
	}
	var result struct {
		Data []Track `json:"data"`
	}
	if e := a.deezer(r.Context(), "/search/track?limit="+strconv.Itoa(in.Limit)+"&q="+url.QueryEscape(in.Query), &result); e != nil {
		fail(w, 502, e.Error())
		return
	}
	tracks := []soulTrack{}
	for _, track := range result.Data {
		tracks = append(tracks, soulTrack{ID: trackTarget(track), Name: trackLabel(track), Artists: []string{track.Artist.Name}, Album: track.Album.Title, DurationMS: int64(track.Duration * 1000), PreviewURL: track.Preview, ImageURL: track.Album.Cover})
	}
	soulSend(w, map[string]any{"tracks": tracks, "source": "deezer"})
}

type soulTrackData struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Artists []string `json:"artists"`
	Album   struct {
		Name   string `json:"name"`
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	} `json:"album"`
	DurationMS int64 `json:"duration_ms"`
	Popularity int   `json:"popularity"`
}

func (a *App) soulAddWishlist(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		TrackData     soulTrackData `json:"track_data"`
		SourceType    string        `json:"source_type"`
		FailureReason string        `json:"failure_reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !targetPattern.MatchString(in.TrackData.ID) || !strings.HasPrefix(in.TrackData.ID, "deezer:") {
		fail(w, 400, "Use the track ID returned by ZenSync search")
		return
	}
	// Do not trust client metadata to choose a download or bypass native validation.
	track, e := a.track(r.Context(), in.TrackData.ID)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	if track.ID <= 0 {
		fail(w, 400, "Invalid recording")
		return
	}
	data, _ := json.Marshal(in.TrackData)
	_, e = a.DB.Exec(`INSERT INTO compatibility_wishlist(owner,target,track_data) VALUES(?,?,?)
 ON CONFLICT(owner,target) DO UPDATE SET track_data=excluded.track_data,
 request_id=CASE WHEN EXISTS(SELECT 1 FROM jobs WHERE request_id=compatibility_wishlist.request_id AND state IN ('completed','available')) THEN NULL ELSE compatibility_wishlist.request_id END`, u.ID, in.TrackData.ID, string(data))
	if e != nil {
		a.internal(w, e)
		return
	}
	soulSend(w, map[string]string{"message": "Track added to wishlist"})
}

func (a *App) soulWishlist(w http.ResponseWriter, u User) {
	rows, e := a.DB.Query(`SELECT c.id,c.target,c.track_data,COALESCE(r.reason,''),COALESCE(j.state,''),COALESCE(j.stage,'')
 FROM compatibility_wishlist c LEFT JOIN requests r ON r.id=c.request_id LEFT JOIN jobs j ON j.request_id=r.id
 WHERE c.owner=? AND COALESCE(j.state,'') NOT IN ('completed','available') ORDER BY c.id`, u.ID)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	tracks := []map[string]any{}
	for rows.Next() {
		var key int64
		var target, data, reason, state, stage string
		if e = rows.Scan(&key, &target, &data, &reason, &state, &stage); e != nil {
			a.internal(w, e)
			return
		}
		var track soulTrackData
		if e = json.Unmarshal([]byte(data), &track); e != nil {
			a.internal(w, e)
			return
		}
		failure := ""
		if state == "failed" || state == "blocked" {
			failure = stage
		}
		tracks = append(tracks, map[string]any{"id": key, "spotify_track_id": target, "track_name": track.Name, "artist_name": strings.Join(track.Artists, ", "), "album_name": track.Album.Name, "track_data": track, "retry_count": 0, "last_attempted": nil, "failure_reason": failure})
	}
	if e = rows.Err(); e != nil {
		a.internal(w, e)
		return
	}
	soulSend(w, map[string]any{"tracks": tracks})
}

func (a *App) soulProcessWishlist(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		TrackIDs []string `json:"track_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.TrackIDs) == 0 || len(in.TrackIDs) > 50 {
		fail(w, 400, "Select between 1 and 50 wishlist track IDs")
		return
	}
	type pending struct {
		id    int64
		data  string
		track Track
	}
	entries := []pending{}
	seen := map[string]bool{}
	// Validate the whole selection before creating any requests.
	for _, target := range in.TrackIDs {
		if seen[target] {
			continue
		}
		seen[target] = true
		var key int64
		var data string
		var request sql.NullString
		e := a.DB.QueryRow("SELECT id,request_id,track_data FROM compatibility_wishlist WHERE owner=? AND target=?", u.ID, target).Scan(&key, &request, &data)
		if e == sql.ErrNoRows {
			fail(w, 404, "Wishlist track not found")
			return
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		if request.Valid {
			continue
		}
		track, e := a.track(r.Context(), target)
		if e != nil {
			fail(w, 502, e.Error())
			return
		}
		entries = append(entries, pending{key, data, track})
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	for _, entry := range entries {
		request, e := insertTrackRequest(tx, u, "soulsync:"+id(), "", entry.track, false)
		if e == nil {
			_, e = tx.Exec("INSERT INTO compatibility_downloads(request_id,owner,track_data) VALUES(?,?,?)", request, u.ID, entry.data)
		}
		if e == nil {
			_, e = tx.Exec("UPDATE compatibility_wishlist SET request_id=? WHERE id=? AND owner=?", request, entry.id, u.ID)
		}
		if e != nil {
			a.internal(w, e)
			return
		}
	}
	if e = tx.Commit(); e != nil {
		a.internal(w, e)
		return
	}
	soulSend(w, map[string]string{"message": "Selected tracks submitted; account approval rules apply"})
}

func boundedInt(value string, fallback, maximum int) int {
	n, e := strconv.Atoi(value)
	if e != nil || n <= 0 {
		return fallback
	}
	if n > maximum {
		return maximum
	}
	return n
}
func (a *App) soulDownloads(w http.ResponseWriter, r *http.Request, u User) {
	limit := boundedInt(r.URL.Query().Get("limit"), 200, 200)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	rows, e := a.DB.Query(`SELECT c.track_data,r.id,r.decision,r.reason,r.created,COALESCE(j.id,''),COALESCE(j.state,''),COALESCE(j.stage,'')
 FROM compatibility_downloads c JOIN requests r ON r.id=c.request_id LEFT JOIN jobs j ON j.request_id=r.id
 WHERE c.owner=? ORDER BY r.created DESC,r.rowid DESC LIMIT ? OFFSET ?`, u.ID, limit, offset)
	if e != nil {
		a.internal(w, e)
		return
	}
	downloads := []map[string]any{}
	for rows.Next() {
		var data, rid, decision, reason, created, jid, state, stage string
		if e = rows.Scan(&data, &rid, &decision, &reason, &created, &jid, &state, &stage); e != nil {
			rows.Close()
			a.internal(w, e)
			return
		}
		var track soulTrackData
		if e = json.Unmarshal([]byte(data), &track); e != nil {
			rows.Close()
			a.internal(w, e)
			return
		}
		status := "pending"
		progress := float64(0)
		var failure any
		switch {
		case decision == "declined" || decision == "withdrawn":
			status = "cancelled"
			failure = reason
		case decision == "awaiting_approval":
			status = "pending"
		case state == "completed" || state == "available":
			status = "completed"
			progress = 100
		case state == "failed" || state == "blocked" || state == "deferred":
			status = "failed"
			failure = stage
		case state == "cancelled" || state == "superseded":
			status = "cancelled"
		case state == "downloading":
			status = "downloading"
		case state == "validating" || state == "importing" || state == "scanning" || state == "awaiting_scan":
			status = "post_processing"
		case state == "checking":
			status = "searching"
		default:
			status = "queued"
		}
		var size any
		if transfer := a.transfer(jid, state); transfer != nil {
			size = transfer.TotalBytes
			if transfer.Percent != nil {
				progress = *transfer.Percent
				if progress > 99 {
					progress = 99
				}
				if progress < 0 {
					progress = 0
				}
			}
		}
		downloads = append(downloads, map[string]any{"id": rid, "status": status, "track_name": track.Name, "artist_name": strings.Join(track.Artists, ", "), "album_name": track.Album.Name, "username": nil, "filename": nil, "progress": progress, "size": size, "error": failure, "batch_id": rid, "track_index": nil, "retry_count": 0, "status_change_time": created})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		a.internal(w, e)
		return
	}
	var total int
	if e = a.DB.QueryRow("SELECT count(*) FROM compatibility_downloads WHERE owner=?", u.ID).Scan(&total); e != nil {
		a.internal(w, e)
		return
	}
	soulSend(w, map[string]any{"downloads": downloads, "total": total, "limit": limit, "offset": offset})
}

func (a *App) soulRecent(w http.ResponseWriter, r *http.Request) {
	if kind := r.URL.Query().Get("type"); kind != "" && kind != "albums" {
		fail(w, 400, "Only recently added albums are supported")
		return
	}
	c, e := a.shared()
	if e != nil {
		fail(w, 503, e.Error())
		return
	}
	limit := boundedInt(r.URL.Query().Get("limit"), 20, 100)
	result, e := a.nav(r.Context(), c, "getAlbumList2", url.Values{"type": {"newest"}, "size": {strconv.Itoa(limit)}})
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	items := []map[string]any{}
	for _, album := range result.AlbumList.Albums {
		thumb := ""
		if album.CoverArt != "" {
			thumb = "/rest/getCoverArt?id=" + url.QueryEscape(album.CoverArt)
		}
		if _, e = a.DB.Exec("INSERT OR IGNORE INTO compatibility_albums(server_id) VALUES(?)", album.ID); e != nil {
			a.internal(w, e)
			return
		}
		var localID int64
		if e = a.DB.QueryRow("SELECT id FROM compatibility_albums WHERE server_id=?", album.ID).Scan(&localID); e != nil {
			a.internal(w, e)
			return
		}
		items = append(items, map[string]any{"id": localID, "title": album.Name, "year": album.Year, "thumb_url": thumb, "server_source": "navidrome", "created_at": album.Created})
	}
	soulSend(w, map[string]any{"items": items, "type": "albums"})
}
