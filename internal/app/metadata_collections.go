package app

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
)

func collectionSongEdit(s Song) MetadataEdit {
	return MetadataEdit{Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumArtist: s.AlbumArtist}
}
func renamedCredits(credits []ArtistCredit, id, name string) ([]string, bool) {
	changed := false
	names := []string{}
	seen := map[string]bool{}
	for _, credit := range credits {
		value := credit.Name
		if credit.ID == id {
			value = name
			changed = true
		}
		key := strings.ToLower(value)
		if !seen[key] {
			names = append(names, value)
			seen[key] = true
		}
	}
	return names, changed
}
func (a *App) metadataCollection(w http.ResponseWriter, r *http.Request, u User, kind, id string) {
	if r.Method != "GET" && r.Method != "PUT" {
		fail(w, 405, "Method not allowed")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	index, err := a.indexedNavSongs(r.Context(), c)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if index.limited {
		fail(w, 409, "Collection editing requires a complete library index")
		return
	}
	songs := []Song{}
	name := ""
	for _, s := range index.songs {
		if kind == "album" && s.AlbumID == id {
			songs = append(songs, s)
			name = s.Album
		}
		if kind == "artist" && creditNameForID(s, id) != "" {
			songs = append(songs, s)
			name = creditNameForID(s, id)
		}
	}
	if len(songs) == 0 {
		fail(w, 404, "Collection no longer exists; refresh the library")
		return
	}
	sort.Slice(songs, func(i, j int) bool { return songs[i].ID < songs[j].ID })
	first := songs[0]
	albumName := first.Album
	if kind == "album" {
		current, err := a.nav(r.Context(), c, "getAlbum", url.Values{"id": {id}})
		if err != nil || current.Album.ID != id {
			fail(w, 502, "Cannot read current album metadata")
			return
		}
		if current.Album.Name != "" {
			albumName = current.Album.Name
			name = albumName
		}
	}
	if r.Method == "GET" {
		edit := collectionSongEdit(first)
		edit.Album = albumName
		edit.ArtistNames = creditNames(songCredits(first, false))
		edit.AlbumArtistNames = creditNames(songCredits(first, true))
		send(w, 200, map[string]any{"name": name, "edit": edit, "duration": 0, "searchArtist": func() string {
			if first.AlbumArtist != "" {
				return first.AlbumArtist
			}
			return first.Artist
		}(), "songId": first.ID, "songs": len(songs)})
		return
	}
	var changes []metadataChange
	if kind == "artist" {
		var in struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 500 {
			fail(w, 400, "Enter an artist name of 1–500 characters")
			return
		}
		if in.Name == name {
			fail(w, 400, "Artist name is unchanged")
			return
		}
		for _, s := range songs {
			edit := collectionSongEdit(s)
			names, changed := renamedCredits(songCredits(s, false), id, in.Name)
			if changed {
				edit.ArtistNames = names
				edit.Artist = strings.Join(names, " / ")
			}
			albumCredits := songCredits(s, true)
			// Legacy albumArtist has no ID; rename only when it is the same primary identity.
			if len(s.AlbumArtists) == 0 && s.ArtistID == id && s.AlbumArtist == name {
				albumCredits = []ArtistCredit{{ID: id, Name: s.AlbumArtist}}
			}
			names, changed = renamedCredits(albumCredits, id, in.Name)
			if changed {
				edit.AlbumArtistNames = names
				edit.AlbumArtist = strings.Join(names, " / ")
			}
			changes = append(changes, metadataChange{Song: s, Edit: edit, CollectionKind: "artist", CollectionName: in.Name})
		}
	} else {
		var in MetadataEdit
		if !decodeLimit(w, r, &in, 1<<20) {
			return
		}
		in.Album = strings.TrimSpace(in.Album)
		if in.Album == "" || len(in.Album) > 500 || len(in.CoverURL) > 2000 {
			fail(w, 400, "Enter a valid album title and artwork URL")
			return
		}
		if err := validateMetadataArtistNames(in.AlbumArtistNames, false); err != nil {
			fail(w, 400, err.Error())
			return
		}
		selected := 0
		if in.CoverURL != "" {
			selected++
		}
		if in.CoverUploadID != "" {
			selected++
		}
		if in.RemoveCover {
			selected++
		}
		if selected > 1 {
			fail(w, 400, "Choose only one artwork action")
			return
		}
		if in.CoverURL != "" {
			parsed, err := url.Parse(in.CoverURL)
			if err != nil {
				fail(w, 400, "Use a public HTTPS artwork URL")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			err = validateArtworkURL(ctx, parsed)
			cancel()
			if err != nil {
				fail(w, 400, err.Error())
				return
			}
		}
		upload := ""
		if in.CoverUploadID != "" {
			upload, err = coverUploadRelative(u.ID, first.ID, in.CoverUploadID)
			if err == nil {
				_, err = a.readCoverUpload(upload)
			}
			if err != nil {
				fail(w, 400, "Album cover upload is unavailable; reopen the editor and upload again")
				return
			}
		}
		for _, s := range songs {
			edit := collectionSongEdit(s)
			if in.Album != albumName {
				edit.Album = in.Album
			}
			if in.AlbumArtistNames != nil && !slices.Equal(in.AlbumArtistNames, creditNames(songCredits(first, true))) {
				edit.AlbumArtistNames = in.AlbumArtistNames
				edit.AlbumArtist = strings.Join(in.AlbumArtistNames, " / ")
			}
			edit.CoverURL = in.CoverURL
			edit.RemoveCover = in.RemoveCover
			changes = append(changes, metadataChange{Song: s, Edit: edit, UploadedCover: upload, CollectionKind: "album", CollectionName: in.Album})
		}
	}
	jid, err := a.queueMetadata(changes, u)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	send(w, 202, map[string]any{"id": jid, "songs": len(changes)})
}
