package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFirstAccessSetup(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "app.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	c := client{a: a}
	w := c.call("GET", "/api/v2/setup", "", "")
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), `"required":true`) {
		t.Fatal(w.Body)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- c.call("POST", "/api/v2/setup", `{"username":"admin","password":"test-password-long"}`, "").Code
		}()
	}
	wg.Wait()
	close(codes)
	count := map[int]int{}
	for code := range codes {
		count[code]++
	}
	if count[201] != 1 || count[409] != 1 {
		t.Fatalf("setup race %v", count)
	}
	login(t, a, "admin")
	expect(t, c.call("POST", "/api/v2/setup", `{"username":"intruder","password":"test-password-long"}`, ""), 409)
}

type navFixture struct {
	mu                 sync.Mutex
	songs              map[string]Song
	playlists          map[string]RemotePlaylist
	stars              map[string]map[string]bool
	writes, starWrites int
	music              string
}

func (n *navFixture) handler(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r.ParseForm()
	user := r.Form.Get("u")
	endpoint := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/"), ".view")
	out := map[string]any{"status": "ok", "version": "1.16.1", "serverVersion": "fixture"}
	switch endpoint {
	case "download":
		song := n.songs[r.Form.Get("id")]
		path := filepath.Join(n.music, filepath.FromSlash(song.Path))
		data, e := os.ReadFile(path)
		if e != nil {
			http.Error(w, "missing", 404)
			return
		}
		w.Write(data)
		return
	case "getUser":
		out["user"] = map[string]any{"username": user, "adminRole": true}
	case "search3":
		songs := []Song{}
		q := strings.ToLower(r.Form.Get("query"))
		for _, s := range n.songs {
			if q == "" || strings.Contains(strings.ToLower(s.Title), q) {
				copy := s
				copy.Path = "Virtual Artist/Album/01 - Song.flac"
				songs = append(songs, copy)
			}
		}
		out["searchResult3"] = map[string]any{"song": songs}
	case "getSong":
		s, ok := n.songs[r.Form.Get("id")]
		if !ok {
			out["status"] = "failed"
			out["error"] = map[string]any{"code": 70}
		} else {
			if n.stars[user][s.ID] {
				s.Starred = "now"
			}
			out["song"] = s
		}
	case "createPlaylist":
		pid := r.Form.Get("playlistId")
		p := n.playlists[pid]
		if pid == "" {
			pid = fmt.Sprint(len(n.playlists) + 1)
			p = RemotePlaylist{ID: pid, Name: r.Form.Get("name"), Owner: user}
		}
		p.Entries = []Song{}
		for _, sid := range r.Form["songId"] {
			p.Entries = append(p.Entries, n.songs[sid])
		}
		n.playlists[pid] = p
		n.writes++
		out["playlist"] = p
	case "getPlaylist":
		p, ok := n.playlists[r.Form.Get("id")]
		if !ok || p.Owner != user {
			out["status"] = "failed"
			out["error"] = map[string]any{"code": 70}
		} else {
			out["playlist"] = p
		}
	case "getPlaylists":
		ps := []RemotePlaylist{}
		for _, p := range n.playlists {
			if p.Owner == user {
				ps = append(ps, p)
			}
		}
		out["playlists"] = map[string]any{"playlist": ps}
	case "star":
		if n.stars[user] == nil {
			n.stars[user] = map[string]bool{}
		}
		n.stars[user][r.Form.Get("id")] = true
		n.starWrites++
	case "getScanStatus":
		out["scanStatus"] = map[string]bool{"scanning": false}
	case "startScan":
		filepath.WalkDir(n.music, func(path string, d os.DirEntry, e error) error {
			if e == nil && !d.IsDir() && strings.HasSuffix(path, ".flac") {
				rel, _ := filepath.Rel(n.music, path)
				n.songs["B"] = Song{ID: "B", Title: "Second song", Artist: "Test artist", Album: "Test album", Duration: 2, Path: filepath.ToSlash(rel)}
			}
			return nil
		})
	default:
		out["status"] = "failed"
		out["error"] = map[string]any{"code": 0}
	}
	send(w, 200, map[string]any{"subsonic-response": out})
}
func makeTrack(i int, title string) Track {
	var t Track
	t.ID = int64(i)
	t.Title = title
	t.Duration = 2
	t.Artist.Name = "Test artist"
	t.Album.Title = "Test album"
	return t
}
func TestMigrationAcquisitionAndPublication(t *testing.T) {
	ffmpeg, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("ffmpeg required for acquisition integration")
	}
	if _, e = exec.LookPath("ffprobe"); e != nil {
		t.Skip("ffprobe required")
	}
	dir := t.TempDir()
	a, e := Open(filepath.Join(dir, "app.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.musicDir = filepath.Join(dir, "music")
	os.Mkdir(a.musicDir, 0755)
	audio := filepath.Join(dir, "fixture.flac")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-metadata", "title=Second song", "-metadata", "artist=Test artist", "-metadata", "album=Test album", "-metadata", "SOURCEID=2", "-metadata", "track=3", "-metadata", "album_artist=Album artist", audio)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("fixture: %s %v", out, e)
	}
	a.dfiPath = filepath.Join(dir, "fake-d-fi")
	script := "#!/bin/sh\n# Controlled fixture replaces only the external downloader.\ncp '" + audio + "' audio.flac\n"
	os.WriteFile(a.dfiPath, []byte(script), 0700)
	nav := &navFixture{songs: map[string]Song{"A": {ID: "A", Title: "First song", Artist: "Test artist", Album: "Test album", Duration: 2}}, playlists: map[string]RemotePlaylist{}, stars: map[string]map[string]bool{}, music: a.musicDir}
	server := httptest.NewServer(http.HandlerFunc(nav.handler))
	defer server.Close()
	dz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/track/2":
			send(w, 200, makeTrack(2, "Second song"))
		case strings.HasPrefix(r.URL.Path, "/search/track"):
			q := r.URL.Query().Get("q")
			tracks := []Track{}
			if strings.Contains(q, "Second") {
				tracks = append(tracks, makeTrack(2, "Second song"))
			}
			send(w, 200, map[string]any{"data": tracks})
		default:
			send(w, 200, map[string]any{"error": map[string]int{"code": 800}})
		}
	}))
	defer dz.Close()
	a.deezerURL = dz.URL
	adminAnon := client{a: a}
	expect(t, adminAnon.call("POST", "/api/v2/setup", `{"username":"admin","password":"test-password-long"}`, ""), 201)
	admin := login(t, a, "admin")
	for _, name := range []string{"alice", "bob"} {
		expect(t, admin.call("POST", "/api/v2/users", fmt.Sprintf(`{"username":%q,"password":"test-password-long","direct":false}`, name), ""), 201)
	}
	alice, bob := login(t, a, "alice"), login(t, a, "bob")
	expect(t, admin.call("PUT", "/api/v2/connections", mustJSON(map[string]string{"kind": "shared", "url": server.URL, "username": "service", "password": "private-service-password"}), ""), 200)
	expect(t, alice.call("PUT", "/api/v2/connections", `{"kind":"personal","username":"alice-nav","password":"private-alice-password"}`, ""), 200)
	expect(t, admin.call("PUT", "/api/v2/connections", `{"kind":"deezer","arl":"fixture-arl-not-a-real-secret-123456789"}`, ""), 200)
	var encrypted []byte
	a.DB.QueryRow("SELECT value FROM secrets WHERE name='navidrome'").Scan(&encrypted)
	if strings.Contains(string(encrypted), "private-service-password") {
		t.Fatal("plaintext credential stored")
	}
	xml, e := os.ReadFile("../apple/testdata/library.xml")
	if e != nil {
		t.Fatal(e)
	}
	upload := func(c client) string {
		w := c.call("POST", "/api/v2/imports", string(xml), "")
		if w.Code != 200 && w.Code != 201 {
			t.Fatal(w.Body)
		}
		var out map[string]string
		json.Unmarshal(w.Body.Bytes(), &out)
		return out["id"]
	}
	mid := upload(alice)
	if upload(alice) != mid {
		t.Fatal("duplicate upload")
	}
	if upload(bob) == mid {
		t.Fatal("cross-owner upload deduplication")
	}
	expect(t, bob.call("GET", "/api/v2/imports/"+mid, "", ""), 404)
	pls, _ := a.playlists(mid)
	pid := pls[2].ID
	prefix := "/api/v2/imports/" + mid
	expect(t, alice.call("POST", prefix+"/commit", mustJSON(map[string]any{"playlists": []string{pid}, "favorites": true}), ""), 200)
	expect(t, alice.call("POST", prefix+"/match", "", ""), 202)
	a.runMigrationTask(context.Background())
	songs, _ := a.sources(mid)
	var second Source
	for _, s := range songs {
		if s.Metadata.Title == "Second song" {
			second = s
		}
	}
	if second.Candidate == nil {
		t.Fatal("missing candidate")
	}
	var count int
	a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&count)
	if count != 0 {
		t.Fatal("matching created acquisition intent")
	}
	expect(t, alice.call("POST", prefix+"/publish", "", ""), 202)
	a.runMigrationTask(context.Background())
	pls, _ = a.playlists(mid)
	p := pls[2]
	if p.RemoteID == "" || p.State != "active" {
		t.Fatalf("publication failed %+v", p)
	}
	if len(nav.playlists[p.RemoteID].Entries) != 2 {
		t.Fatal("missing repeated occurrence")
	}
	if nav.starWrites != 1 {
		t.Fatal("favorite not imported")
	}
	nav.stars["alice-nav"]["A"] = false
	expect(t, alice.call("POST", prefix+"/publish", "", ""), 202)
	a.runMigrationTask(context.Background())
	if nav.starWrites != 1 {
		t.Fatal("re-starred user's removed favorite")
	}
	songPath := prefix + "/songs/" + second.ID
	expect(t, alice.call("POST", songPath+"/defer", "", ""), 200)
	expect(t, alice.call("POST", songPath+"/acquire", "", ""), 409)
	expect(t, alice.call("POST", songPath+"/resume", "", ""), 200)
	w := alice.call("POST", songPath+"/acquire", "", "")
	expect(t, w, 201)
	var req map[string]string
	json.Unmarshal(w.Body.Bytes(), &req)
	rid := req["id"]
	a.runAcquisition(context.Background())
	a.DB.QueryRow("SELECT count(*) FROM assets").Scan(&count)
	if count != 0 {
		t.Fatal("member acquired before approval")
	}
	expect(t, admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved","reason":"Test approval"}`, ""), 200)
	nav.music = "" // Navidrome cannot see the placed file: reproduce the reported wait.
	a.runAcquisition(context.Background())
	var jobID string
	a.DB.QueryRow("SELECT id FROM jobs WHERE request_id=?", rid).Scan(&jobID)
	var waiting string
	a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jobID).Scan(&waiting)
	if waiting != "awaiting_scan" {
		t.Fatalf("expected indexing wait, got %s", waiting)
	}
	a.DB.Exec("UPDATE scan_waits SET started_at=unixepoch()-301 WHERE job_id=?", jobID)
	a.DB.Exec("DELETE FROM scan_schedule WHERE job_id=?", jobID)
	a.runAcquisition(context.Background())
	a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jobID).Scan(&waiting)
	if waiting != "failed" {
		t.Fatal("indexing wait did not time out")
	}
	expect(t, alice.call("POST", "/api/v2/tasks/"+jobID, "", ""), 200)
	nav.music = a.musicDir
	a.scanRequestedAt = time.Time{}
	a.runAcquisition(context.Background())
	var state, stage string
	a.DB.QueryRow("SELECT state,stage FROM jobs WHERE request_id=?", rid).Scan(&state, &stage)
	if state != "available" {
		t.Fatalf("acquisition: %s %s", state, stage)
	}
	var placed string
	a.DB.QueryRow("SELECT file_path FROM assets LIMIT 1").Scan(&placed)
	if placed != "Album artist/Test album/03 - Second song.flac" {
		t.Fatalf("unreadable placement: %s", placed)
	}

	a.runMigrationTask(context.Background())
	remote := nav.playlists[p.RemoteID]
	if len(remote.Entries) != 3 || remote.Entries[0].ID != "A" || remote.Entries[1].ID != "B" || remote.Entries[2].ID != "A" {
		t.Fatalf("lost source order: %+v", p.Entries)
	}
	// An external edit pauses this playlist, without overwriting it.
	remote.Name = "External title"
	nav.playlists[remote.ID] = remote
	before := nav.writes
	expect(t, alice.call("POST", prefix+"/publish", "", ""), 202)
	a.runMigrationTask(context.Background())
	pls, _ = a.playlists(mid)
	if pls[2].State != "paused" || nav.writes != before {
		t.Fatal("external edit overwritten")
	}
	expect(t, alice.call("POST", prefix+"/playlists/"+pid+"/resume", mustJSON(map[string]string{"baseline": pls[2].Desired}), ""), 200)
	expect(t, alice.call("POST", prefix+"/handover", "", ""), 200)
	a.queueMigration(func() string {
		var owner string
		a.DB.QueryRow("SELECT owner FROM migrations WHERE id=?", mid).Scan(&owner)
		return owner
	}(), mid, "publish")
	a.runMigrationTask(context.Background())
	if nav.writes != before {
		t.Fatal("write after handover")
	}
	// Retry-safe filesystem placement never creates another asset for a matching target.
	a.DB.QueryRow("SELECT count(*) FROM assets").Scan(&count)
	if count != 1 {
		t.Fatalf("assets: %d", count)
	}
}
func TestDeezerLinks(t *testing.T) {
	for _, in := range []string{"https://www.deezer.com/track/123?utm=test", "https://deezer.com/en/track/123", "deezer:track:123"} {
		got, e := deezerID(in)
		if e != nil || got != "123" {
			t.Fatalf("%s: %s %v", in, got, e)
		}
	}
	for _, in := range []string{"http://127.0.0.1/track/123", "https://deezer.com.evil/track/123", "https://deezer.com/album/123", "https://user:password@deezer.com/track/123"} {
		if _, e := deezerID(in); e == nil {
			t.Fatal("accepted", in)
		}
	}
}

func TestSingleServiceOwner(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	unlock, e := a.AcquireWorkerLock()
	if e != nil {
		t.Fatal(e)
	}
	if release, e := a.AcquireWorkerLock(); e == nil {
		release()
		t.Fatal("second service acquired lock")
	}
	unlock()
	release, e := a.AcquireWorkerLock()
	if e != nil {
		t.Fatal(e)
	}
	release()
}
func TestUncertainPlaylistRecoveryAndDeletedRemote(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.createAdmin("admin", "test-password-long")
	var owner string
	a.DB.QueryRow("SELECT id FROM users").Scan(&owner)
	nav := &navFixture{songs: map[string]Song{"A": {ID: "A"}}, playlists: map[string]RemotePlaylist{}, stars: map[string]map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(nav.handler))
	defer srv.Close()
	c := Credentials{URL: srv.URL, Username: "personal", Password: "fixture"}
	mid, pid := id(), id()
	a.DB.Exec("INSERT INTO migrations(id,owner,fingerprint,name,source,report,state) VALUES(?,?,'test','Test','','{}','open')", mid, owner)
	p := Playlist{ID: pid, Name: "Test", State: "creating", Selected: true}
	desired := playlistSnapshot{Name: "Test · ZenSync " + pid[:8], Owner: c.Username, IDs: []string{"A", "A"}}
	p.Desired = mustJSON(desired)
	nav.playlists["remote"] = RemotePlaylist{ID: "remote", Name: desired.Name, Owner: c.Username, Entries: []Song{{ID: "A"}, {ID: "A"}}}
	a.DB.Exec("INSERT INTO migration_playlists(id,migration_id,name,metadata,entries,selected,state,desired) VALUES(?,?,?,'{}','[]',1,'creating',?)", pid, mid, p.Name, p.Desired)
	if e = a.publishPlaylist(context.Background(), c, p, []string{"A", "A"}); e != nil {
		t.Fatal(e)
	}
	if nav.writes != 0 {
		t.Fatal("replayed uncertain create")
	}
	pls, _ := a.playlists(mid)
	if pls[0].RemoteID != "remote" {
		t.Fatal("did not adopt verified prior create")
	}
	delete(nav.playlists, "remote")
	if e = a.publishPlaylist(context.Background(), c, pls[0], []string{"A"}); e != nil {
		t.Fatal(e)
	}
	pls, _ = a.playlists(mid)
	if pls[0].State != "paused" || nav.writes != 0 {
		t.Fatal("recreated deleted playlist")
	}
}
func TestStageRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	if e := os.Symlink("/etc/passwd", filepath.Join(dir, "audio.flac")); e != nil {
		t.Fatal(e)
	}
	if _, e := stagedAudio(dir); e == nil {
		t.Fatal("accepted symlink output")
	}
}

func TestQualityFallbackAndStagedRecovery(t *testing.T) {
	ffmpeg, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("ffmpeg required")
	}
	if _, e = exec.LookPath("ffprobe"); e != nil {
		t.Skip("ffprobe required")
	}
	dir := t.TempDir()
	a, e := Open(filepath.Join(dir, "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.musicDir = filepath.Join(dir, "music")
	os.Mkdir(a.musicDir, 0755)
	audio := filepath.Join(dir, "fixture.mp3")
	out, e := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=duration=2", "-c:a", "libmp3lame", "-b:a", "320k", "-metadata", "title=Second song", "-metadata", "artist=Test artist", "-metadata", "album=Test album", "-metadata", "SOURCEID=2", audio).CombinedOutput()
	if e != nil {
		t.Fatalf("%s %v", out, e)
	}
	a.dfiPath = filepath.Join(dir, "dfi")
	os.WriteFile(a.dfiPath, []byte("#!/bin/sh\nfor arg in \"$@\"; do if [ \"$arg\" = flac ]; then exit 1; fi; done\ncp '"+audio+"' audio.mp3\n"), 0700)
	track := makeTrack(2, "Second song")
	if _, _, e = a.acquire(context.Background(), id(), "deezer:track:2", track, `["lossless"]`, "fixture-arl"); e == nil {
		t.Fatal("acquired excluded quality")
	}
	jid := id()
	rel, quality, e := a.acquire(context.Background(), jid, "deezer:track:2", track, `["lossless","320"]`, "fixture-arl")
	if e != nil {
		t.Fatal(e)
	}
	if quality != "320" {
		t.Fatal("actual quality not recorded")
	}
	info, e := os.Stat(filepath.Join(a.musicDir, rel))
	if e != nil || info.Size() == 0 {
		t.Fatal("no placed audio")
	}
	// Simulate a crash after placement and before scan: completed staging is reused,
	// even when the downloader would now fail. The destination is never replaced.
	os.WriteFile(a.dfiPath, []byte("#!/bin/sh\nexit 99\n"), 0700)
	again, quality, e := a.acquire(context.Background(), jid, "deezer:track:2", track, `["320"]`, "fixture-arl")
	if e != nil || again != rel || quality != "320" {
		t.Fatalf("recovery %s %s %v", again, quality, e)
	}
	after, _ := os.Stat(filepath.Join(a.musicDir, rel))
	if !os.SameFile(info, after) {
		t.Fatal("replaced existing audio during recovery")
	}
	if _, e = os.Stat(filepath.Join(a.dataDir, "staging", jid, "320", "d-fi.config.json")); !os.IsNotExist(e) {
		t.Fatal("credential file left after subprocess")
	}
}

type redirectTransport func(*http.Request) (*http.Response, error)

func (f redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDeezerShareLinks(t *testing.T) {
	a := &App{HTTP: &http.Client{Transport: redirectTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 301, Header: http.Header{"Location": {"https://link.deezer.com/?dest=https%3A%2F%2Fwww.deezer.com%2Ftrack%2F4081655051"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}}
	got, e := a.resolveDeezerID(context.Background(), "https://link.deezer.com/s/34QwcW1pZpQI0awlx3GK6")
	if e != nil || got != "4081655051" {
		t.Fatalf("share resolution %s %v", got, e)
	}
	for _, dest := range []string{"http://127.0.0.1/", "https://evil.example/track/1", "https://www.deezer.com/album/123"} {
		a.HTTP.Transport = redirectTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {dest}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})
		if _, e = a.resolveDeezerID(context.Background(), "https://link.deezer.com/s/test"); e == nil {
			t.Fatal("accepted unsafe or non-track redirect", dest)
		}
	}
}

func TestAlbumLinkSearch(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.createAdmin("admin", "test-password-long")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/album/42":
			send(w, 200, map[string]any{"id": 42, "title": "The album"})
		case "/album/42/tracks":
			send(w, 200, map[string]any{"data": []Track{makeTrack(1, "One"), makeTrack(2, "Two")}, "total": 2})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.deezerURL = server.URL
	c := login(t, a, "admin")
	w := c.call("GET", "/api/v2/search?q=https%3A%2F%2Fwww.deezer.com%2Falbum%2F42", "", "")
	expect(t, w, 200)
	var songs []Track
	json.Unmarshal(w.Body.Bytes(), &songs)
	if len(songs) != 2 || songs[0].Album.Title != "The album" {
		t.Fatalf("bad album tracks %s", w.Body)
	}
}

func TestReadablePathParts(t *testing.T) {
	for _, s := range []string{"../escape", "AC/DC", "Artist: Name", "..", ""} {
		part := pathPart(s, "Unknown")
		if !safeRelativePath(part) {
			t.Fatalf("unsafe part %q", part)
		}
	}
	for _, s := range []string{"../bad", "/root/file", "Artist/../file", `Artist\file`} {
		if safeRelativePath(s) {
			t.Fatal("accepted", s)
		}
	}
}

func TestRealNavidromeAssetVerification(t *testing.T) {
	base := os.Getenv("ZENSYNC_TEST_NAV_URL")
	if base == "" {
		t.Skip("run scripts/test-navidrome.py")
	}
	a, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.musicDir = os.Getenv("ZENSYNC_TEST_MUSIC")
	c := Credentials{URL: base, Username: "navadmin", Password: os.Getenv("ZENSYNC_TEST_NAV_PASSWORD")}
	track := makeTrack(1, "First song")
	id, e := a.findPlaced(context.Background(), c, "first.flac", mustJSON(track))
	if e != nil || id == "" {
		t.Fatalf("real virtual-path verification failed: %v", e)
	}
}
