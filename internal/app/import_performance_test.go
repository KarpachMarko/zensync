package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func largeImportFixture(t *testing.T, c client) (string, []Source) {
	t.Helper()
	var xml strings.Builder
	xml.WriteString(`<plist><dict><key>Tracks</key><dict>`)
	for i := 1; i <= 3000; i++ {
		fmt.Fprintf(&xml, `<key>%d</key><dict><key>Name</key><string>Song %d</string><key>Artist</key><string>Artist</string><key>Album</key><string>Album</string><key>Total Time</key><integer>2000</integer></dict>`, i, i)
	}
	xml.WriteString(`</dict></dict></plist>`)
	response := c.call("POST", "/api/v2/imports", xml.String(), "")
	expect(t, response, 201)
	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	mid := result["id"]
	expect(t, c.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[]}`, ""), 200)
	sources, err := c.a.sources(mid)
	if err != nil {
		t.Fatal(err)
	}
	return mid, sources
}

func quickAction(t *testing.T, c client, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- c.call("POST", path, body, "") }()
	select {
	case result := <-done:
		return result
	case <-time.After(time.Second):
		t.Fatal("interactive action blocked behind background network I/O")
		return nil
	}
}

func TestLargeImportActionsDoNotWaitForMatcherAndPreserveManualDecisions(t *testing.T) {
	a, _, member := albumFixture(t)
	mid, sources := largeImportFixture(t, member)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	var unblock sync.Once
	stop := func() { unblock.Do(func() { close(release) }) }
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if first.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "searchResult3": map[string]any{"song": []Song{}}}})
	}))
	defer nav.Close()
	if err := a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		send(w, 200, map[string]any{"data": []Track{makeTrack(77, "Song 1")}})
	}))
	defer provider.Close()
	a.deezerURL = provider.URL
	go func() { defer close(finished); a.runMigrationTask(context.Background()) }()
	defer func() { stop(); <-finished }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("matcher did not start")
	}
	endpoint := "/api/v2/imports/" + mid + "/songs/" + sources[0].ID
	response := quickAction(t, member, endpoint+"/defer", `{}`)
	expect(t, response, 200)
	var result struct {
		Source Source `json:"source"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !result.Source.Deferred || result.Source.ID != sources[0].ID {
		t.Fatalf("missing single-row response: %s %v", response.Body.String(), err)
	}
	if response.Body.Len() > 2048 {
		t.Fatalf("action returned the whole import: %d bytes", response.Body.Len())
	}
	chosen := makeTrack(42, "My choice")
	a.rememberReviewTracks([]Track{chosen})
	expect(t, quickAction(t, member, endpoint+"/resolve", `{"target":"deezer:track:42"}`), 200)
	expect(t, quickAction(t, member, endpoint+"/acquire", `{}`), 201)
	if providerCalls.Load() != 0 {
		t.Fatal("review actions fetched provider metadata again")
	}
	expect(t, quickAction(t, member, "/api/v2/imports/"+mid+"/handover", `{}`), 200)
	stop()
	<-finished
	// The lookup that was already in flight cannot overwrite a manual selection.
	saved, err := a.sources(mid, sources[0].ID)
	if err != nil || len(saved) != 1 || saved[0].Candidate == nil || saved[0].Candidate.ID != 42 || saved[0].MatchState != "manual" {
		t.Fatalf("manual decision was overwritten: %+v %v", saved, err)
	}
}

func TestPublicationVerificationDoesNotBlockActionsOrWriteAfterHandover(t *testing.T) {
	a, _, member := albumFixture(t)
	mid, sources := largeImportFixture(t, member)
	var owner string
	a.DB.QueryRow("SELECT owner FROM migrations WHERE id=?", mid).Scan(&owner)
	a.DB.Exec("DELETE FROM migration_tasks WHERE migration_id=?", mid)
	a.DB.Exec("UPDATE migrations SET target_user='personal',favorites=1 WHERE id=?", mid)
	metadata := sources[0].Metadata
	metadata.Favorite = true
	a.DB.Exec("UPDATE source_songs SET metadata=?,server_id='available',favorite_state='pending' WHERE id=?", mustJSON(metadata), sources[0].ID)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var writes atomic.Int32
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "getSong") {
			close(started)
			<-release
			send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "song": Song{ID: "available"}}})
			return
		}
		writes.Add(1)
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok"}})
	}))
	defer nav.Close()
	a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "fixture"})
	a.seal("navidrome:"+owner, Credentials{Username: "personal", Password: "fixture"})
	a.queueMigration(owner, mid, "publish")
	go func() { defer close(finished); a.runMigrationTask(context.Background()) }()
	defer func() { unblock(); <-finished }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("publication did not start")
	}
	expect(t, quickAction(t, member, "/api/v2/imports/"+mid+"/songs/"+sources[1].ID+"/defer", `{}`), 200)
	expect(t, quickAction(t, member, "/api/v2/imports/"+mid+"/handover", `{}`), 200)
	unblock()
	<-finished
	if writes.Load() != 0 {
		t.Fatal("publication wrote after handover")
	}
}
