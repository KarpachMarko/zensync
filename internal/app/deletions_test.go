package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type deletionFixture struct {
	a                     *App
	admin, member, direct client
	path                  string
	scanFail              atomic.Bool
	scans                 atomic.Int32
}

func newDeletionFixture(t *testing.T, legacyDefaults ...bool) *deletionFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if len(legacyDefaults) > 0 && legacyDefaults[0] {
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`CREATE TABLE jobs(id TEXT PRIMARY KEY,request_id TEXT NOT NULL UNIQUE REFERENCES requests(id),state TEXT NOT NULL DEFAULT 'blocked',stage TEXT NOT NULL DEFAULT 'Integration not configured',created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
		db.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	a, e := Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.DB.Close() })
	a.musicDir = t.TempDir()
	f := &deletionFixture{a: a, path: filepath.Join(a.musicDir, "song.mp3")}
	if e = os.WriteFile(f.path, []byte("original audio"), 0644); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/search3.view":
			songs := []Song{{ID: "song-1", Title: "Shared song"}}
			for n := 1; n < 100; n++ {
				songs = append(songs, Song{ID: string(rune('a' + n)), Title: "Other song"})
			}
			send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "searchResult3": map[string]any{"song": songs}}})
		case "/rest/getSong.view":
			send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "song": Song{ID: "song-1", Title: "Shared song", Artist: "Artist", Path: "/different-container/song.mp3", Size: 14}}})
		case "/rest/download.view":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte("original audio"))
		case "/rest/startScan.view":
			f.scans.Add(1)
			if f.scanFail.Load() {
				w.WriteHeader(503)
				return
			}
			send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok"}})
		default:
			t.Errorf("Unexpected Navidrome endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	if e = a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "test"}); e != nil {
		t.Fatal(e)
	}
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	f.admin = login(t, a, "admin")
	expect(t, f.admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	expect(t, f.admin.call("POST", "/api/v2/users", `{"username":"direct","password":"test-password-long","direct":true}`, ""), 201)
	f.member = login(t, a, "member")
	f.direct = login(t, a, "direct")
	return f
}
func deletionID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	expect(t, w, 201)
	var out map[string]string
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	return out["id"]
}
func (f *deletionFixture) request(t *testing.T, c client, key string) string {
	return deletionID(t, c.call("POST", "/api/v2/deletion-requests", `{"songId":"song-1"}`, key))
}
func (f *deletionFixture) userID(t *testing.T, c client) string {
	t.Helper()
	var out struct {
		User User `json:"user"`
	}
	w := c.call("GET", "/api/v2/session", "", "")
	expect(t, w, 200)
	json.Unmarshal(w.Body.Bytes(), &out)
	return out.User.ID
}
func (f *deletionFixture) grant(t *testing.T, c client, allowed bool) {
	t.Helper()
	b := "false"
	if allowed {
		b = "true"
	}
	expect(t, f.admin.call("PUT", "/api/v2/users/"+f.userID(t, c)+"/permissions", `{"directDelete":`+b+`}`, ""), 200)
}
func assertFile(t *testing.T, p string, want bool) {
	t.Helper()
	_, e := os.Stat(p)
	if want && e != nil {
		t.Fatal(e)
	}
	if !want && !os.IsNotExist(e) {
		t.Fatalf("Expected %s removed, got %v", p, e)
	}
}

func TestDeletionApprovalAndIsolation(t *testing.T) {
	f := newDeletionFixture(t)
	rid := f.request(t, f.direct, "delete-approval") // Direct acquisition never grants deletion.
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, true)
	expect(t, f.direct.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`, ""), 403)
	expect(t, f.member.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`, ""), 404)
	if strings.Contains(f.member.call("GET", "/api/v2/requests", "", "").Body.String(), rid) {
		t.Fatal("Request leaked")
	}
	expect(t, f.admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`, ""), 200)
	// Deletion must never enter the downloader.
	if f.a.claimAcquisition() != nil {
		t.Fatal("Deletion claimed as acquisition")
	}
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, false)
	b, e := os.ReadFile(filepath.Join(f.a.dataDir, "trash", rid, "audio"))
	if e != nil || string(b) != "original audio" {
		t.Fatal("Recovery copy unavailable", e)
	}
	var state string
	f.a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", rid).Scan(&state)
	if state != "completed" || f.scans.Load() != 1 {
		t.Fatal(state, f.scans.Load())
	}
	expect(t, f.direct.call("POST", "/api/v2/deletion-requests", `{"songId":"song-1"}`, "delete-approval"), 200)
}
func TestDeletionGrantRevocationAndReapproval(t *testing.T) {
	f := newDeletionFixture(t)
	f.grant(t, f.member, true)
	rid := f.request(t, f.member, "delete-with-grant")
	f.grant(t, f.member, false)
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, true)
	var decision string
	f.a.DB.QueryRow("SELECT decision FROM requests WHERE id=?", rid).Scan(&decision)
	if decision != "awaiting_approval" {
		t.Fatal(decision)
	}
	expect(t, f.admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`, ""), 200)
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, false)
}
func TestDeletionChangedFileAndSymlink(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed", true: "symlink"}[symlink], func(t *testing.T) {
			f := newDeletionFixture(t)
			rid := f.request(t, f.admin, "delete-unsafe-file")
			if symlink {
				outside := filepath.Join(t.TempDir(), "outside.mp3")
				os.WriteFile(outside, []byte("original audio"), 0600)
				os.Remove(f.path)
				os.Symlink(outside, f.path)
			} else {
				os.WriteFile(f.path, []byte("replacement audio"), 0644)
			}
			f.a.runDeletion(context.Background())
			assertFile(t, f.path, true)
			var state string
			f.a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", rid).Scan(&state)
			if state != "failed" {
				t.Fatal(state)
			}
		})
	}
}
func TestDeletionScanRetryAndStaleMappings(t *testing.T) {
	f := newDeletionFixture(t)
	// Seed a verified shared asset and migration match to check invalidation.
	f.a.DB.Exec("INSERT INTO assets(target,file_path,quality,metadata,server_id) VALUES('deezer:track:1','song.mp3','128','{}','song-1')")
	owner := f.userID(t, f.admin)
	for _, stmt := range []string{
		"INSERT INTO migrations(id,owner,fingerprint,name,source,report) VALUES('migration','" + owner + "','fixture','Migration','{}','{}')",
		"INSERT INTO source_songs(id,migration_id,source_id,metadata,server_id) VALUES('source','migration','1','{}','song-1')",
		"INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) VALUES('old-request','" + owner + "','old-request','deezer:track:1','Song','Artist',0,'[]','approved')",
		"INSERT INTO jobs(id,request_id,state) VALUES('old-job','old-request','available')",
	} {
		if _, e := f.a.DB.Exec(stmt); e != nil {
			t.Fatal(e)
		}
	}
	rid := f.request(t, f.admin, "delete-retry-scan")
	f.scanFail.Store(true)
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, false)
	var phase, state, jid string
	f.a.DB.QueryRow("SELECT phase FROM song_deletions WHERE request_id=?", rid).Scan(&phase)
	if phase != "removed" {
		t.Fatal(phase)
	}
	var sourceID string
	var deferred bool
	if e := f.a.DB.QueryRow("SELECT server_id,deferred FROM source_songs WHERE id='source'").Scan(&sourceID, &deferred); e != nil || sourceID != "" || !deferred {
		t.Fatal("Stale migration mapping", sourceID, deferred, e)
	}
	var oldState string
	f.a.DB.QueryRow("SELECT state FROM jobs WHERE id='old-job'").Scan(&oldState)
	if oldState != "cancelled" {
		t.Fatal("Old acquisition can recreate removed song", oldState)
	}
	var listing struct {
		Songs []Song `json:"songs"`
		Next  int    `json:"nextOffset"`
	}
	w := f.admin.call("GET", "/api/v2/library?details=1", "", "")
	expect(t, w, 200)
	json.Unmarshal(w.Body.Bytes(), &listing)
	if len(listing.Songs) != 99 || listing.Next != 100 {
		t.Fatal("Removed song visibility/pagination", len(listing.Songs), listing.Next)
	}

	var assets int
	f.a.DB.QueryRow("SELECT count(*) FROM assets").Scan(&assets)
	if assets != 0 {
		t.Fatal("Stale availability retained")
	}
	f.a.DB.QueryRow("SELECT id,state FROM jobs WHERE request_id=?", rid).Scan(&jid, &state)
	if state != "failed" {
		t.Fatal(state)
	}
	expect(t, f.admin.call("POST", "/api/v2/tasks/"+jid, "{}", ""), 200)
	f.scanFail.Store(false)
	f.a.runDeletion(context.Background())
	f.a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jid).Scan(&state)
	if state != "completed" {
		t.Fatal(state)
	}
}
func TestDeletionPreparedRecovery(t *testing.T) {
	f := newDeletionFixture(t)
	rid := f.request(t, f.admin, "delete-prepared")
	dir := filepath.Join(f.a.dataDir, "trash", rid)
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "audio"), []byte("original audio"), 0600)
	f.a.DB.Exec("UPDATE song_deletions SET phase='prepared' WHERE request_id=?", rid)
	os.Remove(f.path)
	// Reopen the database to exercise recovery after the source removal was interrupted.
	f.a.DB.Close()
	reopened, e := Open(filepath.Join(f.a.dataDir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	f.a.DB = reopened.DB
	f.a.runDeletion(context.Background())
	var state string
	f.a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", rid).Scan(&state)
	if state != "completed" {
		t.Fatal(state)
	}
}
func TestDeletionBearerScopeAndRevocation(t *testing.T) {
	f := newDeletionFixture(t)
	w := f.member.call("POST", "/api/v2/deletion-api-keys", "{}", "")
	expect(t, w, 201)
	var key APIKey
	json.Unmarshal(w.Body.Bytes(), &key)
	bearer := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+key.Token)
		r.Header.Set("Idempotency-Key", "bearer-delete-key")
		w := httptest.NewRecorder()
		f.a.Handler().ServeHTTP(w, r)
		return w
	}
	rid := deletionID(t, bearer("POST", "/api/v2/deletion-requests", `{"songId":"song-1"}`))
	expect(t, bearer("GET", "/api/v2/deletion-requests/"+rid, ""), 200)
	expect(t, bearer("PUT", "/api/v2/users/anything/permissions", `{"directDelete":true}`), 403)
	expect(t, bearer("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`), 403)
	expect(t, f.member.call("PUT", "/api/v2/users/"+f.userID(t, f.member)+"/permissions", `{"directDelete":true}`, ""), 403)
	expect(t, f.member.call("DELETE", "/api/v2/deletion-api-keys/"+key.ID, "", ""), 200)
	expect(t, bearer("POST", "/api/v2/deletion-requests", `{"songId":"song-1"}`), 401)
	assertFile(t, f.path, true)
}
func TestDeletionWithdrawDeclineAndActiveAcquisition(t *testing.T) {
	f := newDeletionFixture(t)
	rid := f.request(t, f.member, "delete-withdraw")
	expect(t, f.member.call("POST", "/api/v2/requests/"+rid, `{"decision":"withdrawn"}`, ""), 200)
	rid = f.request(t, f.member, "delete-decline")
	expect(t, f.admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"declined","reason":"Keep this recording"}`, ""), 200)
	rid = f.request(t, f.admin, "delete-final")
	f.a.activeJobs = map[string]string{"running": "deezer:track:2"}
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, true)
	f.a.activeJobs = nil
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, false)
}

func TestDeletionDirectGrantExecutesWithoutApproval(t *testing.T) {
	f := newDeletionFixture(t)
	f.grant(t, f.member, true)
	rid := f.request(t, f.member, "delete-direct-now")
	f.a.runDeletion(context.Background())
	assertFile(t, f.path, false)
	var state string
	f.a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", rid).Scan(&state)
	if state != "completed" {
		t.Fatal(state)
	}
}

func TestSongPathResolutionWithDuplicateCopies(t *testing.T) {
	for _, scenario := range []struct {
		name, reported, exactData string
		wantExact, wantError      bool
	}{
		{"exact real path", "exact", "original audio", true, false},
		{"exact path byte mismatch", "exact", "different audio", false, true},
		{"different mounts remain ambiguous", "/other-container/Artist/Album/song.mp3", "original audio", false, true},
		{"missing exact path must not fall back", "exact", "missing", false, true},
		{"different mounts unique copy", "/other-container/Artist/Album/song.mp3", "unique", true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, err := Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer a.DB.Close()
			a.musicDir = t.TempDir()
			for _, rel := range []string{"Artist/Album/song.mp3", "Album/song.mp3", "song.mp3"} {
				if scenario.exactData == "unique" && rel != "Artist/Album/song.mp3" {
					continue
				}
				if (scenario.exactData == "different audio" || scenario.exactData == "missing") && rel == "Album/song.mp3" {
					continue
				}
				if scenario.exactData == "missing" && rel == "Artist/Album/song.mp3" {
					continue
				}
				path := filepath.Join(a.musicDir, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				data := "original audio"
				if rel == "Artist/Album/song.mp3" && scenario.exactData != "unique" {
					data = scenario.exactData
				}
				if err := os.WriteFile(path, []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "audio/mpeg")
				w.Write([]byte("original audio"))
			}))
			defer server.Close()
			reported := scenario.reported
			if reported == "exact" {
				reported = filepath.Join(a.musicDir, "Artist/Album/song.mp3")
			}
			got, _, err := a.deletionPath(context.Background(), Credentials{URL: server.URL}, Song{ID: "song", Path: reported})
			if err != nil && strings.Contains(err.Error(), "deletion") {
				t.Fatal("Shared path lookup must not describe metadata updates as deletion")
			}
			if scenario.wantError && err == nil {
				t.Fatalf("Must not choose an arbitrary duplicate: %s", got)
			}
			if scenario.wantExact && (err != nil || got != "Artist/Album/song.mp3") {
				t.Fatalf("Exact real path should win: %q %v", got, err)
			}
		})
	}
}

func TestDeletionLegacyDefaultAndStartupRecovery(t *testing.T) {
	f := newDeletionFixture(t, true)
	rid := f.request(t, f.admin, "legacy-delete-test")
	var state, stage string
	if err := f.a.DB.QueryRow("SELECT state,stage FROM jobs WHERE request_id=?", rid).Scan(&state, &stage); err != nil || state != "queued" || stage != "Queued for library removal" {
		t.Fatalf("legacy insert: %s %s %v", state, stage, err)
	}
	// Simulate an already-stuck deletion created by the previous version.
	if _, err := f.a.DB.Exec("UPDATE jobs SET state='blocked' WHERE request_id=?", rid); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []struct{ id, decision, stage string }{
		{"real-block", "approved", "Deletion requires administrator approval"},
		{"unapproved", "awaiting_approval", "Queued for library removal"},
		{"failed", "approved", "Queued for library removal"},
	} {
		if _, err := f.a.DB.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) SELECT ?,owner,?,'navidrome:song:other','Other','Artist',0,'[]',? FROM requests WHERE id=?", seed.id, seed.id, seed.decision, rid); err != nil {
			t.Fatal(err)
		}
		if _, err := f.a.DB.Exec("INSERT INTO song_deletions(request_id,song_id,file_path,checksum) SELECT ?,'other',file_path,checksum FROM song_deletions WHERE request_id=?", seed.id, rid); err != nil {
			t.Fatal(err)
		}
		initial := "blocked"
		if seed.id == "failed" {
			initial = "failed"
		}
		if _, err := f.a.DB.Exec("INSERT INTO jobs(id,request_id,state,stage) VALUES(?,?,?,?)", seed.id, seed.id, initial, seed.stage); err != nil {
			t.Fatal(err)
		}
	}
	// A cancelled worker still performs startup recovery, then exits before running jobs.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	<-f.a.StartWorker(ctx)
	if err := f.a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", rid).Scan(&state); err != nil || state != "queued" {
		t.Fatal("stuck deletion not recovered", state, err)
	}
	for _, id := range []string{"real-block", "unapproved", "failed"} {
		want := "blocked"
		if id == "failed" {
			want = "failed"
		}
		if err := f.a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", id).Scan(&state); err != nil || state != want {
			t.Fatalf("recovery changed %s: %s %v", id, state, err)
		}
	}
	f.a.runDeletion(context.Background())
	if err := f.a.DB.QueryRow("SELECT state FROM jobs WHERE request_id=?", rid).Scan(&state); err != nil || state != "completed" {
		t.Fatal("recovered deletion not executed", state, err)
	}
	assertFile(t, f.path, false)
}
