package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func durationReviewFixture(t *testing.T) (*App, client, client, Track, string) {
	t.Helper()
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg required")
	}
	if _, e := exec.LookPath("ffprobe"); e != nil {
		t.Skip("ffprobe required")
	}
	a, admin, member := albumFixture(t)
	a.musicDir = t.TempDir()
	track := makeTrack(79906978, "Waves (feat. Lantsberg) (Original Mix)")
	track.Duration = 324
	track.ISRC = "USLZJ1417478"
	track.Artist.Name = "Ganju"
	for _, query := range []string{
		`INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) SELECT 'duration-request',id,'duration','deezer:track:79906978','Waves','Ganju',0,'["lossless"]','approved' FROM users WHERE username='admin'`,
		`INSERT INTO jobs(id,request_id,state,stage) VALUES('duration-job','duration-request','failed','Downloaded duration differs')`,
	} {
		if _, e := a.DB.Exec(query); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := a.DB.Exec("INSERT INTO request_metadata(request_id,metadata) VALUES('duration-request',?)", mustJSON(track)); e != nil {
		t.Fatal(e)
	}
	stage := filepath.Join(a.dataDir, "staging", "duration-job", "lossless")
	if e := os.MkdirAll(stage, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(stage, "retained.flac")
	out, e := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=204.069", "-c:a", "flac", "-metadata", "SOURCEID=79906978", "-metadata", "ISRC="+track.ISRC, "-metadata", "title="+track.Title, "-metadata", "artist="+track.Artist.Name, "-metadata", "album="+track.Album.Title, path).CombinedOutput()
	if e != nil {
		t.Fatalf("fixture %s %v", out, e)
	}
	return a, admin, member, track, path
}
func reviewCandidate(t *testing.T, c client) reviewAudioCandidate {
	t.Helper()
	w := c.call("GET", "/api/v2/tasks/duration-job/review-audio", "", "")
	expect(t, w, 200)
	var result struct {
		Candidates []reviewAudioCandidate `json:"candidates"`
		ReviewURL  string                 `json:"reviewURL"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if len(result.Candidates) != 1 || !strings.HasPrefix(result.ReviewURL, "/search?q=") {
		t.Fatalf("review: %s", w.Body)
	}
	return result.Candidates[0]
}
func TestDurationReviewApprovalReusesOnlyExplicitlyAcceptedBytes(t *testing.T) {
	a, admin, member, track, path := durationReviewFixture(t)
	var mismatch *audioDurationMismatch
	if e := a.validateAudio(path, track, "lossless"); !errors.As(e, &mismatch) {
		t.Fatalf("mismatch: %v", e)
	}
	candidate := reviewCandidate(t, admin)
	if candidate.Expected != 324 || candidate.Measured < 204 || candidate.Measured > 205 {
		t.Fatalf("durations %+v", candidate)
	}
	body := mustJSON(map[string]string{"quality": "lossless", "checksum": candidate.Checksum})
	expect(t, member.call("GET", "/api/v2/tasks/duration-job/review-audio", "", ""), 404)
	expect(t, member.call("POST", "/api/v2/tasks/duration-job/review-audio", body, ""), 404)
	expect(t, admin.call("POST", "/api/v2/tasks/duration-job/review-audio", `{"quality":"lossless","checksum":"wrong"}`, ""), 409)
	audio := admin.call("GET", "/api/v2/tasks/duration-job/review-audio/file?quality=lossless&checksum="+candidate.Checksum, "", "")
	expect(t, audio, 200)
	if audio.Body.Len() < 1024 || audio.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("original audio not served privately")
	}
	expect(t, admin.call("POST", "/api/v2/tasks/duration-job/review-audio", body, ""), 200)
	if e := a.validateAudio(path, track, "lossless"); e != nil {
		t.Fatalf("approved duration rejected: %v", e)
	}
	changed := track
	changed.ID++
	if e := a.validateAudio(path, changed, "lossless"); e == nil {
		t.Fatal("approval bypassed provider identity")
	}
	var actor, checksum, state string
	if e := a.DB.QueryRow("SELECT actor,checksum FROM audio_duration_approvals").Scan(&actor, &checksum); e != nil {
		t.Fatal(e)
	}
	if checksum != candidate.Checksum || actor == "" {
		t.Fatal("missing approval audit")
	}
	if e := a.DB.QueryRow("SELECT state FROM jobs WHERE id='duration-job'").Scan(&state); e != nil || state != "queued" {
		t.Fatalf("not queued %s %v", state, e)
	}
	// Recovery must use the retained file; the downloader is present but fails
	// if invoked, and no provider access is needed to place the accepted bytes.
	a.dfiPath = filepath.Join(t.TempDir(), "must-not-run")
	if e := os.WriteFile(a.dfiPath, []byte("#!/bin/sh\nexit 99\n"), 0700); e != nil {
		t.Fatal(e)
	}
	rel, quality, e := a.acquire(context.Background(), "duration-job", "deezer:track:79906978", track, `["lossless"]`, "unused")
	if e != nil || quality != "lossless" || rel == "" {
		t.Fatalf("retained recovery: %s %s %v", rel, quality, e)
	}
	if e := a.verifyPlaced(rel, mustJSON(track), quality); e != nil {
		t.Fatal(e)
	}
	// Even a byte appended after approval must invalidate the exception.
	f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte("changed after approval"))
	f.Close()
	if e := a.validateAudio(path, track, "lossless"); e == nil {
		t.Fatal("approval applied to changed bytes")
	}
}
func TestDurationReviewCannotBypassRequestApprovalOrOtherFailures(t *testing.T) {
	a, admin, _, track, path := durationReviewFixture(t)
	candidate := reviewCandidate(t, admin)
	body := mustJSON(map[string]string{"quality": candidate.Quality, "checksum": candidate.Checksum})
	if _, e := a.DB.Exec("UPDATE requests SET decision='awaiting_approval'"); e != nil {
		t.Fatal(e)
	}
	expect(t, admin.call("POST", "/api/v2/requests/duration-request/review-audio", body, ""), 409)
	if _, e := a.DB.Exec("UPDATE requests SET decision='approved'"); e != nil {
		t.Fatal(e)
	}
	wrong := track
	wrong.ISRC = "DIFFERENT"
	if _, e := a.DB.Exec("UPDATE request_metadata SET metadata=?", mustJSON(wrong)); e != nil {
		t.Fatal(e)
	}
	w := admin.call("GET", "/api/v2/requests/duration-request/review-audio", "", "")
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), `"candidates":[]`) {
		t.Fatal("identity mismatch was approvable", w.Body)
	}
	expect(t, admin.call("POST", "/api/v2/requests/duration-request/review-audio", body, ""), 409)
	if e := os.WriteFile(path, []byte("not audio"), 0600); e != nil {
		t.Fatal(e)
	}
	w = admin.call("GET", "/api/v2/tasks/duration-job/review-audio", "", "")
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), `"candidates":[]`) {
		t.Fatal("corrupt file was approvable")
	}
}

func TestDurationAcceptanceSurvivesRestartAndPrioritizesChosenQuality(t *testing.T) {
	a, admin, _, track, path := durationReviewFixture(t)
	mp3dir := filepath.Join(a.dataDir, "staging", "duration-job", "320")
	if e := os.MkdirAll(mp3dir, 0700); e != nil {
		t.Fatal(e)
	}
	mp3 := filepath.Join(mp3dir, "retained.mp3")
	if out, e := exec.Command("ffmpeg", "-v", "error", "-i", path, "-c:a", "libmp3lame", "-b:a", "320k", mp3).CombinedOutput(); e != nil {
		t.Fatalf("mp3 fixture %s %v", out, e)
	}
	if _, e := a.DB.Exec(`UPDATE requests SET quality='["lossless","320"]'`); e != nil {
		t.Fatal(e)
	}
	w := admin.call("GET", "/api/v2/tasks/duration-job/review-audio", "", "")
	expect(t, w, 200)
	var review struct {
		Candidates []reviewAudioCandidate `json:"candidates"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &review); e != nil {
		t.Fatal(e)
	}
	var selected reviewAudioCandidate
	for _, c := range review.Candidates {
		if c.Quality == "320" {
			selected = c
		}
	}
	if selected.Checksum == "" {
		t.Fatal("MP3 was not reviewable", w.Body)
	}
	expect(t, admin.call("POST", "/api/v2/tasks/duration-job/review-audio", mustJSON(map[string]string{"quality": "320", "checksum": selected.Checksum}), ""), 200)
	var sequence int
	var name, dbpath string
	if e := a.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &dbpath); e != nil {
		t.Fatal(e)
	}
	if e := a.DB.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(dbpath)
	if e != nil {
		t.Fatal(e)
	}
	a.DB = reopened.DB // albumFixture's cleanup closes the reopened connection.
	a.dfiPath = filepath.Join(t.TempDir(), "downloader")
	marker := filepath.Join(t.TempDir(), "downloader-was-called")
	// The marker path is generated by the test and contains no shell characters.
	if e := os.WriteFile(a.dfiPath, []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 99\n"), 0700); e != nil {
		t.Fatal(e)
	}
	rel, quality, e := a.acquire(context.Background(), "duration-job", "deezer:track:79906978", track, `["lossless","320"]`, "unused")
	if e != nil || quality != "320" || rel == "" {
		t.Fatalf("chosen quality recovery: %s %s %v", rel, quality, e)
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("accepting 320 restarted a lossless download")
	}
}
