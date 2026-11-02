package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func relocationFixture(t *testing.T) (*App, relocation, *navFixture) {
	t.Helper()
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
	t.Cleanup(func() { a.DB.Close() })
	a.createAdmin("admin", "test-password-long")
	a.musicDir = filepath.Join(dir, "music")
	os.MkdirAll(a.musicDir, 0755)
	temp := filepath.Join(dir, "source.flac")
	out, e := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=duration=2", "-metadata", "title=Second song", "-metadata", "artist=Test artist", "-metadata", "album=Test album", "-metadata", "track=2", "-metadata", "SOURCEID=2", temp).CombinedOutput()
	if e != nil {
		t.Fatalf("%s %v", out, e)
	}
	hash, _ := hashFile(temp)
	old := filepath.ToSlash(filepath.Join("ZenSync", "2", hash+".flac"))
	os.MkdirAll(filepath.Dir(filepath.Join(a.musicDir, old)), 0755)
	os.Rename(temp, filepath.Join(a.musicDir, old))
	track := makeTrack(2, "Second song")
	_, e = a.DB.Exec("INSERT INTO assets(target,file_path,quality,metadata,server_id) VALUES('deezer:track:2',?,'lossless',?,'B')", old, mustJSON(track))
	if e != nil {
		t.Fatal(e)
	}
	nav := &navFixture{songs: map[string]Song{"B": {ID: "B", Title: track.Title, Artist: track.Artist.Name, Album: track.Album.Title, Duration: 2, Path: old}}, playlists: map[string]RemotePlaylist{}, stars: map[string]map[string]bool{}, music: a.musicDir}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/ping.view") {
			send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "type": "navidrome", "serverVersion": "0.64.2"}})
			return
		}
		nav.handler(w, r)
	}))
	t.Cleanup(server.Close)
	if e = a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "fixture"}); e != nil {
		t.Fatal(e)
	}
	if e = a.scheduleRelocations(); e != nil {
		t.Fatal(e)
	}
	var r relocation
	e = a.DB.QueryRow("SELECT id,target,owner,old_path,state FROM asset_relocations").Scan(&r.id, &r.target, &r.owner, &r.old, &r.state)
	if e != nil {
		t.Fatal(e)
	}
	r.metadata = mustJSON(track)
	r.quality = "lossless"
	return a, r, nav
}
func TestLegacyRelocationAndJournalRecovery(t *testing.T) {
	a, r, nav := relocationFixture(t)
	now := time.Now().Unix()
	if e := a.processRelocation(context.Background(), &r, now); e != nil {
		t.Fatal(e)
	}
	if r.new != "Test artist/Test album/02 - Second song.flac" {
		t.Fatalf("path %s", r.new)
	}
	if _, e := os.Stat(filepath.Join(a.musicDir, r.old)); !os.IsNotExist(e) {
		t.Fatal("old hashed file remains")
	}
	var assetPath string
	a.DB.QueryRow("SELECT file_path FROM assets").Scan(&assetPath)
	if assetPath != r.new {
		t.Fatal("asset path not updated")
	}
	if _, e := os.Stat(filepath.Join(a.dataDir, "relocation-backups", r.id+".flac")); e != nil {
		t.Fatal("backup missing")
	}
	// Simulate death after atomic rename and before committing its database update.
	a.DB.Exec("UPDATE asset_relocations SET state='planned' WHERE id=?", r.id)
	a.DB.Exec("UPDATE assets SET file_path=?", r.old)
	r.state = "planned"
	if e := a.processRelocation(context.Background(), &r, now); e != nil {
		t.Fatal(e)
	}
	if e := a.processRelocation(context.Background(), &r, now+20); e != nil {
		t.Fatal(e)
	}
	var state string
	a.DB.QueryRow("SELECT state FROM asset_relocations WHERE id=?", r.id).Scan(&state)
	if state != "completed" {
		t.Fatal(state)
	}
	if nav.songs["B"].ID != "B" {
		t.Fatal("identity changed")
	}
	a.DB.QueryRow("SELECT server_id FROM assets").Scan(&state)
	if state != "B" {
		t.Fatal("mapping changed")
	}
	a.scheduleRelocations()
	var count int
	a.DB.QueryRow("SELECT count(*) FROM asset_relocations").Scan(&count)
	if count != 1 {
		t.Fatal("duplicated relocation")
	}
}
func TestRelocationNeverOverwritesDestination(t *testing.T) {
	a, r, _ := relocationFixture(t)
	r.new = "Artist/Album/Song.flac"
	r.hash = legacyAssetPath.FindStringSubmatch(r.old)[1]
	r.server = "B"
	r.state = "planned"
	a.ensureMusicDirs("Artist/Album")
	dest := filepath.Join(a.musicDir, r.new)
	os.WriteFile(dest, []byte("keep existing file"), 0644)
	if e := a.processRelocation(context.Background(), &r, time.Now().Unix()); e == nil {
		t.Fatal("overwrote collision")
	}
	contents, _ := os.ReadFile(dest)
	if string(contents) != "keep existing file" {
		t.Fatal("destination changed")
	}
	if _, e := os.Stat(filepath.Join(a.musicDir, r.old)); e != nil {
		t.Fatal("lost original")
	}
}
func TestRelocationAuthorizationAndRetry(t *testing.T) {
	a, r, _ := relocationFixture(t)
	admin := login(t, a, "admin")
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long","direct":false}`, ""), 201)
	member := login(t, a, "member")
	a.failRelocation(r, os.ErrPermission)
	expect(t, member.call("POST", "/api/v2/tasks/"+r.id, "", ""), 404)
	expect(t, admin.call("POST", "/api/v2/tasks/"+r.id, "", ""), 200)
	var state string
	a.DB.QueryRow("SELECT state FROM asset_relocations WHERE id=?", r.id).Scan(&state)
	if state != "queued" {
		t.Fatal("retry did not resume journal phase")
	}
}

func TestRealLegacyRelocation(t *testing.T) {
	base := os.Getenv("ZENSYNC_TEST_NAV_URL")
	if base == "" {
		t.Skip("run scripts/test-navidrome.py")
	}
	a, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.createAdmin("admin", "test-password-long")
	a.musicDir = os.Getenv("ZENSYNC_TEST_MUSIC")
	c := Credentials{URL: base, Username: "navadmin", Password: os.Getenv("ZENSYNC_TEST_NAV_PASSWORD")}
	a.seal("navidrome", c)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	track := makeTrack(1, "First song")
	serverID, e := a.findPlaced(ctx, c, "first.flac", mustJSON(track))
	if e != nil || serverID == "" {
		t.Fatal("original song missing", e)
	}
	playlist, e := a.nav(ctx, c, "createPlaylist", mapValues("name", "Relocation reference proof"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.nav(context.Background(), c, "deletePlaylist", mapValues("id", playlist.Playlist.ID))
	_, e = a.nav(ctx, c, "createPlaylist", url.Values{"playlistId": {playlist.Playlist.ID}, "songId": {serverID, serverID}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.nav(ctx, c, "star", mapValues("id", serverID)); e != nil {
		t.Fatal(e)
	}
	defer a.nav(context.Background(), c, "unstar", mapValues("id", serverID))
	source := filepath.Join(a.musicDir, "first.flac")
	hash, e := hashFile(source)
	if e != nil {
		t.Fatal(e)
	}
	legacy := filepath.ToSlash(filepath.Join("ZenSync", "1", hash+".flac"))
	a.ensureMusicDirs(filepath.Dir(legacy))
	if e = renameNoReplace(source, filepath.Join(a.musicDir, legacy)); e != nil {
		t.Fatal(e)
	}
	a.nav(ctx, c, "startScan", nil)
	wait := func(condition func() bool) {
		t.Helper()
		for ctx.Err() == nil {
			if condition() {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatal("Navidrome relocation verification timed out")
	}
	wait(func() bool { scan, e := a.nav(ctx, c, "getScanStatus", nil); return e == nil && !scan.Scan.Scanning })
	wait(func() bool {
		out, e := a.nav(ctx, c, "getSong", mapValues("id", serverID))
		if e != nil {
			return false
		}
		same, e := a.sameRemoteAudio(ctx, c, out.Song, legacy)
		return e == nil && same
	})
	if _, e = a.DB.Exec("INSERT INTO assets(target,file_path,quality,metadata,server_id) VALUES('deezer:track:1',?,'lossless',?,?)", legacy, mustJSON(track), serverID); e != nil {
		t.Fatal(e)
	}
	if e = a.scheduleRelocations(); e != nil {
		t.Fatal(e)
	}
	var r relocation
	e = a.DB.QueryRow("SELECT id,target,owner,old_path,state FROM asset_relocations").Scan(&r.id, &r.target, &r.owner, &r.old, &r.state)
	if e != nil {
		t.Fatal(e)
	}
	r.metadata = mustJSON(track)
	r.quality = "lossless"
	wait(func() bool {
		if e = a.processRelocation(ctx, &r, time.Now().Unix()); e != nil {
			t.Fatal(e)
		}
		var state string
		a.DB.QueryRow("SELECT state FROM asset_relocations WHERE id=?", r.id).Scan(&state)
		return state == "completed"
	})
	if _, e = os.Stat(filepath.Join(a.musicDir, legacy)); !os.IsNotExist(e) {
		t.Fatal("old hashed file still exists")
	}
	if _, e = os.Stat(filepath.Join(a.musicDir, "Test artist", "Test album", "First song.flac")); e != nil {
		t.Fatal("readable file missing", e)
	}
	song, e := a.nav(ctx, c, "getSong", mapValues("id", serverID))
	if e != nil || song.Song.ID != serverID || song.Song.Starred == "" {
		t.Fatal("song identity/favorite not preserved", e)
	}
	out, e := a.nav(ctx, c, "getPlaylist", mapValues("id", playlist.Playlist.ID))
	if e != nil || len(out.Playlist.Entries) != 2 || out.Playlist.Entries[0].ID != serverID || out.Playlist.Entries[1].ID != serverID {
		t.Fatal("duplicate playlist references not preserved", e)
	}
}

func TestRelocationRecoversInterruptedRollback(t *testing.T) {
	a, r, _ := relocationFixture(t)
	now := time.Now().Unix()
	if e := a.processRelocation(context.Background(), &r, now); e != nil {
		t.Fatal(e)
	}
	if e := renameNoReplace(filepath.Join(a.musicDir, r.new), filepath.Join(a.musicDir, r.old)); e != nil {
		t.Fatal(e)
	}
	if e := a.processRelocation(context.Background(), &r, now+20); e == nil {
		t.Fatal("expected restored-path recovery report")
	}
	var path string
	a.DB.QueryRow("SELECT file_path FROM assets").Scan(&path)
	if path != r.old || r.state != "queued" {
		t.Fatal("failed to reconcile restored original path")
	}
}
func TestRelocationRequiresSupportedServer(t *testing.T) {
	a, r, _ := relocationFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "type": "navidrome", "serverVersion": "0.63.2"}})
	}))
	defer server.Close()
	a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "fixture"})
	if e := a.processRelocation(context.Background(), &r, time.Now().Unix()); e == nil {
		t.Fatal("moved on unverified server version")
	}
	if _, e := os.Stat(filepath.Join(a.musicDir, r.old)); e != nil {
		t.Fatal("original path changed on unsupported server")
	}
}
