package app

import (
	"context"
	"os/exec"
	"path/filepath"
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
