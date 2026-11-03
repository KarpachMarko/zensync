package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
)

func TestReviewLibrarySearchUsesEnteredQueryAndPreservesPartialResults(t *testing.T) {
	a, _, member := albumFixture(t)
	const query = "новая песня & live"
	const libraryFailure = "library-down"
	local := Song{ID: "local", Title: "Found recording", Artist: "Artist", Album: "Album", Duration: 180}
	expectedLocalQuery := query
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		q := r.Form.Get("query")
		if q != expectedLocalQuery {
			t.Errorf("local query %q, want %q", q, expectedLocalQuery)
		}
		if r.Form.Get("songCount") != "100" || r.Form.Get("songOffset") != "0" {
			t.Error("review and library pagination differ")
		}
		if q == libraryFailure || q == "both-down" {
			http.Error(w, "Navidrome unavailable", 503)
			return
		}
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "searchResult3": map[string]any{"song": []Song{local}}}})
	}))
	defer nav.Close()
	if err := a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "deezer-down" || q == "both-down" {
			http.Error(w, "Deezer unavailable", 503)
			return
		}
		if r.URL.Path == "/track/42" {
			send(w, 200, makeTrack(42, "Direct recording"))
			return
		}
		send(w, 200, map[string]any{"data": []Track{makeTrack(42, "Deezer recording")}})
	}))
	defer provider.Close()
	a.deezerURL = provider.URL
	data, err := os.ReadFile("../apple/testdata/playlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	w := member.call("POST", "/api/v2/imports", string(data), "")
	expect(t, w, 201)
	var upload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	sources, err := a.sources(upload["id"])
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/v2/imports/" + upload["id"] + "/songs/" + sources[0].ID + "/candidates"
	w = member.call("GET", "/api/v2/library?q="+url.QueryEscape(query), "", "")
	expect(t, w, 200)
	var library []Song
	if err := json.Unmarshal(w.Body.Bytes(), &library); err != nil {
		t.Fatal(err)
	}
	type response struct {
		Library      []Song  `json:"library"`
		Deezer       []Track `json:"deezer"`
		LibraryError string  `json:"libraryError"`
		DeezerError  string  `json:"deezerError"`
	}
	search := func(q string) response {
		t.Helper()
		w := member.call("POST", endpoint, mustJSON(map[string]string{"query": q}), "")
		expect(t, w, 200)
		var result response
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Library == nil || result.Deezer == nil {
			t.Fatalf("result arrays must not be null: %s", w.Body.String())
		}
		return result
	}
	result := search(query)
	if !reflect.DeepEqual(result.Library, library) || result.LibraryError != "" || result.DeezerError != "" {
		t.Fatalf("different results for the same query: %+v vs %+v", result, library)
	}
	expectedLocalQuery = "deezer-down"
	result = search("deezer-down")
	if len(result.Library) != 1 || result.DeezerError == "" || result.LibraryError != "" {
		t.Fatalf("Deezer failure hid local matches: %+v", result)
	}
	expectedLocalQuery = libraryFailure
	result = search(libraryFailure)
	if len(result.Deezer) != 1 || result.LibraryError == "" || result.DeezerError != "" {
		t.Fatalf("library failure was hidden: %+v", result)
	}
	expectedLocalQuery = "both-down"
	result = search("both-down")
	if result.LibraryError == "" || result.DeezerError == "" {
		t.Fatalf("search failure reported as empty results: %+v", result)
	}
	expectedLocalQuery = sources[0].Metadata.Artist + " " + sources[0].Metadata.Title
	result = search("https://www.deezer.com/track/42")
	if len(result.Library) != 1 || len(result.Deezer) != 1 {
		t.Fatalf("direct URL lookup failed: %+v", result)
	}
}
