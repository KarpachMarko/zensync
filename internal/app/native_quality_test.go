package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func nativeFixtureAudio(t *testing.T, codec, rate, ext string) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	path := filepath.Join(t.TempDir(), "fixture"+ext)
	args := []string{"-v", "error", "-f", "lavfi", "-i", "anoisesrc=color=white:sample_rate=44100:duration=2", "-ac", "2", "-c:a", codec}
	if rate != "" {
		args = append(args, "-b:a", rate)
	}
	args = append(args, "-metadata", "title=Song", "-metadata", "artist=Artist", "-metadata", "album=Album", path)
	if output, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestLosslessRetainsBestAvailableOriginalEncoding(t *testing.T) {
	for _, test := range []struct{ name, codec, rate, ext, mime, quality string }{
		{"mp3-320", "libmp3lame", "320k", ".mp3", "audio/mpeg", "mp3:320"},
		{"mp3-192", "libmp3lame", "192k", ".mp3", "audio/mpeg", "mp3:192"},
		{"mp3-128", "libmp3lame", "128k", ".mp3", "application/octet-stream", "mp3:128"},
		{"aac", "aac", "256k", ".m4a", "audio/mp4", "aac:"},
		{"flac-mime-wrong", "libmp3lame", "192k", ".mp3", "audio/flac", "mp3:192"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := nativeFixtureAudio(t, test.codec, test.rate, test.ext)
			downloads := 0
			a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
				downloads++
				w.Header().Set("Content-Type", test.mime)
				w.Write(data)
			})
			a.musicDir = t.TempDir()
			track := makeTrack(42, "Song")
			track.Provider = "lossless"
			track.Source = "main"
			track.Duration = 2
			track.Artist.Name = "Artist"
			track.Album.Title = "Album"
			rel, quality, err := a.acquireProvider(context.Background(), "job", trackTarget(track), track, `["lossless","320","128"]`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(quality, test.quality) || !strings.HasSuffix(rel, test.ext) {
				t.Fatalf("source mislabeled: %s %s", quality, rel)
			}
			actual, err := os.ReadFile(filepath.Join(a.musicDir, rel))
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != string(data) {
				t.Fatal("original audio was transcoded or modified")
			}
			if err = a.validateAudio(filepath.Join(a.musicDir, rel), track, quality); err != nil {
				t.Fatal(err)
			}
			_, _, err = a.acquireProvider(context.Background(), "job", trackTarget(track), track, `["lossless","320","128"]`)
			if err != nil {
				t.Fatal(err)
			}
			if downloads != 1 {
				t.Fatal("validated staged audio downloaded again")
			}
			_, _, err = a.acquireProvider(context.Background(), "lossless-only", trackTarget(track), track, `["lossless"]`)
			if err == nil || !strings.Contains(err.Error(), "allowed quality profile") {
				t.Fatalf("lossy source bypassed profile: %v", err)
			}
		})
	}
}
func TestLosslessLossyDurationReviewPreservesActualQuality(t *testing.T) {
	data := nativeFixtureAudio(t, "libmp3lame", "192k", ".mp3")
	downloads := 0
	a := losslessFixture(t, func(w http.ResponseWriter, r *http.Request) {
		downloads++
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(data)
	})
	a.musicDir = t.TempDir()
	a.createAdmin("admin", "test-password-long")
	admin := login(t, a, "admin")
	var owner string
	a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&owner)
	track := makeTrack(42, "Song")
	track.Provider = "lossless"
	track.Source = "main"
	track.Duration = 8
	track.Artist.Name = "Artist"
	track.Album.Title = "Album"
	tx, err := a.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	rid, err := insertTrackRequest(tx, User{ID: owner, Role: "admin", Quality: json.RawMessage(`["lossless","320","128"]`)}, "native-duration", "", track, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var jid string
	a.DB.QueryRow("SELECT id FROM jobs WHERE request_id=?", rid).Scan(&jid)
	_, _, err = a.acquireProvider(context.Background(), jid, trackTarget(track), track, `["lossless","320","128"]`)
	var mismatch *audioDurationMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("duration mismatch lost: %v", err)
	}
	a.jobState(jid, "failed", err.Error())
	path := "/api/v2/requests/" + rid + "/review-audio"
	response := admin.call("GET", path, "", "")
	expect(t, response, 200)
	var review struct {
		Candidates []reviewAudioCandidate `json:"candidates"`
	}
	json.Unmarshal(response.Body.Bytes(), &review)
	if len(review.Candidates) != 1 || review.Candidates[0].Quality != "mp3:192" {
		t.Fatalf("retained quality lost: %s", response.Body)
	}
	expect(t, admin.call("POST", path, mustJSON(map[string]string{"quality": review.Candidates[0].Quality, "checksum": review.Candidates[0].Checksum}), ""), 200)
	_, actual, err := a.acquireProvider(context.Background(), jid, trackTarget(track), track, `["lossless","320","128"]`)
	if err != nil {
		t.Fatal(err)
	}
	if actual != "mp3:192" || downloads != 1 {
		t.Fatalf("accepted original not reused: %s %d", actual, downloads)
	}
}
