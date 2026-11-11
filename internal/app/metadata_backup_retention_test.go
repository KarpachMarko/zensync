package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetadataBackupRetentionAndBudget(t *testing.T) {
	a, _, songs := metadataLibraryFixture(t)
	var actor string
	a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&actor)
	root := filepath.Join(a.dataDir, "metadata-backups")
	create := func(state string, done int, size int, age time.Duration) string {
		t.Helper()
		jid := id()
		song := songs["one"]
		_, err := a.DB.Exec("INSERT INTO metadata_jobs(id,actor,changes,server_url,state,done) VALUES(?,?,?,'server',?,?)", jid, actor, mustJSON([]metadataChange{{Song: song}}), state, done)
		if err != nil {
			t.Fatal(err)
		}
		key := jid + "-" + digest(song.ID)
		dir := filepath.Join(root, key)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, "original.flac")
		if err := os.WriteFile(file, []byte(strings.Repeat("a", size)), 0600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Now().Add(-age)
		os.Chtimes(file, stamp, stamp)
		os.Chtimes(dir, stamp, stamp)
		return dir
	}
	exists := func(path string, want bool) {
		t.Helper()
		_, err := os.Stat(path)
		if (err == nil) != want {
			t.Fatalf("%s exists=%v, wanted %v", path, err == nil, want)
		}
	}
	old := create("completed", 1, 6, 3*time.Hour)
	newest := create("completed", 1, 6, time.Hour)
	pending := create("failed", 0, 6, 20*24*time.Hour)
	removed, err := a.cleanupMetadataBackups(12, 0, time.Now())
	if err != nil || len(removed) != 1 {
		t.Fatalf("%v %v", removed, err)
	}
	exists(old, false)
	exists(newest, true)
	exists(pending, true)
	// Protected work is never evicted to satisfy a new allocation.
	_, err = a.cleanupMetadataBackups(6, 1, time.Now())
	if err == nil || !strings.Contains(err.Error(), "limit reached") {
		t.Fatal(err)
	}
	exists(newest, false)
	exists(pending, true)
	// Successfully processed files in a partially failed batch are eligible.
	processed := create("failed", 1, 6, 8*24*time.Hour)
	_, err = a.cleanupMetadataBackups(100, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	exists(processed, false)
	exists(pending, true)
	// Unknown directories are protected and counted in the budget too.
	unknown := filepath.Join(root, "operator-files")
	os.MkdirAll(unknown, 0700)
	os.WriteFile(filepath.Join(unknown, "keep"), []byte("123456"), 0600)
	_, err = a.cleanupMetadataBackups(6, 0, time.Now())
	if err == nil {
		t.Fatal("Unrecognized storage was not counted")
	}
	exists(unknown, true)
}

func TestMetadataBackupCleanupRejectsSymlinks(t *testing.T) {
	a, _, _ := metadataLibraryFixture(t)
	outside := t.TempDir()
	file := filepath.Join(outside, "keep")
	os.WriteFile(file, []byte("original"), 0600)
	root := filepath.Join(a.dataDir, "metadata-backups")
	os.MkdirAll(root, 0700)
	os.Symlink(outside, filepath.Join(root, "linked"))
	if _, err := a.CleanupMetadataBackups(); err == nil {
		t.Fatal("Symlink accepted")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("Outside file removed")
	}
}

func TestMetadataRecoveryBudgetBlocksBeforeChangingMusic(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	t.Setenv("ZENSYNC_METADATA_BACKUP_LIMIT_MIB", "1")
	path := filepath.Join(a.musicDir, "one.flac")
	before, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(a.dataDir, "metadata-backups", "unrecognized")
	os.MkdirAll(root, 0700)
	if err := os.WriteFile(filepath.Join(root, "protected"), make([]byte, 1<<20), 0600); err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", `{"title":"New title","artist":"Band","album":"Album"}`, ""), 202)
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "failed" || !strings.Contains(message, "limit reached") {
		t.Fatalf("%s %s", state, message)
	}
	after, err := hashFile(path)
	if err != nil || before != after {
		t.Fatal("Music modified after budget refusal", err)
	}
	files, _ := filepath.Glob(filepath.Join(a.dataDir, "metadata-backups", "*", "original.flac"))
	if len(files) != 0 {
		t.Fatal("Allocated a backup past limit")
	}
}

func TestPrunedPublishedRecoveryCanStillResume(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", `{"title":"New title","artist":"Band","album":"Album"}`, ""), 202)
	a.runMetadata(context.Background())
	var jid, state string
	a.DB.QueryRow("SELECT id,state FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&jid, &state)
	if state != "completed" {
		t.Fatal(state)
	}
	entries, err := a.MetadataBackupInventory()
	if err != nil || len(entries) != 1 || !entries[0].Reclaimable || entries[0].MusicPath != "one.flac" || entries[0].BeforeHash == "" {
		t.Fatalf("%+v %v", entries, err)
	}
	if _, err := a.cleanupMetadataBackups(0, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	a.DB.Exec("UPDATE metadata_jobs SET state='running',done=0 WHERE id=?", jid)
	a.runMetadata(context.Background())
	a.DB.QueryRow("SELECT state FROM metadata_jobs WHERE id=?", jid).Scan(&state)
	if state != "completed" {
		t.Fatal("Published file could not resume without old recovery copy", state)
	}
}
