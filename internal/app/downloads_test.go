package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDownloadSettingsAuthorizationAndPersistence(t *testing.T) {
	a, admin, member := albumFixture(t)
	if a.downloadLimit() != 3 {
		t.Fatal("default concurrency")
	}
	expect(t, member.call("PUT", "/api/v2/settings/downloads", `{"parallelDownloads":8}`, ""), 403)
	for _, value := range []string{"0", "-1", "1.5", "\"3\""} {
		expect(t, admin.call("PUT", "/api/v2/settings/downloads", `{"parallelDownloads":`+value+`}`, ""), 400)
	}
	expect(t, admin.call("PUT", "/api/v2/settings/downloads", `{"parallelDownloads":24}`, ""), 200)
	if a.downloadLimit() != 24 {
		t.Fatal("concurrency above eight was not accepted")
	}
	expect(t, admin.call("PUT", "/api/v2/settings/downloads", `{"parallelDownloads":2}`, ""), 200)
	if a.downloadLimit() != 2 {
		t.Fatal("limit not changed")
	}
	path := filepath.Join(a.dataDir, "db")
	other, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer other.DB.Close()
	if other.downloadLimit() != 2 {
		t.Fatal("limit not persisted")
	}
}
func TestParallelTransfersRespectLiveLimitAndTargetDeduplication(t *testing.T) {
	if _, e := exec.LookPath("ffprobe"); e != nil {
		t.Skip("ffprobe required")
	}
	dir := t.TempDir()
	a, e := Open(filepath.Join(dir, "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.createAdmin("admin", "test-password-long")
	admin := login(t, a, "admin")
	var owner string
	a.DB.QueryRow("SELECT id FROM users WHERE role='admin'").Scan(&owner)
	a.musicDir = filepath.Join(dir, "music")
	os.Mkdir(a.musicDir, 0755)
	a.metadataInterval = 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/track/") {
			n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/track/"))
			send(w, 200, makeTrack(n, "Song "+strconv.Itoa(n)))
			return
		}
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "searchResult3": map[string]any{"song": []Song{}}}})
	}))
	defer server.Close()
	a.deezerURL = server.URL
	a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "fixture"})
	a.seal("deezer", "fixture-cookie-not-a-real-secret")
	marker := filepath.Join(dir, "started")
	control := filepath.Join(dir, "gates")
	os.Mkdir(control, 0755)
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	a.dfiPath = filepath.Join(dir, "fake-d-fi")
	script := "#!/bin/sh\ntrack=''\nwhile [ $# -gt 0 ]; do if [ \"$1\" = --url ]; then shift; track=${1##*/}; fi; shift; done\nprintf '%s\\n' \"$track\" >> " + quote(marker) + "\nwhile [ ! -f " + quote(control) + "/\"$track\" ]; do sleep 0.05; done\nexit 1\n"
	os.WriteFile(a.dfiPath, []byte(script), 0700)
	enqueue := func(n int) {
		t.Helper()
		rid := id()
		track := makeTrack(n, "Song "+strconv.Itoa(n))
		_, e := a.DB.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision,reason) VALUES(?,?,?,?,?,?,0,'[\"lossless\"]','approved','Allowed without approval')", rid, owner, id(), fmt.Sprintf("deezer:track:%d", n), track.Title, track.Artist.Name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.DB.Exec("INSERT INTO request_metadata VALUES(?,?)", rid, mustJSON(track)); e != nil {
			t.Fatal(e)
		}
		if _, e = a.DB.Exec("INSERT INTO jobs(id,request_id,state,stage) VALUES(?,?,'queued','Queued for acquisition')", id(), rid); e != nil {
			t.Fatal(e)
		}
	}
	enqueue(1)
	enqueue(1)
	enqueue(2)
	enqueue(3)
	expect(t, admin.call("PUT", "/api/v2/settings/downloads", `{"parallelDownloads":2}`, ""), 200)
	ctx, cancel := context.WithCancel(context.Background())
	done := a.StartWorker(ctx)
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Error("workers failed to stop safely")
		}
	}()
	started := func() []string { b, _ := os.ReadFile(marker); return strings.Fields(string(b)) }
	wait := func(condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if condition() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("transfer condition timed out; starts=%v active=%d", started(), a.activeAcquisitions())
	}
	wait(func() bool { return len(started()) == 2 })
	first := started()
	if !((first[0] == "1" && first[1] == "2") || (first[0] == "2" && first[1] == "1")) {
		t.Fatalf("same target started twice or limit exceeded: %v", first)
	}
	if a.activeAcquisitions() != 2 {
		t.Fatal("downloads did not overlap")
	}
	expect(t, admin.call("PUT", "/api/v2/settings/downloads", `{"parallelDownloads":1}`, ""), 200)
	if a.activeAcquisitions() != 2 {
		t.Fatal("limit reduction cancelled transfers")
	}
	os.WriteFile(filepath.Join(control, "2"), nil, 0600)
	wait(func() bool { return a.activeAcquisitions() == 1 })
	time.Sleep(1100 * time.Millisecond)
	if len(started()) != 2 {
		t.Fatal("new transfer started above reduced limit")
	}
	os.WriteFile(filepath.Join(control, "1"), nil, 0600)
	wait(func() bool { return strings.Contains(strings.Join(started(), ","), "3") })
	enqueue(4)
	expect(t, admin.call("PUT", "/api/v2/settings/downloads", `{"parallelDownloads":2}`, ""), 200)
	wait(func() bool { return strings.Contains(strings.Join(started(), ","), "4") })
	if a.activeAcquisitions() != 2 {
		t.Fatal("increasing limit did not admit another transfer")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("shutdown did not cancel and join active downloaders")
	}
	if a.activeAcquisitions() != 0 {
		t.Fatal("claims remain after workers stopped")
	}
}

func TestParallelCompletionRequeuesRunningPublication(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	a.createAdmin("admin", "test-password-long")
	var owner string
	a.DB.QueryRow("SELECT id FROM users").Scan(&owner)
	mid := id()
	a.DB.Exec("INSERT INTO migrations(id,owner,fingerprint,name,source,report,state) VALUES(?,?,'fixture','Fixture','','{}','open')", mid, owner)
	tid, e := a.queueMigration(owner, mid, "publish")
	if e != nil {
		t.Fatal(e)
	}
	a.DB.Exec("UPDATE migration_tasks SET state='running' WHERE id=?", tid)
	for range 3 {
		if _, e = a.queueMigration(owner, mid, "publish"); e != nil {
			t.Fatal(e)
		}
	}
	if e = a.finishMigrationTask(tid, owner, mid, "publish", "completed", "Finished"); e != nil {
		t.Fatal(e)
	}
	var queued int
	a.DB.QueryRow("SELECT count(*) FROM migration_tasks WHERE migration_id=? AND state='queued'", mid).Scan(&queued)
	if queued != 1 {
		t.Fatal("late completions lost or duplicated publication")
	}
	a.DB.Exec("UPDATE migration_tasks SET state='running' WHERE migration_id=? AND state='queued'", mid)
	var next string
	a.DB.QueryRow("SELECT id FROM migration_tasks WHERE migration_id=? AND state='running'", mid).Scan(&next)
	a.queueMigration(owner, mid, "publish")
	a.DB.Exec("UPDATE migrations SET state='handed_over' WHERE id=?", mid)
	if e = a.finishMigrationTask(next, owner, mid, "publish", "completed", "Finished"); e != nil {
		t.Fatal(e)
	}
	a.DB.QueryRow("SELECT count(*) FROM migration_tasks WHERE migration_id=? AND state='queued'", mid).Scan(&queued)
	if queued != 0 {
		t.Fatal("handover allowed queued publication")
	}
}

func TestAcquisitionClaimsGiveOtherUsersAFreeSlot(t *testing.T) {
	a, _, _ := albumFixture(t)
	var admin, member string
	a.DB.QueryRow("SELECT id FROM users WHERE role='admin'").Scan(&admin)
	a.DB.QueryRow("SELECT id FROM users WHERE username='member'").Scan(&member)
	add := func(owner string, n int) {
		rid := id()
		if _, e := a.DB.Exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) VALUES(?,?,?,?,?,'Artist',0,'[\"lossless\"]','approved')", rid, owner, id(), fmt.Sprintf("deezer:track:%d", n), "Song"); e != nil {
			t.Fatal(e)
		}
		if _, e := a.DB.Exec("INSERT INTO jobs(id,request_id) VALUES(?,?)", id(), rid); e != nil {
			t.Fatal(e)
		}
	}
	add(admin, 1)
	add(admin, 2)
	add(admin, 3)
	add(member, 4)
	first := a.claimAcquisition()
	if first == nil || first.owner != admin {
		t.Fatal("first claim should honor oldest work")
	}
	defer a.releaseAcquisition(first)
	second := a.claimAcquisition()
	if second == nil || second.owner != member {
		t.Fatal("large owner queue monopolized next free slot")
	}
	a.releaseAcquisition(second)
}
