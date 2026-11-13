package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSongMergeReplacesAudioPreservingLocationAndAlbum(t *testing.T) {
	a, admin, songs := metadataLibraryFixtureFormat(t, ".flac", "flac", "")
	source, target := songs["two"], songs["one"]
	sourcePath, targetPath := filepath.Join(a.musicDir, source.Path), filepath.Join(a.musicDir, target.Path)
	// Different audio proves the operation uses the selected recording.
	cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=880:duration=1", "-c:a", "flac", "-metadata", "title="+target.Title, "-metadata", "artist="+target.Artist, "-metadata", "album="+target.Album, targetPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	want, err := metadataAudioHash(context.Background(), targetPath)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/merge", mustJSON(map[string]string{"kind": "song", "source": source.ID, "target": target.ID}), ""), 202)
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s: %s", state, message)
	}
	actual, err := metadataAudioHash(context.Background(), sourcePath)
	if err != nil || actual != want {
		t.Fatalf("Replacement audio mismatch: %v", err)
	}
	probe, err := a.probeUpload(sourcePath, "flac")
	if err != nil {
		t.Fatal(err)
	}
	tags := uploadTags(probe)
	if tags["album"] != source.Album || tags["title"] != source.Title {
		t.Fatalf("Location metadata lost: %+v", tags)
	}
}

func TestSongMergeIncompatibleFormatsAreReportedBeforeQueueing(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	source, target := songs["two"], songs["one"]
	old := filepath.Join(a.musicDir, target.Path)
	replacement := strings.TrimSuffix(old, filepath.Ext(old)) + ".mp3"
	output, err := exec.Command("ffmpeg", "-v", "error", "-y", "-i", old, "-c:a", "libmp3lame", "-b:a", "128k", replacement).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	target.Path = strings.TrimSuffix(target.Path, filepath.Ext(target.Path)) + ".mp3"
	// Suffix deliberately disagrees: actual local files determine compatibility.
	target.Suffix = "flac"
	songs["one"] = target
	before, _ := os.ReadFile(filepath.Join(a.musicDir, source.Path))
	preview := admin.call("GET", "/api/v2/metadata/merge-compatibility?source=two&target=one", "", "")
	expect(t, preview, 200)
	var compatibility mergeCompatibilityResult
	if err := json.Unmarshal(preview.Body.Bytes(), &compatibility); err != nil {
		t.Fatal(err)
	}
	if compatibility.Compatible || compatibility.SourceFormat != "flac" || compatibility.TargetFormat != "mp3" || !strings.Contains(compatibility.Warning, "matching file formats") {
		t.Fatal(preview.Body.String())
	}
	request := admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","source":"two","target":"one"}`, "")
	expect(t, request, 409)
	var jobs int
	a.DB.QueryRow("SELECT count(*) FROM metadata_jobs").Scan(&jobs)
	if jobs != 0 {
		t.Fatal("incompatible replacement created a job")
	}
	after, _ := os.ReadFile(filepath.Join(a.musicDir, source.Path))
	if !bytes.Equal(before, after) {
		t.Fatal("preflight changed original audio")
	}
	expect(t, client{a: a}.call("GET", "/api/v2/metadata/merge-compatibility?source=two&target=one", "", ""), 401)
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	expect(t, member.call("GET", "/api/v2/metadata/merge-compatibility?source=two&target=one", "", ""), 403)
}
