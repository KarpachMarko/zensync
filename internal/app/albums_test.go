package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func albumFixture(t *testing.T) (*App, client, client) {
	t.Helper()
	a, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.DB.Close() })
	a.createAdmin("admin", "test-password-long")
	admin := login(t, a, "admin")
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long","direct":false}`, ""), 201)
	member := login(t, a, "member")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/album/42":
			send(w, 200, map[string]any{"id": 42, "title": "The album", "artist": map[string]string{"name": "Test artist"}, "tracks": map[string]any{"data": []Track{}}})
		case r.URL.Path == "/album/42/tracks":
			send(w, 200, map[string]any{"data": []Track{makeTrack(1, "First song"), makeTrack(2, "Second song")}, "total": 2})
		case strings.HasPrefix(r.URL.Path, "/track/"):
			var id int
			fmt.Sscanf(r.URL.Path, "/track/%d", &id)
			title := "First song"
			if id == 2 {
				title = "Second song"
			}
			track := makeTrack(id, title)
			track.Album.Title = "The album"
			send(w, 200, track)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	a.deezerURL = server.URL
	a.metadataInterval = 0
	return a, admin, member
}
func TestAlbumSingleApprovalAndProgress(t *testing.T) {
	a, admin, member := albumFixture(t)
	w := member.call("POST", "/api/v2/requests", `{"target":"deezer:album:42"}`, "album-request-key")
	expect(t, w, 201)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	parent := result["id"]
	expect(t, member.call("POST", "/api/v2/requests", `{"target":"deezer:album:42"}`, "album-request-key"), 200)
	var count int
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&count)
	if count != 0 {
		t.Fatal("album created jobs before approval")
	}
	a.runAcquisition(context.Background())
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&count)
	if count != 0 {
		t.Fatal("member acquired without approval")
	}
	var requests []Request
	w = admin.call("GET", "/api/v2/requests", "", "")
	expect(t, w, 200)
	json.Unmarshal(w.Body.Bytes(), &requests)
	if len(requests) != 1 || requests[0].Kind != "album" || requests[0].TrackCount != 2 || len(requests[0].Items) != 2 {
		t.Fatalf("not one complete album request: %s", w.Body)
	}
	expect(t, member.call("PUT", "/api/v2/quality", `{"quality":["128"]}`, ""), 200)
	// Concurrent approvals must atomically create one set of execution children.
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- admin.call("POST", "/api/v2/requests/"+parent, `{"decision":"approved","reason":"Approved album"}`, "").Code
		}()
	}
	wg.Wait()
	close(codes)
	status := map[int]int{}
	for c := range codes {
		status[c]++
	}
	if status[200] != 1 || status[409] != 1 {
		t.Fatal(status)
	}
	a.DB.QueryRow("SELECT count(*) FROM request_children WHERE parent_id=?", parent).Scan(&count)
	if count != 2 {
		t.Fatal("duplicate/missing children")
	}
	var wrong int
	a.DB.QueryRow("SELECT count(*) FROM request_children c JOIN requests r ON r.id=c.child_id WHERE c.parent_id=? AND r.quality<>?", parent, `["lossless","320","128"]`).Scan(&wrong)
	if wrong != 0 {
		t.Fatal("album quality snapshot changed")
	}
	w = admin.call("GET", "/api/v2/requests", "", "")
	json.Unmarshal(w.Body.Bytes(), &requests)
	if len(requests) != 1 {
		t.Fatalf("per-track approval rows leaked: %s", w.Body)
	}
	var child, parentJob string
	a.DB.QueryRow("SELECT child_id FROM request_children WHERE parent_id=? AND position=0", parent).Scan(&child)
	a.DB.QueryRow("SELECT id FROM jobs WHERE request_id=?", parent).Scan(&parentJob)
	expect(t, admin.call("POST", "/api/v2/requests/"+child, `{"decision":"approved","reason":""}`, ""), 409)
	a.DB.Exec("UPDATE jobs SET state='available' WHERE request_id=?", child)
	a.DB.Exec("UPDATE jobs SET state='failed' WHERE request_id IN (SELECT child_id FROM request_children WHERE parent_id=? AND position=1)", parent)
	a.refreshAlbums()
	var state string
	a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", parent).Scan(&state)
	if state != "failed" {
		t.Fatal("partial album falsely fulfilled")
	}
	expect(t, member.call("POST", "/api/v2/tasks/"+parentJob, "", ""), 200)
	a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", child).Scan(&state)
	if state != "available" {
		t.Fatal("retry requeued available audio")
	}
	a.DB.Exec("UPDATE jobs SET state='available' WHERE request_id IN (SELECT child_id FROM request_children WHERE parent_id=?)", parent)
	a.refreshAlbums()
	a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", parent).Scan(&state)
	if state != "available" {
		t.Fatal("completed album not fulfilled")
	}
}
func TestAlbumDecisionIsolation(t *testing.T) {
	a, admin, member := albumFixture(t)
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"other","password":"test-password-long","direct":false}`, ""), 201)
	other := login(t, a, "other")
	w := member.call("POST", "/api/v2/requests", `{"target":"deezer:album:42"}`, "album-decline-key")
	expect(t, w, 201)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	parent := result["id"]
	expect(t, other.call("POST", "/api/v2/requests/"+parent, `{"decision":"approved","reason":""}`, ""), 404)
	if strings.Contains(other.call("GET", "/api/v2/requests", "", "").Body.String(), "The album") {
		t.Fatal("album leaked")
	}
	expect(t, admin.call("POST", "/api/v2/requests/"+parent, `{"decision":"declined","reason":"No"}`, ""), 200)
	var count int
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&count)
	if count != 0 {
		t.Fatal("declined album scheduled downloads")
	}
}
func TestAlbumDirectGrantRevocation(t *testing.T) {
	a, admin, member := albumFixture(t)
	a.DB.Exec("UPDATE users SET direct=1 WHERE username='member'")
	member = login(t, a, "member")
	w := member.call("POST", "/api/v2/requests", `{"target":"deezer:album:42"}`, "album-direct-key")
	expect(t, w, 201)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	parent := result["id"]
	a.DB.Exec("UPDATE users SET direct=0 WHERE username='member'")
	a.runAcquisition(context.Background())
	var decision string
	a.DB.QueryRow("SELECT decision FROM requests WHERE id=?", parent).Scan(&decision)
	if decision != "awaiting_approval" {
		t.Fatal("revoked grant did not restore album approval")
	}
	expect(t, admin.call("POST", "/api/v2/requests/"+parent, `{"decision":"approved","reason":"Explicit album approval"}`, ""), 200)
	var children int
	a.DB.QueryRow("SELECT count(*) FROM request_children WHERE parent_id=?", parent).Scan(&children)
	if children != 2 {
		t.Fatal("reapproval duplicated album work")
	}
}

func TestApprovedAlbumExecutesTracksAndReusesExistingMusic(t *testing.T) {
	a, admin, member := albumFixture(t)
	fixture := &navFixture{songs: map[string]Song{"A": {ID: "A", Title: "First song", Artist: "Test artist", Album: "The album", Duration: 2}, "B": {ID: "B", Title: "Second song", Artist: "Test artist", Album: "The album", Duration: 2}}, playlists: map[string]RemotePlaylist{}, stars: map[string]map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(fixture.handler))
	defer server.Close()
	a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "fixture"})
	w := member.call("POST", "/api/v2/requests", `{"target":"deezer:album:42"}`, "album-execution-key")
	expect(t, w, 201)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	parent := result["id"]
	expect(t, admin.call("POST", "/api/v2/requests/"+parent, `{"decision":"approved","reason":"Album approval"}`, ""), 200)
	for range 2 {
		a.runAcquisition(context.Background())
		a.refreshAlbums()
	}
	var state string
	a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", parent).Scan(&state)
	if state != "available" {
		t.Fatalf("album did not complete through track execution: %s", state)
	}
	var attempts int
	a.DB.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts)
	if attempts != 0 {
		t.Fatal("existing album music was downloaded again")
	}
	items, _ := a.albumItems(parent)
	for _, item := range items {
		if item.State != "available" {
			t.Fatal("track was not fulfilled")
		}
	}
}
