package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCatalogImportMatchesUnplaylistedSongsWithoutSelectingPlaylists(t *testing.T) {
	a, _, member := albumFixture(t)
	fixture := &navFixture{songs: map[string]Song{
		"first":   {ID: "first", Title: "First song", Artist: "Test artist", Album: "Test album", Duration: 2},
		"second":  {ID: "second", Title: "Second song", Artist: "Test artist", Album: "Test album", Duration: 2},
		"outside": {ID: "outside", Title: "Outside playlists", Artist: "Test artist", Album: "Test album", Duration: 2},
	}}
	nav := httptest.NewServer(http.HandlerFunc(fixture.handler))
	defer nav.Close()
	if err := a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../apple/testdata/library.xml")
	if err != nil {
		t.Fatal(err)
	}
	w := member.call("POST", "/api/v2/imports", string(data), "")
	expect(t, w, 201)
	var upload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	mid := upload["id"]
	expect(t, member.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[],"favorites":false}`, ""), 200)
	a.runMigrationTask(context.Background())
	sources, err := a.sources(mid)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 4 {
		t.Fatalf("lost catalog entries: %d", len(sources))
	}
	unplaylistedMatched := false
	for _, source := range sources {
		if !source.Metadata.Missing && source.ServerID == "" {
			t.Fatalf("catalog song was not matched: %+v", source)
		}
		if source.Metadata.Title == "Outside playlists" && source.ServerID == "outside" {
			unplaylistedMatched = true
		}
	}
	if !unplaylistedMatched {
		t.Fatal("song outside playlists was not included in matching")
	}
	playlists, err := a.playlists(mid)
	if err != nil {
		t.Fatal(err)
	}
	for _, playlist := range playlists {
		if playlist.Selected {
			t.Fatal("playlist was selected despite empty selection")
		}
	}
	var requests int
	if err := a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&requests); err != nil || requests != 0 {
		t.Fatalf("matching should not request downloads: %d, %v", requests, err)
	}
}
