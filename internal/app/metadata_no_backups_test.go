package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataWriteFailureAndRetryWithoutBackups(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	known := songs["one"]
	known.Artists = []ArtistCredit{{ID: "vacio", Name: "Vacio"}, {ID: "lsp", Name: "ЛСП"}}
	songs["one"] = known
	song := songs["two"]
	song.Artist = "Vacio, ЛСП"
	song.Artists = []ArtistCredit{{ID: "combined", Name: song.Artist}}
	songs["two"] = song
	path := filepath.Join(a.musicDir, song.Path)
	writeNativeCredits(t, path, nativeCredits{[]string{song.Artist, "Guest"}, []string{"Band"}, []string{"combined", "guest"}, []string{"band"}})
	before, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", "", ""), 202)
	oldPython := os.Getenv("ZENSYNC_PYTHON")
	badPython := filepath.Join(t.TempDir(), "fail-writer")
	os.WriteFile(badPython, []byte("#!/bin/sh\nexit 1\n"), 0700)
	t.Setenv("ZENSYNC_PYTHON", badPython)
	a.runMetadata(context.Background())
	var jid, state string
	a.DB.QueryRow("SELECT id,state FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&jid, &state)
	if state != "failed" {
		t.Fatal(state)
	}
	after, err := hashFile(path)
	if err != nil || before != after {
		t.Fatal("Failed writer modified original", err)
	}
	assertNoMetadataCopies(t, a)
	t.Setenv("ZENSYNC_PYTHON", oldPython)
	expect(t, admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": jid}), ""), 200)
	a.runMetadata(context.Background())
	a.DB.QueryRow("SELECT state FROM metadata_jobs WHERE id=?", jid).Scan(&state)
	if state != "completed" {
		t.Fatal("Retry without recovery copy failed", state)
	}
	assertNoMetadataCopies(t, a)
}

func assertNoMetadataCopies(t *testing.T, a *App) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(a.dataDir, "metadata-backups")); !os.IsNotExist(err) {
		t.Fatal("Persistent backup directory created")
	}
	for _, pattern := range []string{".zensync-tags-*", ".zensync-cover-*"} {
		paths, _ := filepath.Glob(filepath.Join(a.musicDir, pattern))
		if len(paths) > 0 {
			t.Fatal("Working copies retained", paths)
		}
	}
}

func TestMetadataCrashTempCleanupOnlyRemovesIdentifiedWorkingFiles(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", `{"title":"Changed","artist":"Band","album":"Album"}`, ""), 202)
	var jid string
	a.DB.QueryRow("SELECT id FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&jid)
	key := jid + "-" + digest("one")
	paths := []string{".zensync-tags-" + key + "-123.flac", ".zensync-cover-" + key + "-456"}
	for _, name := range paths {
		os.WriteFile(filepath.Join(a.musicDir, name), []byte("partial output"), 0600)
	}
	keep := []string{"one.flac", ".zensync-tags-" + key + "-notes.flac", ".zensync-tags-" + strings.Repeat("a", 64) + "-" + digest("one") + "-123.flac"}
	for _, name := range keep[1:] {
		os.WriteFile(filepath.Join(a.musicDir, name), []byte("keep"), 0600)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("keep"), 0600)
	os.Symlink(outside, filepath.Join(a.musicDir, ".zensync-tags-"+key+"-789.flac"))
	if err := a.cleanupMetadataTemps(); err != nil {
		t.Fatal(err)
	}
	for _, name := range paths {
		if _, err := os.Stat(filepath.Join(a.musicDir, name)); !os.IsNotExist(err) {
			t.Fatal("Crash temporary file retained", name)
		}
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(a.musicDir, name)); err != nil {
			t.Fatal("Unrelated file removed", name)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("Symlink followed", err)
	}
}

func TestMetadataResumeBeforeRenameWithoutBackups(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	path := filepath.Join(a.musicDir, "one.flac")
	before, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", `{"title":"Changed","artist":"Band","album":"Album"}`, ""), 202)
	var jid string
	a.DB.QueryRow("SELECT id FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&jid)
	staged := filepath.Join(a.musicDir, ".zensync-tags-"+jid+"-"+digest("one")+"-123.flac")
	if output, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-c", "copy", "-metadata", "title=Changed", staged).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	after, err := hashFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec("INSERT INTO metadata_files(job_id,song_id,file_path,before_hash,after_hash) VALUES(?,'one','one.flac',?,?)", jid, before, after); err != nil {
		t.Fatal(err)
	}
	// Crash after journal persistence but before rename: original is intact,
	// and the abandoned candidate can be removed and rebuilt from that original.
	if err := a.cleanupMetadataTemps(); err != nil {
		t.Fatal(err)
	}
	current, err := hashFile(path)
	if err != nil || current != before {
		t.Fatal("Cleanup touched the original", err)
	}
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", jid).Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s %s", state, message)
	}
	probe, err := a.probeUpload(path, uploadFormat(".flac"))
	if err != nil || uploadTags(probe)["title"] != "Changed" {
		t.Fatal("Resumed edit not published", err)
	}
	assertNoMetadataCopies(t, a)
}
