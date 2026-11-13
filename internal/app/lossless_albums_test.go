package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func losslessAlbumWire(id, source string) map[string]any {
	return map[string]any{"id": id, "source": source, "title": "The album", "artists": []Artist{{Name: "Test artist"}}, "number_of_tracks": 2, "picture": map[string]string{"medium": "https://lossless.am/cover.jpg"}}
}
func losslessAlbumTracks(source string) []map[string]any {
	tracks := []map[string]any{}
	for i, title := range []string{"First song", "Second song"} {
		tracks = append(tracks, map[string]any{"id": i + 1, "source": source, "title": title, "duration": 2, "artists": []Artist{{Name: "Test artist"}}, "album": map[string]any{"id": "42", "title": "The album"}, "disc_number": i + 1, "track_number": 1})
	}
	return tracks
}
func losslessAlbumTestFixture(t *testing.T) (*App, client, client, *int) {
	t.Helper()
	albumCalls := 0
	nav := &navFixture{songs: map[string]Song{"A": {ID: "A", Title: "First song", Artist: "Test artist", Album: "The album", Duration: 2}, "B": {ID: "B", Title: "Second song", Artist: "Test artist", Album: "The album", Duration: 2}}, playlists: map[string]RemotePlaylist{}, stars: map[string]map[string]bool{}}
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		if source == "" {
			source = "main"
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/"):
			nav.handler(w, r)
		case r.URL.Path == "/api/search":
			send(w, 200, map[string]any{"tracks": []Track{}, "albums": []any{losslessAlbumWire("42", "main"), losslessAlbumWire("42", "alt"), losslessAlbumWire("opaque-42", "main")}})
		case strings.HasPrefix(r.URL.Path, "/api/album/"):
			albumCalls++
			albumID := strings.TrimPrefix(r.URL.Path, "/api/album/")
			tracks := losslessAlbumTracks(source)
			for _, track := range tracks {
				track["album"].(map[string]any)["id"] = albumID
			}
			send(w, 200, map[string]any{"album": losslessAlbumWire(albumID, source), "tracks": tracks})
		case strings.HasPrefix(r.URL.Path, "/api/track/"):
			var id int
			fmt.Sscanf(r.URL.Path, "/api/track/%d", &id)
			if id < 1 || id > 2 {
				http.NotFound(w, r)
				return
			}
			send(w, 200, map[string]any{"tracks": []any{losslessAlbumTracks(source)[id-1]}})
		default:
			t.Errorf("unexpected provider request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long","direct":false}`, ""), 201)
	member := login(t, a, "member")
	return a, admin, member, &albumCalls
}
func TestLosslessAlbumSearchAndDirectLink(t *testing.T) {
	_, _, member, _ := losslessAlbumTestFixture(t)
	w := member.call("GET", "/api/v2/search?provider=lossless&q=Album&details=1", "", "")
	expect(t, w, 200)
	var results losslessCatalogResult
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results.Albums) != 3 || albumTarget(results.Albums[0]) != "lossless:album:main:42" || albumTarget(results.Albums[1]) != "lossless:album:alt:42" || albumTarget(results.Albums[2]) != "lossless:album:main:opaque-42" {
		t.Fatal(w.Body.String())
	}
	w = member.call("GET", "/api/v2/search?provider=lossless&q=https%3A%2F%2Flossless.am%2Fapp%2Falbum%2Fmain%2F42&details=1", "", "")
	expect(t, w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if results.Album == nil || albumTarget(*results.Album) != "lossless:album:main:42" || len(results.Tracks) != 2 || results.Tracks[0].Provider != "lossless" {
		t.Fatal(w.Body.String())
	}
}
func TestLosslessAlbumSingleApprovalAndExecution(t *testing.T) {
	a, admin, member, calls := losslessAlbumTestFixture(t)
	w := member.call("POST", "/api/v2/requests", `{"target":"lossless:album:main:42"}`, "lossless-album-key")
	expect(t, w, 201)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	parent := result["id"]
	expect(t, member.call("POST", "/api/v2/requests", `{"target":"lossless:album:main:42"}`, "lossless-album-key"), 200)
	if *calls != 1 {
		t.Fatalf("idempotent retry fetched changed album: %d", *calls)
	}
	var jobs int
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&jobs)
	if jobs != 0 {
		t.Fatal("unapproved album scheduled downloads")
	}
	w = member.call("GET", "/api/v2/requests", "", "")
	expect(t, w, 200)
	var requests []Request
	json.Unmarshal(w.Body.Bytes(), &requests)
	if len(requests) != 1 || requests[0].Kind != "album" || requests[0].TrackCount != 2 || len(requests[0].Items) != 2 {
		t.Fatal(w.Body.String())
	}
	expect(t, member.call("PUT", "/api/v2/quality", `{"quality":["128"]}`, ""), 200)
	expect(t, admin.call("POST", "/api/v2/requests/"+parent, `{"decision":"approved","reason":"Approve album"}`, ""), 200)
	if *calls != 1 {
		t.Fatal("approval refetched the frozen album snapshot")
	}
	rows, err := a.DB.Query("SELECT r.target,r.quality,m.metadata FROM request_children c JOIN requests r ON r.id=c.child_id JOIN request_metadata m ON m.request_id=r.id WHERE c.parent_id=? ORDER BY c.position", parent)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		var target, quality, metadata string
		rows.Scan(&target, &quality, &metadata)
		count++
		if target != fmt.Sprintf("lossless:track:main:%d", count) || quality != `["lossless","320","128"]` || !strings.Contains(metadata, `"provider":"lossless"`) {
			t.Fatalf("wrong child: %s %s %s", target, quality, metadata)
		}
	}
	rows.Close()
	if count != 2 {
		t.Fatal("missing album children")
	}
	if err := a.seal("navidrome", Credentials{URL: "http://navidrome.test", Username: "service", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		a.runAcquisition(context.Background())
		a.refreshAlbums()
	}
	var state string
	a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", parent).Scan(&state)
	if state != "available" {
		t.Fatalf("album did not complete through provider-aware track execution: %s", state)
	}
	a.DB.QueryRow("SELECT count(*) FROM attempts").Scan(&jobs)
	if jobs != 0 {
		t.Fatal("existing music downloaded again")
	}
}
func TestLosslessAlbumIdentityAndRetry(t *testing.T) {
	a, admin, _, _ := losslessAlbumTestFixture(t)
	for i, target := range []string{"lossless:album:main:42", "lossless:album:alt:42", "lossless:album:main:opaque-42", "lossless:album:main:00042"} {
		w := admin.call("POST", "/api/v2/requests", mustJSON(map[string]string{"target": target}), fmt.Sprintf("album-identity-%d", i))
		expect(t, w, 201)
		var result map[string]string
		json.Unmarshal(w.Body.Bytes(), &result)
		var snapshot string
		a.DB.QueryRow("SELECT snapshot FROM album_requests WHERE request_id=?", result["id"]).Scan(&snapshot)
		var album AlbumSnapshot
		json.Unmarshal([]byte(snapshot), &album)
		if album.SourceID != strings.Split(target, ":")[3] {
			t.Fatal("opaque album ID lost")
		}
		var parentJob, first, second string
		a.DB.QueryRow("SELECT id FROM jobs WHERE request_id=?", result["id"]).Scan(&parentJob)
		a.DB.QueryRow("SELECT child_id FROM request_children WHERE parent_id=? AND position=0", result["id"]).Scan(&first)
		a.DB.QueryRow("SELECT child_id FROM request_children WHERE parent_id=? AND position=1", result["id"]).Scan(&second)
		a.DB.Exec("UPDATE jobs SET state='available' WHERE request_id=?", first)
		a.DB.Exec("UPDATE jobs SET state='failed' WHERE request_id=?", second)
		a.refreshAlbums()
		expect(t, admin.call("POST", "/api/v2/tasks/"+parentJob, "", ""), 200)
		var state string
		a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", first).Scan(&state)
		if state != "available" {
			t.Fatal("retry reacquired an available song")
		}
		a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", second).Scan(&state)
		if state != "queued" {
			t.Fatal("failed child not retried")
		}
	}
	expect(t, admin.call("POST", "/api/v2/requests", `{"target":"lossless:album:alt:42"}`, "album-identity-0"), 409)
	claim := a.claimAcquisition()
	if claim == nil || !strings.HasPrefix(claim.target, "lossless:track:") {
		t.Fatal("album parent entered track acquisition")
	}
}
func TestLosslessAlbumRejectsIncompleteAndMismatchedMetadata(t *testing.T) {
	for _, mode := range []string{"wrong album", "wrong source", "missing track", "duplicate track", "invalid track", "foreign track"} {
		t.Run(mode, func(t *testing.T) {
			a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
				album := losslessAlbumWire("42", "main")
				tracks := losslessAlbumTracks("main")
				switch mode {
				case "wrong album":
					album["id"] = "43"
				case "wrong source":
					album["source"] = "alt"
				case "missing track":
					tracks = tracks[:1]
				case "duplicate track":
					tracks[1] = tracks[0]
				case "invalid track":
					tracks[1]["title"] = ""
				case "foreign track":
					tracks[1]["source"] = "alt"
				}
				send(w, 200, map[string]any{"album": album, "tracks": tracks})
			})
			if _, err := (losslessProvider{a}).Album(context.Background(), "lossless:album:main:42"); err == nil {
				t.Fatal("accepted " + mode)
			}
		})
	}
	for _, bad := range []string{"https://evil.test/app/album/main/42", "https://lossless.am@evil.test/app/album/main/42", "https://lossless.am/app/album/main/../42", "lossless:album:main:0", "lossless:album:main:bad/id", "https://lossless.am/app/track/main/42"} {
		if _, err := losslessAlbumTarget(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestLosslessAlbumLinkFormsAndDecisionIsolation(t *testing.T) {
	for _, link := range []string{"https://lossless.am/app/album/main/opaque-42", "https://lossless.am/app/album/opaque-42?source=main"} {
		if target, err := losslessAlbumTarget(link); err != nil || target != "lossless:album:main:opaque-42" {
			t.Fatalf("%s: %s %v", link, target, err)
		}
	}
	for _, decision := range []string{"withdrawn", "declined"} {
		t.Run(decision, func(t *testing.T) {
			a, admin, member, _ := losslessAlbumTestFixture(t)
			w := member.call("POST", "/api/v2/requests", `{"target":"lossless:album:main:42"}`, "album-decision-key")
			expect(t, w, 201)
			var result map[string]string
			json.Unmarshal(w.Body.Bytes(), &result)
			expect(t, member.call("POST", "/api/v2/requests/"+result["id"], `{"decision":"approved","reason":""}`, ""), 403)
			actor := admin
			if decision == "withdrawn" {
				actor = member
			}
			expect(t, actor.call("POST", "/api/v2/requests/"+result["id"], mustJSON(map[string]string{"decision": decision, "reason": "No"}), ""), 200)
			a.runAcquisition(context.Background())
			var count int
			a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&count)
			if count != 0 {
				t.Fatal("unapproved album started acquisition")
			}
		})
	}
}
