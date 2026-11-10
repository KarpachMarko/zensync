package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Metadata edits are a review layer, independent of acquisition and file identity.
type MetadataEdit struct {
	AlbumArtist  string `json:"albumArtist,omitempty"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	CoverURL     string `json:"coverUrl"`
	Lyrics       string `json:"lyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
	LyricsSource string `json:"lyricsSource"`
}
type duplicateArtist struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AlbumArtist string `json:"albumArtist"`
}
type duplicateMember struct {
	Artists []duplicateArtist `json:"artists,omitempty"`
	ID      string            `json:"id"`
	Label   string            `json:"label"`
	Song    *Song             `json:"song,omitempty"`
	Count   int               `json:"count,omitempty"`
	Tracks  []Song            `json:"tracks,omitempty"`
}
type duplicateGroup struct {
	Kind    string            `json:"kind"`
	Reason  string            `json:"reason"`
	Members []duplicateMember `json:"members"`
}

func metadataKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func metadataDuplicates(songs []Song) []duplicateGroup {
	buckets := map[string]map[string]duplicateMember{}
	add := func(kind, key, id, label string) {
		if key == "" || id == "" {
			return
		}
		k := kind + ":" + key
		if buckets[k] == nil {
			buckets[k] = map[string]duplicateMember{}
		}
		buckets[k][id] = duplicateMember{ID: id, Label: label}
	}
	tracks := map[string][]Song{}
	counts := map[string]int{}
	songByID := map[string]Song{}
	for _, s := range songs {
		counts["artist:"+s.ArtistID]++
		counts["album:"+s.AlbumID]++
		counts["song:"+s.ID]++
		copy := s
		copy.Path = ""
		songByID[s.ID] = copy
		add("artist", metadataKey(s.Artist), s.ArtistID, s.Artist)
		if metadataKey(s.Artist) != "" && metadataKey(s.Album) != "" {
			add("album", metadataKey(s.Artist)+":"+metadataKey(s.Album), s.AlbumID, s.Artist+" — "+s.Album)
		}
		if metadataKey(s.Artist) != "" && metadataKey(s.Title) != "" {
			k := metadataKey(s.Artist) + ":" + metadataKey(s.Title)
			tracks[k] = append(tracks[k], s)
		}
	}
	// Duration bins alone miss recordings straddling a bin boundary. Compare sorted neighbors.
	for k, ss := range tracks {
		sort.Slice(ss, func(i, j int) bool { return ss[i].Duration < ss[j].Duration })
		for start := 0; start < len(ss); {
			end := start + 1
			for end < len(ss) && ss[start].Duration > 0 && math.Abs(ss[end].Duration-ss[start].Duration) <= 2 {
				end++
			}
			for _, s := range ss[start:end] {
				add("song", k+":"+strconv.Itoa(start), s.ID, s.Artist+" — "+s.Title+" · "+s.Album)
			}
			start = end
		}
	}
	keys := []string{}
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	groups := []duplicateGroup{}
	for _, k := range keys {
		b := buckets[k]
		if len(b) < 2 {
			continue
		}
		kind := strings.SplitN(k, ":", 2)[0]
		members := []duplicateMember{}
		for _, m := range b {
			members = append(members, m)
		}
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		reason := "Matching normalized names with distinct library IDs"
		if kind == "song" {
			reason = "Matching artist and title, duration within two seconds; editions may differ"
		}
		for i := range members {
			members[i].Count = counts[kind+":"+members[i].ID]
			if kind == "album" {
				for _, song := range songs {
					if song.AlbumID == members[i].ID {
						clean := song
						clean.Path = ""
						members[i].Tracks = append(members[i].Tracks, clean)
						artist := duplicateArtist{song.ArtistID, song.Artist, song.AlbumArtist}
						found := false
						for _, existing := range members[i].Artists {
							if existing == artist {
								found = true
							}
						}
						if !found {
							members[i].Artists = append(members[i].Artists, artist)
						}
					}
				}
			}
			if kind == "song" {
				copy := songByID[members[i].ID]
				members[i].Song = &copy
			}
		}

		groups = append(groups, duplicateGroup{kind, reason, members})
	}
	return groups
}

func (a *App) metadataRoutes(w http.ResponseWriter, r *http.Request, u User) {
	if u.Role != "admin" {
		fail(w, 403, "Metadata management requires an administrator")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/metadata")
	if path == "/merge" && r.Method == "POST" {
		a.mergeMetadata(w, r, u)
		return
	}
	if path == "/jobs" {
		a.metadataJobRoutes(w, r)
		return
	}
	if path == "/suggestions" && r.Method == "GET" {
		a.metadataSuggestions(w, r)
		return
	}

	if path == "/duplicates" && r.Method == "GET" {
		c, e := a.shared()
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		index, e := a.indexedNavSongs(r.Context(), c)
		if e != nil {
			fail(w, 502, e.Error())
			return
		}
		send(w, 200, map[string]any{"groups": metadataDuplicates(index.songs), "scanned": len(index.songs), "limited": index.limited})
		return
	}
	if (path == "/lyrics" || path == "/covers") && r.Method == "GET" {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" || len(q) > 500 {
			fail(w, 400, "Enter a search of 1–500 characters")
			return
		}
		if path == "/lyrics" {
			a.metadataLyrics(w, r, q)
		} else {
			a.metadataCovers(w, r, q)
		}
		return
	}
	if strings.HasPrefix(path, "/songs/") && strings.HasSuffix(path, "/cover") && r.Method == "GET" {
		sid := strings.TrimSuffix(strings.TrimPrefix(path, "/songs/"), "/cover")
		c, err := a.shared()
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		res, err := a.navRaw(r.Context(), c, "getCoverArt", mapValues("id", sid))
		if err != nil {
			fail(w, 502, "Could not load current cover")
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			fail(w, 404, "No current cover")
			return
		}
		w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		io.Copy(w, io.LimitReader(res.Body, 20<<20))
		return
	}
	if strings.HasPrefix(path, "/songs/") && (r.Method == "GET" || r.Method == "PUT" || r.Method == "DELETE") {
		sid := strings.TrimPrefix(path, "/songs/")
		if sid == "" || len(sid) > 500 {
			fail(w, 400, "Invalid song ID")
			return
		}
		if r.Method == "DELETE" {
			var busy int
			if e := a.DB.QueryRow("SELECT count(*) FROM metadata_jobs WHERE state IN ('queued','running')").Scan(&busy); e != nil {
				a.internal(w, e)
				return
			}
			if busy > 0 {
				fail(w, 409, "Wait for the pending tag operation before discarding its review record")
				return
			}
			if _, e := a.DB.Exec("DELETE FROM metadata_edits WHERE song_id=?", sid); e != nil {
				a.internal(w, e)
				return
			}
			send(w, 200, map[string]bool{"ok": true})
			return
		}
		c, e := a.shared()
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		remote, e := a.nav(r.Context(), c, "getSong", mapValues("id", sid))
		if e != nil {
			fail(w, 502, e.Error())
			return
		}
		if remote.Song.ID != sid {
			fail(w, 404, "Song no longer exists")
			return
		}
		if r.Method == "GET" {
			edit := MetadataEdit{Title: remote.Song.Title, Artist: remote.Song.Artist, Album: remote.Song.Album, AlbumArtist: remote.Song.AlbumArtist}
			var raw string
			e = a.DB.QueryRow("SELECT metadata FROM metadata_edits WHERE song_id=?", sid).Scan(&raw)
			saved := e == nil
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				a.internal(w, e)
				return
			}
			if saved {
				if e = json.Unmarshal([]byte(raw), &edit); e != nil {
					a.internal(w, e)
					return
				}
			}
			send(w, 200, map[string]any{"edit": edit, "saved": saved, "duration": remote.Song.Duration})
			return
		}
		var edit MetadataEdit
		if !decodeLimit(w, r, &edit, 1<<20) {
			return
		}
		edit.Title = strings.TrimSpace(edit.Title)
		edit.Artist = strings.TrimSpace(edit.Artist)
		edit.Album = strings.TrimSpace(edit.Album)
		if edit.Title == "" || edit.Artist == "" || len(edit.Title) > 500 || len(edit.Artist) > 500 || len(edit.Album) > 500 || len(edit.AlbumArtist) > 500 || len(edit.Lyrics) > 200000 || len(edit.SyncedLyrics) > 200000 || len(edit.LyricsSource) > 500 || len(edit.CoverURL) > 2000 {
			fail(w, 400, "Invalid metadata or lyrics too long")
			return
		}
		if edit.CoverURL != "" {
			v, e := url.Parse(edit.CoverURL)
			if e != nil || v.Scheme != "https" || v.Host == "" || v.User != nil {
				fail(w, 400, "Cover must be an HTTPS URL")
				return
			}
		}
		jid, e := a.queueMetadata([]metadataChange{{Song: remote.Song, Edit: edit}}, u)
		if e != nil {
			fail(w, 409, e.Error())
			return
		}
		send(w, 202, map[string]string{"id": jid})
		return
	}
	fail(w, 404, "Endpoint unavailable")
}

// A single paced queue prevents parallel searches from overrunning public services.
// The cooldown also applies to subsequent requests after an upstream 429/503.
func (a *App) metadataJSON(ctx context.Context, endpoint string, out any) error {
	a.metadataMu.Lock()
	defer a.metadataMu.Unlock()
	if wait := time.Until(a.metadataNext); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	a.metadataNext = time.Now().Add(time.Second)
	req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if e != nil {
		return e
	}
	req.Header.Set("User-Agent", "ZenSync/0.1 (https://github.com/KarpachMarko/zensync)")
	req.Header.Set("Accept", "application/json")
	client := *a.HTTP
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		host := next.URL.Hostname()
		allowed := host == "musicbrainz.org" || host == "coverartarchive.org" || host == "lrclib.net" || host == "archive.org" || strings.HasSuffix(host, ".archive.org")
		if len(via) >= 5 || next.URL.Scheme != "https" || next.URL.User != nil || (next.URL.Port() != "" && next.URL.Port() != "443") || !allowed {
			return errors.New("Metadata provider returned an unsafe redirect")
		}
		return nil
	}
	res, e := client.Do(req)
	if e != nil {
		return errors.New("Metadata provider is unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode == 429 || res.StatusCode == 503 {
		delay := time.Minute
		if seconds, e := strconv.Atoi(res.Header.Get("Retry-After")); e == nil && seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		} else if date, e := http.ParseTime(res.Header.Get("Retry-After")); e == nil && time.Until(date) > 0 {
			delay = time.Until(date)
		}
		a.metadataNext = time.Now().Add(delay)
		return errors.New("Metadata provider is busy; try again later")
	}
	if res.StatusCode == 404 {
		return sql.ErrNoRows
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("Metadata provider returned HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
}

type lyricCandidate struct {
	ID           int64   `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func (a *App) metadataLyrics(w http.ResponseWriter, r *http.Request, q string) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	rows := []lyricCandidate{}
	if e := a.metadataJSON(ctx, "https://lrclib.net/api/search?"+url.Values{"q": {q}}.Encode(), &rows); e != nil {
		fail(w, 502, e.Error())
		return
	}
	if len(rows) > 20 {
		rows = rows[:20]
	}
	send(w, 200, rows)
}

type coverCandidate struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	URL    string `json:"url"`
	Source string `json:"source"`
}

func (a *App) metadataCovers(w http.ResponseWriter, r *http.Request, q string) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	var search struct {
		Releases []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Credits []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
		} `json:"releases"`
	}
	if e := a.metadataJSON(ctx, "https://musicbrainz.org/ws/2/release/?"+url.Values{"query": {q}, "fmt": {"json"}, "limit": {"5"}}.Encode(), &search); e != nil {
		fail(w, 502, e.Error())
		return
	}
	rows := []coverCandidate{}
	for _, release := range search.Releases {
		// IDs from upstream are never allowed to change the destination host or path.
		if len(release.ID) != 36 || strings.ContainsAny(release.ID, "/?#%") {
			continue
		}
		var art struct {
			Images []struct {
				Front      bool              `json:"front"`
				Thumbnails map[string]string `json:"thumbnails"`
			} `json:"images"`
		}
		e := a.metadataJSON(ctx, "https://coverartarchive.org/release/"+release.ID, &art)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			fail(w, 502, e.Error())
			return
		}
		for _, im := range art.Images {
			if !im.Front {
				continue
			}
			image := im.Thumbnails["500"]
			if image == "" {
				image = im.Thumbnails["large"]
			}
			if !strings.HasPrefix(image, "https://") {
				continue
			}
			names := []string{}
			for _, credit := range release.Credits {
				names = append(names, credit.Name)
			}
			rows = append(rows, coverCandidate{release.ID, release.Title, strings.Join(names, ", "), image, "https://musicbrainz.org/release/" + release.ID})
			break
		}
	}
	send(w, 200, rows)
}

func (a *App) metadataSuggestions(w http.ResponseWriter, r *http.Request) {
	c, e := a.shared()
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	index, e := a.indexedNavSongs(r.Context(), c)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	field := r.URL.Query().Get("field")
	if field != "artist" && field != "album" {
		fail(w, 400, "Choose artist or album suggestions")
		return
	}
	q := strings.ToLower(r.URL.Query().Get("q"))
	seen := map[string]bool{}
	values := []string{}
	for _, song := range index.songs {
		value := song.Artist
		if field == "album" {
			value = song.Album
		}
		if value != "" && !seen[value] && strings.Contains(strings.ToLower(value), q) {
			seen[value] = true
			values = append(values, value)
		}
	}
	sort.Strings(values)
	if len(values) > 20 {
		values = values[:20]
	}
	send(w, 200, map[string]any{"values": values, "limited": index.limited})
}
