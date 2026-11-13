package app

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type libraryCollection struct {
	CoverArt string  `json:"coverArt,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Year     int     `json:"year,omitempty"`
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Artist   string  `json:"artist,omitempty"`
	Count    int     `json:"songCount"`
}

func (a *App) browseLibrary(w http.ResponseWriter, r *http.Request, u User) {
	c, e := a.shared()
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	q := r.URL.Query()
	if q.Get("view") == "playlists" {
		personal, err := a.personal(u.ID)
		if err != nil {
			fail(w, 400, "Link your personal Navidrome account in Settings to browse playlists")
			return
		}
		c = personal
	}
	search := strings.TrimSpace(q.Get("q"))
	view := q.Get("view")
	sid := q.Get("id")
	offset, _ := strconv.Atoi(q.Get("offset"))
	offset = max(0, offset)
	collections := []libraryCollection{}
	songs := []Song{}
	name := ""
	next := -1
	endpoint := ""
	params := url.Values{}
	switch view {
	case "playlists":
		if sid == "" {
			endpoint = "getPlaylists"
		} else {
			endpoint = "getPlaylist"
			params.Set("id", sid)
		}
	case "artists":
		if sid == "" {
			endpoint = "getArtists"
		} else {
			endpoint = "getArtist"
			params.Set("id", sid)
		}
	case "albums":
		if sid == "" {
			endpoint = "getAlbumList2"
			params = url.Values{"type": {"alphabeticalByName"}, "size": {"100"}, "offset": {strconv.Itoa(offset)}}
			if search != "" {
				endpoint = "search3"
				params = url.Values{"query": {search}, "albumCount": {"100"}, "albumOffset": {strconv.Itoa(offset)}, "songCount": {"0"}, "artistCount": {"0"}}
			}
		} else {
			endpoint = "getAlbum"
			params.Set("id", sid)
		}
	default:
		fail(w, 400, "Unknown library view")
		return
	}
	out, e := a.nav(r.Context(), c, endpoint, params)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	switch endpoint {
	case "getPlaylists":
		for _, p := range out.Playlists.Items {
			collections = append(collections, libraryCollection{ID: p.ID, Name: p.Name, Count: p.SongCount})
		}
	case "getPlaylist":
		songs = out.Playlist.Entries
		name = out.Playlist.Name
	case "getArtists":
		for _, index := range out.Artists.Index {
			for _, artist := range index.Artists {
				collections = append(collections, libraryCollection{ID: artist.ID, Name: artist.Name, Count: artist.AlbumCount, CoverArt: artist.CoverArt})
			}
		}
	case "getArtist":
		if q.Get("contents") == "songs" {
			offset = min(offset, len(out.Artist.Albums))
			end := offset + min(10, len(out.Artist.Albums)-offset)
			for _, album := range out.Artist.Albums[min(offset, len(out.Artist.Albums)):end] {
				details, err := a.nav(r.Context(), c, "getAlbum", mapValues("id", album.ID))
				if err != nil {
					fail(w, 502, err.Error())
					return
				}
				songs = append(songs, details.Album.Songs...)
			}
			if end < len(out.Artist.Albums) {
				next = end
			}
			name = out.Artist.Name
			break
		}
		name = out.Artist.Name
		for _, album := range out.Artist.Albums {
			collections = append(collections, libraryCollection{ID: album.ID, Name: album.Name, Artist: album.Artist, Count: album.SongCount, CoverArt: album.CoverArt, Year: album.Year, Duration: album.Duration})
		}
	case "getAlbum":
		songs = out.Album.Songs
		name = out.Album.Name
	case "search3":
		for _, album := range out.Search.Albums {
			collections = append(collections, libraryCollection{ID: album.ID, Name: album.Name, Artist: album.Artist, Count: album.SongCount, CoverArt: album.CoverArt, Year: album.Year, Duration: album.Duration})
		}
		if len(collections) == 100 {
			next = offset + 100
		}
	case "getAlbumList2":
		for _, album := range out.AlbumList.Albums {
			collections = append(collections, libraryCollection{ID: album.ID, Name: album.Name, Artist: album.Artist, Count: album.SongCount, CoverArt: album.CoverArt, Year: album.Year, Duration: album.Duration})
		}
		if len(collections) == 100 {
			next = offset + 100
		}
	}
	if endpoint != "getAlbumList2" {
		sort.SliceStable(collections, func(i, j int) bool {
			return strings.ToLower(collections[i].Name) < strings.ToLower(collections[j].Name)
		})
	}
	search = strings.ToLower(search)
	if search != "" && endpoint != "search3" {
		filtered := []libraryCollection{}
		for _, item := range collections {
			if strings.Contains(strings.ToLower(item.Name+" "+item.Artist), search) {
				filtered = append(filtered, item)
			}
		}
		collections = filtered
		filteredSongs := []Song{}
		for _, s := range songs {
			if strings.Contains(strings.ToLower(s.Title+" "+s.Artist+" "+s.Album), search) {
				filteredSongs = append(filteredSongs, s)
			}
		}
		songs = filteredSongs
	}
	if songs == nil {
		songs = []Song{}
	}
	// Filter removed entries from playlist/album responses until Navidrome's scan catches up.
	rows, e := a.DB.Query("SELECT song_id FROM song_deletions WHERE phase='removed'")
	if e != nil {
		a.internal(w, e)
		return
	}
	removed := map[string]bool{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			a.internal(w, e)
			return
		}
		removed[id] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		a.internal(w, e)
		return
	}
	visible := []Song{}
	for _, s := range songs {
		if !removed[s.ID] {
			visible = append(visible, s)
		}
	}
	send(w, 200, map[string]any{"collections": collections, "songs": visible, "name": name, "nextOffset": next})
}
