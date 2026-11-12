package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAlbumFilterSearchesLibraryAndPaginatesMatches(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if !strings.Contains(r.URL.Path, "search3") {
			t.Errorf("Album filter used page browsing: %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
			return
		}
		if r.Form.Get("albumCount") != "100" || r.Form.Get("songCount") != "0" || r.Form.Get("artistCount") != "0" {
			t.Error("Wrong search scope", r.Form)
		}
		albums := []navAlbum{{ID: "slaang-aarne", Name: "SLAANG", Artist: "Aarne", SongCount: 4}, {ID: "slaang-toxi", Name: "SLAANG", Artist: "Aarne / Toxi$", SongCount: 2}}
		if r.Form.Get("query") == "many" {
			albums = nil
			count := 100
			if r.Form.Get("albumOffset") == "100" {
				count = 1
			}
			for i := 0; i < count; i++ {
				albums = append(albums, navAlbum{ID: fmt.Sprintf("%s-%d", r.Form.Get("albumOffset"), i), Name: "Matching album"})
			}
		} else if r.Form.Get("query") != "Slaang" {
			t.Error("Search text changed", r.Form)
		}
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "searchResult3": map[string]any{"album": albums}}})
	}))
	defer nav.Close()
	if err := a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "password"}); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		query               string
		offset, count, next int
	}{{"Slaang", 0, 2, -1}, {"many", 0, 100, 100}, {"many", 100, 1, -1}} {
		response := admin.call("GET", fmt.Sprintf("/api/v2/library/browse?view=albums&q=%s&offset=%d", url.QueryEscape(check.query), check.offset), "", "")
		expect(t, response, 200)
		var result struct {
			Collections []libraryCollection
			NextOffset  int
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Collections) != check.count || result.NextOffset != check.next {
			t.Fatalf("Wrong search page: %s", response.Body)
		}
		if check.query == "Slaang" && (result.Collections[0].Name != "SLAANG" || result.Collections[1].Name != "SLAANG") {
			t.Fatal("Matching albums were lost", result)
		}
	}
}
