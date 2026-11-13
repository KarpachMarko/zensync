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

func TestSongMergeFormatChangeWarnsBeforeQueueing(t *testing.T) {
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
	if !compatibility.Compatible || !compatibility.FormatChange || compatibility.SourceFormat != "flac" || compatibility.TargetFormat != "mp3" || !strings.Contains(compatibility.Warning, "native format") {
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

func TestSongMergeNativeFormatChangeAndRetry(t *testing.T) {
	for _, format := range []struct{ ext, codec, rate string }{{".mp3", "libmp3lame", "128k"}, {".m4a", "aac", "128k"}, {".ogg", "libvorbis", "128k"}, {".opus", "libopus", "128k"}, {".wav", "pcm_s16le", ""}, {".aiff", "pcm_s16be", ""}} {
		t.Run(format.ext, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixture(t)
			source, target := songs["two"], songs["one"]
			original := filepath.Join(a.musicDir, source.Path)
			selected := filepath.Join(a.musicDir, "donor"+format.ext)
			args := []string{"-v", "error", "-y", "-i", filepath.Join(a.musicDir, target.Path), "-c:a", format.codec}
			if format.rate != "" {
				args = append(args, "-b:a", format.rate)
			}
			args = append(args, selected)
			if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			target.Path = filepath.Base(selected)
			songs["one"] = target
			want, _ := metadataAudioHash(context.Background(), selected)
			response := admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","source":"two","target":"one","allowFormatChange":true}`, "")
			expect(t, response, 202)
			a.runMetadata(context.Background())
			var jid, state, message string
			a.DB.QueryRow("SELECT id,state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&jid, &state, &message)
			if state != "completed" {
				t.Fatalf("%s: %s", state, message)
			}
			newPath := strings.TrimSuffix(original, filepath.Ext(original)) + format.ext
			if _, err := os.Stat(original); !os.IsNotExist(err) {
				t.Fatal("old extension retained", err)
			}
			got, err := metadataAudioHash(context.Background(), newPath)
			if err != nil || got != want {
				t.Fatal("native audio differs", err)
			}
			p, err := a.probeUpload(newPath, uploadFormat(format.ext))
			if err != nil {
				t.Fatal(err)
			}
			tags := uploadTags(p)
			if tags["title"] != source.Title || tags["album"] != source.Album || tags["artist"] != source.Artist {
				t.Fatal(tags)
			}
			// The scanner can already have removed the old ID when this is retried.
			delete(songs, "two")
			if complete, err := a.reconcileFormatChange(jid, source.ID); err != nil || !complete {
				t.Fatal(complete, err)
			}
		})
	}
}

func TestSongMergeFormatCollisionKeepsOriginal(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	original := filepath.Join(a.musicDir, songs["two"].Path)
	before, _ := hashFile(original)
	donor := filepath.Join(a.musicDir, "one.mp3")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", filepath.Join(a.musicDir, songs["one"].Path), "-c:a", "libmp3lame", donor).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	target := songs["one"]
	target.Path = "one.mp3"
	songs["one"] = target
	collision := filepath.Join(a.musicDir, "two.mp3")
	os.WriteFile(collision, []byte("another file"), 0644)
	expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","source":"two","target":"one","allowFormatChange":true}`, ""), 202)
	a.runMetadata(context.Background())
	after, _ := hashFile(original)
	if before != after {
		t.Fatal("original changed on collision")
	}
	body, _ := os.ReadFile(collision)
	if string(body) != "another file" {
		t.Fatal("existing file overwritten")
	}
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message)
	if state != "failed" || !strings.Contains(message, "already exists") {
		t.Fatal(state, message)
	}
}

func TestSongMergeAcrossContainersRetainsArtworkAndNativeCredits(t *testing.T) {
	for _, format := range []struct{ ext, codec string }{{".flac", "flac"}, {".ogg", "libvorbis"}, {".wav", "pcm_s16le"}} {
		t.Run(format.ext, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixtureFormat(t, ".mp3", "libmp3lame", "128k")
			source := songs["two"]
			original := filepath.Join(a.musicDir, source.Path)
			writeNativeCredits(t, original, nativeCredits{[]string{"band", "Guest"}, []string{"band"}, []string{"artist-one", "guest-id"}, []string{"album-artist"}})
			script := `import base64,sys,mutagen
from mutagen.id3 import APIC
f=mutagen.File(sys.argv[1]);f.tags.add(APIC(encoding=3,mime="image/png",type=3,desc="Cover",data=base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")));f.save()`
			if out, err := exec.Command(metadataPythonPath(), "-c", script, original).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			selected := filepath.Join(a.musicDir, "donor"+format.ext)
			if out, err := exec.Command("ffmpeg", "-v", "error", "-i", filepath.Join(a.musicDir, songs["one"].Path), "-c:a", format.codec, selected).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			target := songs["one"]
			target.Path = filepath.Base(selected)
			songs["one"] = target
			credits := readNativeCredits(t, original)
			coverHash := func(path string) string {
				t.Helper()
				out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-map", "0:v:0", "-c:v", "copy", "-f", "hash", "-hash", "sha256", "-").CombinedOutput()
				if err != nil {
					t.Fatalf("cover %v %s", err, out)
				}
				return string(out)
			}
			cover := coverHash(original)
			expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","source":"two","target":"one","allowFormatChange":true}`, ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message)
			if state != "completed" {
				t.Fatal(state, message)
			}
			newPath := strings.TrimSuffix(original, ".mp3") + format.ext
			actual := readNativeCredits(t, newPath)
			if mustJSON(credits) != mustJSON(actual) {
				t.Fatalf("credits changed: %s / %s", mustJSON(credits), mustJSON(actual))
			}
			if got := coverHash(newPath); got != cover {
				t.Fatal("artwork changed")
			}
		})
	}
}
