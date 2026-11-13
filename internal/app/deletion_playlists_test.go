package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type preserveFixture struct {
	songs             map[string]Song
	a                 *App
	admin             client
	ids               map[string][]string
	writes            []string
	failWrite         bool
	mutateBeforeWrite bool
	reads             map[string]int
	path              string
}

func newPreserveFixture(t *testing.T) *preserveFixture {
	t.Helper()
	a, admin, songs := metadataLibraryFixture(t)
	f := &preserveFixture{songs: songs, a: a, admin: admin, path: filepath.Join(a.musicDir, songs["one"].Path), ids: map[string][]string{
		"service-list": {"two", "one", "two", "one"}, "personal-list": {"one", "two", "one"}, "public-other": {"one"}, "readonly": {"one"},
	}, reads: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		user := r.Form.Get("u")
		out := map[string]any{"status": "ok"}
		id := r.Form.Get("id")
		switch strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/"), ".view") {
		case "getSong":
			s := songs[id]
			if info, err := os.Stat(filepath.Join(a.musicDir, s.Path)); err == nil {
				s.Size = info.Size()
			}
			out["song"] = s
		case "download":
			b, err := os.ReadFile(filepath.Join(a.musicDir, songs[id].Path))
			if err != nil {
				http.Error(w, "missing", 404)
				return
			}
			w.Write(b)
			return
		case "getPlaylists":
			owned := "service-list"
			if user == "personal" {
				owned = "personal-list"
			}
			out["playlists"] = map[string]any{"playlist": []RemotePlaylist{{ID: owned, Name: owned, Owner: user}, {ID: "public-other", Name: "Other user's", Owner: "other", Public: true}, {ID: "readonly", Name: "Smart", Owner: user, Readonly: true}}}
		case "getPlaylist":
			f.reads[id]++
			if f.mutateBeforeWrite && id == "service-list" && f.reads[id] == 2 {
				f.ids[id] = append(f.ids[id], "new-song")
			}
			entries := []Song{}
			for _, sid := range f.ids[id] {
				entries = append(entries, Song{ID: sid})
			}
			out["playlist"] = RemotePlaylist{ID: id, Name: id, Owner: user, Entries: entries}
		case "createPlaylist":
			// All playlist writes must happen while the removed file still exists.
			if _, err := os.Stat(f.path); err != nil {
				t.Error("playlist update happened after deletion")
			}
			id = r.Form.Get("playlistId")
			f.writes = append(f.writes, id)
			f.ids[id] = append([]string{}, r.Form["songId"]...)
			if f.failWrite {
				http.Error(w, "response lost after write", 503)
				return
			}
			out["playlist"] = RemotePlaylist{ID: id, Owner: user}
		case "getScanStatus":
			out["scanStatus"] = map[string]any{"scanning": false}
		case "getMusicFolders":
			out["musicFolders"] = map[string]any{"musicFolder": []map[string]any{{"id": "1"}}}
		case "startScan":
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		send(w, 200, map[string]any{"subsonic-response": out})
	}))
	t.Cleanup(server.Close)
	if err := a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "password"}); err != nil {
		t.Fatal(err)
	}
	var uid string
	if err := a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := a.seal("navidrome:"+uid, Credentials{Username: "personal", Password: "personal-password"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *preserveFixture) request(t *testing.T) string {
	t.Helper()
	response := f.admin.call("POST", "/api/v2/deletion-requests", `{"songId":"one","keepSongId":"two"}`, "preserve-playlists-key")
	expect(t, response, 201)
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.ID
}
func (f *preserveFixture) job(t *testing.T, rid string) (string, string, string) {
	t.Helper()
	var jid, state, stage string
	if err := f.a.DB.QueryRow("SELECT id,state,stage FROM jobs WHERE request_id=?", rid).Scan(&jid, &state, &stage); err != nil {
		t.Fatal(err)
	}
	return jid, state, stage
}

func TestKeepSongPreservesOwnedPlaylistsBeforeDeletion(t *testing.T) {
	f := newPreserveFixture(t)
	rid := f.request(t)
	// The idempotency key cannot be reused for a different survivor (including omission).
	expect(t, f.admin.call("POST", "/api/v2/deletion-requests", `{"songId":"one"}`, "preserve-playlists-key"), 409)
	f.a.runDeletion(context.Background())
	_, state, stage := f.job(t, rid)
	if state != "completed" {
		t.Fatal(state, stage)
	}
	if !slices.Equal(f.ids["service-list"], []string{"two", "two", "two", "two"}) || !slices.Equal(f.ids["personal-list"], []string{"two", "two", "two"}) {
		t.Fatal("order or repetitions changed", f.ids)
	}
	if !slices.Equal(f.writes, []string{"service-list", "personal-list"}) {
		t.Fatal("uneditable playlists modified", f.writes)
	}
	assertFile(t, f.path, false)
	assertFile(t, filepath.Join(f.a.musicDir, "two.flac"), true)
}

func TestKeepSongReconcilesUncertainPlaylistWriteBeforeRetryingDeletion(t *testing.T) {
	f := newPreserveFixture(t)
	rid := f.request(t)
	f.failWrite = true
	f.a.runDeletion(context.Background())
	jid, state, stage := f.job(t, rid)
	if state != "failed" || !strings.Contains(stage, "Could not update playlist") {
		t.Fatal(state, stage)
	}
	assertFile(t, f.path, true)
	// The remote write succeeded despite the failed response. Simulate a process restart.
	f.failWrite = false
	var seq int
	var name, dbPath string
	if err := f.a.DB.QueryRow("PRAGMA database_list").Scan(&seq, &name, &dbPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	reopened.musicDir = f.a.musicDir
	f.a = reopened
	f.admin.a = reopened
	t.Cleanup(func() { reopened.DB.Close() })
	expect(t, f.admin.call("POST", "/api/v2/tasks/"+jid, "{}", ""), 200)
	f.a.runDeletion(context.Background())
	_, state, stage = f.job(t, rid)
	if state != "completed" {
		t.Fatal(state, stage)
	}
	if !slices.Equal(f.writes, []string{"service-list", "personal-list"}) {
		t.Fatal("uncertain write repeated", f.writes)
	}
	assertFile(t, f.path, false)
}

func TestKeepSongStopsOnConcurrentPlaylistChanges(t *testing.T) {
	f := newPreserveFixture(t)
	rid := f.request(t)
	f.mutateBeforeWrite = true
	f.a.runDeletion(context.Background())
	_, state, stage := f.job(t, rid)
	if state != "failed" || !strings.Contains(stage, "changed before replacement") {
		t.Fatal(state, stage)
	}
	if len(f.writes) != 0 {
		t.Fatal("concurrent edit overwritten")
	}
	assertFile(t, f.path, true)
}

func TestKeepSongValidatesSurvivorAndServer(t *testing.T) {
	f := newPreserveFixture(t)
	for i, body := range []string{`{"songId":"one","keepSongId":"one"}`, `{"songId":"one","keepSongId":"missing"}`} {
		response := f.admin.call("POST", "/api/v2/deletion-requests", body, fmt.Sprintf("invalid-survivor-%d", i))
		if response.Code < 400 {
			t.Fatal(response.Code)
		}
	}
	rid := f.request(t)
	if err := f.a.seal("navidrome", Credentials{URL: "https://different.invalid", Username: "service", Password: "password"}); err != nil {
		t.Fatal(err)
	}
	f.a.runDeletion(context.Background())
	_, state, stage := f.job(t, rid)
	if state != "failed" || !strings.Contains(stage, "connection changed") {
		t.Fatal(state, stage)
	}
	assertFile(t, f.path, true)
}

func TestKeepSongMissingSurvivorPreventsPlaylistWritesAndDeletion(t *testing.T) {
	f := newPreserveFixture(t)
	rid := f.request(t)
	if err := os.Remove(filepath.Join(f.a.musicDir, "two.flac")); err != nil {
		t.Fatal(err)
	}
	f.a.runDeletion(context.Background())
	_, state, _ := f.job(t, rid)
	if state != "failed" {
		t.Fatal(state)
	}
	if len(f.writes) != 0 {
		t.Fatal("playlists changed without a survivor")
	}
	assertFile(t, f.path, true)
}

func TestKeepSongPlaylistReplacementRequiresAdministrator(t *testing.T) {
	f := newPreserveFixture(t)
	expect(t, f.admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, f.a, "member")
	expect(t, member.call("POST", "/api/v2/deletion-requests", `{"songId":"one","keepSongId":"two"}`, "no-playlist-access"), 403)
}
