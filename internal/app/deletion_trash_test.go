package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func trashFixture(t *testing.T, a *App, n int, bytes int64, age time.Duration) string {
	t.Helper()
	rid := fmt.Sprintf("%064x", n)
	path := filepath.Join(a.dataDir, "trash", rid, "audio")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(bytes)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().Add(-age)
	if err = os.Chtimes(path, date, date); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestDeletionTrashRetentionAgeAndOldestFirstCapacity(t *testing.T) {
	f := newDeletionFixture(t)
	expired := trashFixture(t, f.a, 1, 1, 31*24*time.Hour)
	old := trashFixture(t, f.a, 2, 300_000_000, 20*24*time.Hour)
	recent := trashFixture(t, f.a, 3, 250_000_000, time.Hour)
	if err := f.a.pruneDeletionTrash(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, expired, false)
	assertFile(t, old, false)
	assertFile(t, recent, true)
	if err := f.a.reserveDeletionTrash(strings.Repeat("a", 64), 300_000_000); err != nil {
		t.Fatal(err)
	}
	assertFile(t, recent, false)
	if err := f.a.reserveDeletionTrash("", maxDeletionTrashBytes+1); err == nil {
		t.Fatal("oversized copy allowed")
	}
}
func restoreActivity(t *testing.T, c client, jid string) map[string]any {
	t.Helper()
	w := c.call("GET", "/api/v2/activity", "", "")
	expect(t, w, 200)
	var items []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item["id"] == jid {
			return item
		}
	}
	t.Fatal("deletion missing from Activity")
	return nil
}
func completedDeletion(t *testing.T, f *deletionFixture) (string, string) {
	t.Helper()
	rid := f.request(t, f.admin, "restore-request-key")
	f.a.runDeletion(context.Background())
	var jid, state string
	if err := f.a.DB.QueryRow("SELECT id,state FROM jobs WHERE request_id=?", rid).Scan(&jid, &state); err != nil || state != "completed" {
		t.Fatal(state, err)
	}
	return rid, jid
}
func TestRestoreDeletedFileAndActivityPermissions(t *testing.T) {
	f := newDeletionFixture(t)
	rid, jid := completedDeletion(t, f)
	endpoint := "/api/v2/tasks/" + jid + "/restore"
	if restoreActivity(t, f.admin, jid)["canRestore"] != true {
		t.Fatal("missing Restore action")
	}
	expect(t, f.member.call("POST", endpoint, "{}", ""), 403)
	expect(t, f.admin.call("POST", endpoint, "{}", ""), 200)
	data, err := os.ReadFile(f.path)
	if err != nil || string(data) != "original audio" {
		t.Fatal(string(data), err)
	}
	if restoreActivity(t, f.admin, jid)["canRestore"] != false {
		t.Fatal("Restore still offered")
	}
	expect(t, f.admin.call("POST", endpoint, "{}", ""), 200)
	assertFile(t, filepath.Join(f.a.dataDir, "trash", rid, "audio"), false)
	if f.scans.Load() != 2 {
		t.Fatal("restore replayed the scan or never requested it", f.scans.Load())
	}
}
func TestRestoreRefusesOccupiedPathAndExpiredCopy(t *testing.T) {
	f := newDeletionFixture(t)
	rid, jid := completedDeletion(t, f)
	endpoint := "/api/v2/tasks/" + jid + "/restore"
	if err := os.WriteFile(f.path, []byte("new recording"), 0644); err != nil {
		t.Fatal(err)
	}
	expect(t, f.admin.call("POST", endpoint, "{}", ""), 409)
	data, _ := os.ReadFile(f.path)
	if string(data) != "new recording" {
		t.Fatal("overwritten")
	}
	os.Remove(f.path)
	backup := filepath.Join(f.a.dataDir, "trash", rid, "audio")
	old := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(backup, old, old); err != nil {
		t.Fatal(err)
	}
	if restoreActivity(t, f.admin, jid)["canRestore"] != false {
		t.Fatal("expired Restore offered")
	}
	expect(t, f.admin.call("POST", endpoint, "{}", ""), 410)
	assertFile(t, f.path, false)
}
func TestRestoreRetriesScanWithoutOverwritingFile(t *testing.T) {
	f := newDeletionFixture(t)
	_, jid := completedDeletion(t, f)
	endpoint := "/api/v2/tasks/" + jid + "/restore"
	f.scanFail.Store(true)
	expect(t, f.admin.call("POST", endpoint, "{}", ""), 502)
	before, err := os.Stat(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if restoreActivity(t, f.admin, jid)["canRestore"] != true {
		t.Fatal("scan retry hidden")
	}
	f.scanFail.Store(false)
	expect(t, f.admin.call("POST", endpoint, "{}", ""), 200)
	after, _ := os.Stat(f.path)
	if !os.SameFile(before, after) {
		t.Fatal("restore recopied on scan retry")
	}
}
func TestRestoreDoesNotUndoPlaylistReplacements(t *testing.T) {
	f := newPreserveFixture(t)
	rid := f.request(t)
	f.a.runDeletion(context.Background())
	jid, state, stage := f.job(t, rid)
	if state != "completed" {
		t.Fatal(state, stage)
	}
	before := fmt.Sprint(f.ids)
	writes := len(f.writes)
	expect(t, f.admin.call("POST", "/api/v2/tasks/"+jid+"/restore", "{}", ""), 200)
	if fmt.Sprint(f.ids) != before || len(f.writes) != writes {
		t.Fatal("restore edited playlists")
	}
	assertFile(t, f.path, true)
}

func TestRestoreCleansInterruptedWorkingFile(t *testing.T) {
	f := newDeletionFixture(t)
	rid, jid := completedDeletion(t, f)
	c, err := f.a.shared()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.DB.Exec("INSERT INTO deletion_restores(request_id,server_url) VALUES(?,?)", rid, c.URL); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(filepath.Dir(f.path), ".zensync-restore-"+rid+filepath.Ext(f.path))
	if err = os.WriteFile(temp, []byte("partial copy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = f.a.cleanupDeletionRestoreTemps(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, temp, false)
	expect(t, f.admin.call("POST", "/api/v2/tasks/"+jid+"/restore", "{}", ""), 200)
	assertFile(t, f.path, true)
}
