package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func consolidationFixture(t *testing.T) *preserveFixture {
	t.Helper()
	f := newPreserveFixture(t)
	original := filepath.Join(f.a.musicDir, "two.flac")
	selected := filepath.Join(f.a.musicDir, "two.mp3")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", original, "-c:a", "libmp3lame", "-b:a", "128k", selected).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	os.Remove(original)
	song := f.songs["two"]
	song.Path = "two.mp3"
	song.Suffix = "mp3"
	f.songs["two"] = song
	return f
}
func queueConsolidation(t *testing.T, f *preserveFixture, metadata string) string {
	t.Helper()
	body := `{"kind":"song","songOperation":"merge","source":"one","target":"two"`
	if metadata != "" {
		body += `,"metadata":` + metadata
	}
	body += "}"
	response := f.admin.call("POST", "/api/v2/metadata/merge", body, "")
	expect(t, response, 202)
	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	return result.ID
}
func assertConsolidationState(t *testing.T, f *preserveFixture, jid, state string) {
	t.Helper()
	var actual, message string
	f.a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", jid).Scan(&actual, &message)
	if actual != state {
		t.Fatal(actual, message)
	}
}
func TestLibrarySongMergeKeepsSelectedMP3AndRemovesOriginalFLAC(t *testing.T) {
	for _, metadata := range []string{"", `{"title":"Заедем в Азбуку (Explicit)","album":"Original album"}`} {
		t.Run(metadata, func(t *testing.T) {
			f := consolidationFixture(t)
			selected := filepath.Join(f.a.musicDir, "two.mp3")
			before, _ := hashFile(selected)
			audio, _ := metadataAudioHash(context.Background(), selected)
			jid := queueConsolidation(t, f, metadata)
			f.a.runMetadata(context.Background())
			assertConsolidationState(t, f, jid, "completed")
			files, _ := filepath.Glob(filepath.Join(f.a.musicDir, "*"))
			if len(files) != 1 || files[0] != selected {
				t.Fatal("merge left duplicate audio files", files)
			}
			after, _ := hashFile(selected)
			if metadata == "" && after != before {
				t.Fatal("unchanged selected recording was rewritten")
			}
			got, _ := metadataAudioHash(context.Background(), selected)
			if got != audio {
				t.Fatal("selected audio changed")
			}
			if metadata != "" {
				probe, err := f.a.probeUpload(selected, "mp3")
				if err != nil {
					t.Fatal(err)
				}
				tags := uploadTags(probe)
				if tags["title"] != "Заедем в Азбуку (Explicit)" || tags["album"] != "Original album" {
					t.Fatal(tags)
				}
			}
			if !slices.Equal(f.ids["service-list"], []string{"two", "two", "two", "two"}) || !slices.Equal(f.ids["personal-list"], []string{"two", "two", "two"}) {
				t.Fatal("playlist references lost", f.ids)
			}
			var rid string
			f.a.DB.QueryRow("SELECT request_id FROM deletion_replacements WHERE keep_song_id='two'").Scan(&rid)
			if _, err := f.a.deletionTrashPath(rid); err != nil {
				t.Fatal("original recovery copy missing", err)
			}
		})
	}
}
func TestLibrarySongMergeRetriesPlaylistFailureWithoutDuplicateCopies(t *testing.T) {
	f := consolidationFixture(t)
	jid := queueConsolidation(t, f, `{"title":"Reviewed title"}`)
	f.failWrite = true
	f.a.runMetadata(context.Background())
	assertConsolidationState(t, f, jid, "failed")
	assertFile(t, f.path, true)
	expect(t, f.admin.call("PUT", "/api/v2/metadata/songs/one", `{"title":"Other","artist":"Band","album":"Album"}`, ""), 409)
	expect(t, f.admin.call("POST", "/api/v2/deletion-requests", `{"songId":"two"}`, "overlapping-merge-deletion"), 409)
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
	f.failWrite = false
	expect(t, f.admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": jid}), ""), 200)
	f.a.runMetadata(context.Background())
	assertConsolidationState(t, f, jid, "completed")
	assertFile(t, f.path, false)
	files, _ := filepath.Glob(filepath.Join(f.a.musicDir, "*.mp3"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	if !slices.Equal(f.writes, []string{"service-list", "personal-list"}) {
		t.Fatal("uncertain playlist write repeated", f.writes)
	}
}
func TestLibrarySongMergeProtectsSelectedAndOriginalFromChangedFiles(t *testing.T) {
	for _, changed := range []string{"one.flac", "two.mp3"} {
		t.Run(changed, func(t *testing.T) {
			f := consolidationFixture(t)
			jid := queueConsolidation(t, f, "")
			path := filepath.Join(f.a.musicDir, changed)
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			file.WriteString("changed")
			file.Close()
			f.a.runMetadata(context.Background())
			assertConsolidationState(t, f, jid, "failed")
			assertFile(t, f.path, true)
			if changed == "two.mp3" && len(f.writes) != 0 {
				t.Fatal("changed selected recording entered playlists")
			}
		})
	}
}
func TestLibrarySongMergeValidatesMetadataAndPermissions(t *testing.T) {
	f := consolidationFixture(t)
	for _, metadata := range []string{`{"title":""}`, `{"artist":""}`, `{"album":"` + strings.Repeat("x", 501) + `"}`} {
		expect(t, f.admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","songOperation":"merge","source":"one","target":"two","metadata":`+metadata+`}`, ""), 400)
	}
	expect(t, f.admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","songOperation":"merge","source":"one","providerTarget":"deezer:track:1"}`, ""), 400)
	expect(t, client{a: f.a}.call("POST", "/api/v2/metadata/merge", `{"kind":"song","songOperation":"merge","source":"one","target":"two"}`, ""), 401)
	var jobs int
	f.a.DB.QueryRow("SELECT count(*) FROM metadata_jobs").Scan(&jobs)
	if jobs != 0 {
		t.Fatal("invalid merge queued work")
	}
}

func TestLibrarySongMergeCannotRemoveOriginalBeforeMetadataSucceeds(t *testing.T) {
	f := consolidationFixture(t)
	selected := filepath.Join(f.a.musicDir, "two.mp3")
	before, _ := hashFile(selected)
	jid := queueConsolidation(t, f, `{"title":"Reviewed title"}`)
	var removalJob string
	f.a.DB.QueryRow("SELECT j.id FROM jobs j JOIN deletion_replacements d ON d.request_id=j.request_id WHERE d.keep_song_id='two'").Scan(&removalJob)
	expect(t, f.admin.call("POST", "/api/v2/tasks/"+removalJob, "{}", ""), 409)
	python := metadataPythonPath()
	bad := filepath.Join(t.TempDir(), "fail-metadata")
	os.WriteFile(bad, []byte("#!/bin/sh\nexit 1\n"), 0700)
	t.Setenv("ZENSYNC_PYTHON", bad)
	f.a.runMetadata(context.Background())
	assertConsolidationState(t, f, jid, "failed")
	assertFile(t, f.path, true)
	after, _ := hashFile(selected)
	if after != before {
		t.Fatal("failed metadata changed the selected file")
	}
	if len(f.writes) != 0 {
		t.Fatal("playlists updated before metadata succeeded")
	}
	expect(t, f.admin.call("POST", "/api/v2/tasks/"+removalJob, "{}", ""), 409)
	t.Setenv("ZENSYNC_PYTHON", python)
	expect(t, f.admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": jid}), ""), 200)
	f.a.runMetadata(context.Background())
	assertConsolidationState(t, f, jid, "completed")
	assertFile(t, f.path, false)
}

func TestLibrarySongMergeConsolidatesExistingTwoMP3Copies(t *testing.T) {
	f := consolidationFixture(t)
	original := filepath.Join(f.a.musicDir, "one.mp3")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", f.path, "-c:a", "libmp3lame", "-b:a", "128k", original).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	os.Remove(f.path)
	f.path = original
	song := f.songs["one"]
	song.Path = "one.mp3"
	song.Suffix = "mp3"
	f.songs["one"] = song
	selected := filepath.Join(f.a.musicDir, "two.mp3")
	before, _ := hashFile(selected)
	jid := queueConsolidation(t, f, "")
	f.a.runMetadata(context.Background())
	assertConsolidationState(t, f, jid, "completed")
	files, _ := filepath.Glob(filepath.Join(f.a.musicDir, "*.mp3"))
	if len(files) != 1 || files[0] != selected {
		t.Fatal("MP3 duplicates remain", files)
	}
	after, _ := hashFile(selected)
	if before != after {
		t.Fatal("selected MP3 changed")
	}
}
