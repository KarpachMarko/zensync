package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestKeywordSearchIncludesAlbumsAndSongs(t *testing.T) {
	a, _, member := albumFixture(t)
	query := "Кубрик Джон Гарик"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != query {
			t.Errorf("query was not preserved: %q", r.URL.Query().Get("q"))
		}
		switch r.URL.Path {
		case "/search/track":
			send(w, 200, map[string]any{"data": []Track{makeTrack(1, "Song")}})
		case "/search/album":
			send(w, 200, map[string]any{"data": []map[string]any{{"id": 42, "title": "Album", "artist": map[string]string{"name": "Artist"}, "nb_tracks": 12}, {"id": 42, "title": "Duplicate", "artist": map[string]string{"name": "Artist"}}, {"id": 0, "title": "Invalid"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.deezerURL = server.URL
	w := member.call("GET", "/api/v2/search?q="+url.QueryEscape(query)+"&details=1", "", "")
	expect(t, w, 200)
	var result struct {
		Tracks   []Track        `json:"tracks"`
		Albums   []AlbumSummary `json:"albums"`
		Warnings []string       `json:"warnings"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if len(result.Tracks) != 1 || len(result.Albums) != 1 || result.Albums[0].ID != 42 || result.Albums[0].TrackCount != 12 || len(result.Warnings) != 0 {
		t.Fatalf("missing typed keyword results: %s", w.Body)
	}
	// Existing consumers without details continue receiving a track array.
	w = member.call("GET", "/api/v2/search?q="+url.QueryEscape(query), "", "")
	expect(t, w, 200)
	var tracks []Track
	if e := json.Unmarshal(w.Body.Bytes(), &tracks); e != nil || len(tracks) != 1 {
		t.Fatal("legacy search response changed", w.Body)
	}
}
func TestKeywordSearchPreservesPartialResults(t *testing.T) {
	a, _, member := albumFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "both-down" || (q == "albums-down" && r.URL.Path == "/search/album") || (q == "songs-down" && r.URL.Path == "/search/track") {
			send(w, 200, map[string]any{"error": map[string]int{"code": 4}})
			return
		}
		if r.URL.Path == "/search/album" {
			send(w, 200, map[string]any{"data": []map[string]any{{"id": 42, "title": "Still available", "artist": map[string]string{"name": "Artist"}}}})
		} else {
			send(w, 200, map[string]any{"data": []Track{makeTrack(1, "Still available")}})
		}
	}))
	defer server.Close()
	a.deezerURL = server.URL
	for _, q := range []string{"songs-down", "albums-down"} {
		w := member.call("GET", "/api/v2/search?q="+q+"&details=1", "", "")
		expect(t, w, 200)
		var result struct {
			Tracks   []Track        `json:"tracks"`
			Albums   []AlbumSummary `json:"albums"`
			Warnings []string       `json:"warnings"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		if len(result.Warnings) != 1 || len(result.Tracks)+len(result.Albums) != 1 || !strings.Contains(w.Body.String(), "Still available") {
			t.Fatal("partial results lost", w.Body)
		}
	}
	expect(t, member.call("GET", "/api/v2/search?q=both-down&details=1", "", ""), 502)
}

func TestKeywordSearchOmitsSingleTrackAlbums(t *testing.T) {
	a, _, member := albumFixture(t)
	single := makeTrack(1, "Single song")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/track":
			send(w, 200, map[string]any{"data": []Track{single}})
		case "/search/album":
			send(w, 200, map[string]any{"data": []map[string]any{
				{"id": 42, "title": "Album", "nb_tracks": 12},
				{"id": 43, "title": "Single release", "nb_tracks": 1},
				{"id": 44, "title": "Unknown track count"},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.deezerURL = server.URL
	w := member.call("GET", "/api/v2/search?q=single&details=1", "", "")
	expect(t, w, 200)
	var result struct {
		Albums []AlbumSummary `json:"albums"`
		Tracks []Track        `json:"tracks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Albums) != 2 || result.Albums[0].ID != 42 || result.Albums[1].ID != 44 || len(result.Tracks) != 1 || result.Tracks[0].ID != single.ID {
		t.Fatalf("single album should be omitted while its song remains: %s", w.Body)
	}
}
