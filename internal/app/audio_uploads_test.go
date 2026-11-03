package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func audioFixture(t *testing.T, directory, ext, codec string, tags bool) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe required")
	}
	path := filepath.Join(directory, "fixture"+ext)
	args := []string{"-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", codec}
	if tags {
		args = append(args, "-metadata", "title=My version", "-metadata", "artist=My artist", "-metadata", "album=My album")
	}
	args = append(args, path)
	if output, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("audio fixture: %v %s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func uploadAudioForTest(c client, path, name, key string, data []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", c.baseURL()+path, bytes.NewReader(data))
	request.Header.Set("Origin", c.baseURL())
	request.Header.Set("X-CSRF-Token", c.csrf)
	request.Header.Set("X-File-Name", url.QueryEscape(name))
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("Content-Type", "application/octet-stream")
	if c.cookie != nil {
		request.AddCookie(c.cookie)
	}
	response := httptest.NewRecorder()
	c.a.Handler().ServeHTTP(response, request)
	return response
}

func stageAudioMigration(t *testing.T, c client) (string, string) {
	t.Helper()
	xml, err := os.ReadFile("../apple/testdata/library.xml")
	if err != nil {
		t.Fatal(err)
	}
	response := c.call("POST", "/api/v2/imports", string(xml), "")
	expect(t, response, 201)
	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	sources, err := c.a.sources(result["id"])
	if err != nil {
		t.Fatal(err)
	}
	return result["id"], sources[0].ID
}

func TestUploadedAudioApprovalIndexingAndOwnVersion(t *testing.T) {
	a, admin, member := albumFixture(t)
	a.musicDir = t.TempDir()
	mid, sid := stageAudioMigration(t, member)
	prefix := "/api/v2/imports/" + mid
	expect(t, member.call("POST", prefix+"/commit", `{"playlists":[],"favorites":false}`, ""), 200)
	audio := audioFixture(t, t.TempDir(), ".flac", "flac", true)
	endpoint := prefix + "/songs/" + sid + "/upload"
	noCSRF := member
	noCSRF.csrf = ""
	expect(t, uploadAudioForTest(noCSRF, endpoint, "version.flac", "upload-test-key", audio), 403)
	expect(t, uploadAudioForTest(admin, endpoint, "version.flac", "upload-test-key", audio), 404)
	response := uploadAudioForTest(member, endpoint, "version.flac", "upload-test-key", audio)
	expect(t, response, 201)
	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	rid := result["id"]
	if result["decision"] != "awaiting_approval" {
		t.Fatal("member upload bypassed approval")
	}
	expect(t, uploadAudioForTest(member, endpoint, "version.flac", "upload-test-key", audio), 200)
	expect(t, uploadAudioForTest(member, endpoint, "version.flac", "another-upload-key", audio), 409)
	var jobs int
	if err := a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("job before approval: %d %v", jobs, err)
	}
	files, _ := os.ReadDir(a.musicDir)
	if len(files) != 0 {
		t.Fatal("unapproved upload entered the library")
	}
	// A later automatic scan must preserve the explicitly selected own version.
	var owner string
	a.DB.QueryRow("SELECT owner FROM migrations WHERE id=?", mid).Scan(&owner)
	if err := a.matchMigration(context.Background(), owner, mid); err == nil {
		t.Fatal("fixture has no Navidrome yet")
	}
	sources, _ := a.sources(mid)
	if sources[0].Candidate != nil || sources[0].MatchState != "manual" {
		t.Fatal("upload resolution changed")
	}
	// Existing audio at the natural destination must not be overwritten.
	original := filepath.Join(a.musicDir, "My artist", "My album", "My version.flac")
	if err := os.MkdirAll(filepath.Dir(original), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("existing audio must remain"), 0644); err != nil {
		t.Fatal(err)
	}
	indexed := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		out := map[string]any{"status": "ok"}
		switch strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/"), ".view") {
		case "getScanStatus":
			out["scanStatus"] = map[string]bool{"scanning": false}
		case "startScan":
			filepath.WalkDir(a.musicDir, func(path string, entry os.DirEntry, err error) error {
				if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".flac") && path != original {
					indexed["uploaded"] = path
				}
				return nil
			})
		case "search3":
			songs := []Song{}
			if path := indexed["uploaded"]; path != "" {
				rel, _ := filepath.Rel(a.musicDir, path)
				songs = append(songs, Song{ID: "uploaded", Title: "My version", Artist: "My artist", Album: "My album", Duration: 2, Path: filepath.ToSlash(rel)})
			}
			out["searchResult3"] = map[string]any{"song": songs}
		case "download":
			data, err := os.ReadFile(indexed[r.Form.Get("id")])
			if err != nil {
				http.Error(w, "missing", 404)
				return
			}
			w.Write(data)
			return
		default:
			out["status"] = "failed"
		}
		send(w, 200, map[string]any{"subsonic-response": out})
	}))
	defer server.Close()
	if err := a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	// Importing uploaded audio must never contact Deezer.
	a.deezerURL = "http://127.0.0.1:1"
	expect(t, admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`, ""), 200)
	a.runAcquisition(context.Background())
	var state, detail, target string
	if err := a.DB.QueryRow("SELECT j.state,j.stage,r.target FROM jobs j JOIN requests r ON r.id=j.request_id WHERE r.id=?", rid).Scan(&state, &detail, &target); err != nil {
		t.Fatal(err)
	}
	if state != "available" {
		t.Fatalf("upload did not finish: %s %s", state, detail)
	}
	sources, _ = a.sources(mid)
	if sources[0].ServerID != "uploaded" {
		t.Fatalf("uploaded file not linked: %+v", sources[0])
	}
	unchanged, _ := os.ReadFile(original)
	if string(unchanged) != "existing audio must remain" {
		t.Fatal("existing audio overwritten")
	}
	var rel, quality, metadata, stage string
	if err := a.DB.QueryRow("SELECT file_path,quality,metadata FROM assets WHERE target=?", target).Scan(&rel, &quality, &metadata); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rel, "[Upload ") || quality != "uploaded:flac" {
		t.Fatalf("bad upload asset %s %s", rel, quality)
	}
	if err := a.verifyPlaced(rel, metadata, quality); err != nil {
		t.Fatal(err)
	}
	a.DB.QueryRow("SELECT file_path FROM audio_uploads WHERE target=?", target).Scan(&stage)
	if stage != "" {
		t.Fatal("completed upload staging was retained")
	}
	// The validated journal survives a process restart.
	if err := a.DB.QueryRow("SELECT metadata FROM request_metadata WHERE request_id=?", rid).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var track Track
	json.Unmarshal([]byte(metadata), &track)
	if track.ID != 0 || track.Title != "My version" {
		t.Fatal("own-version metadata lost")
	}
}

func TestUploadAudioFormatsValidationAndLifecycle(t *testing.T) {
	for _, format := range []struct{ ext, codec string }{{".flac", "flac"}, {".mp3", "libmp3lame"}, {".m4a", "aac"}, {".ogg", "libopus"}, {".opus", "libopus"}, {".wav", "pcm_s16le"}, {".aiff", "pcm_s16be"}} {
		t.Run(format.ext, func(t *testing.T) {
			a, admin, member := albumFixture(t)
			mid, sid := stageAudioMigration(t, member)
			prefix := "/api/v2/imports/" + mid
			endpoint := prefix + "/songs/" + sid + "/upload"
			data := audioFixture(t, t.TempDir(), format.ext, format.codec, false)
			expect(t, uploadAudioForTest(member, endpoint, "audio"+format.ext, "staged-upload-key", data), 409)
			expect(t, member.call("POST", prefix+"/commit", `{"playlists":[]}`, ""), 200)
			expect(t, uploadAudioForTest(member, endpoint, "audio.exe", "invalid-upload-key", data), 400)
			expect(t, uploadAudioForTest(member, endpoint, "audio"+format.ext, "invalid-upload-key", []byte("not audio")), 400)
			response := uploadAudioForTest(member, endpoint, "audio"+format.ext, "valid-upload-key", data)
			expect(t, response, 201)
			var out map[string]string
			json.Unmarshal(response.Body.Bytes(), &out)
			var target, metadata, path string
			a.DB.QueryRow("SELECT target FROM requests WHERE id=?", out["id"]).Scan(&target)
			a.DB.QueryRow("SELECT metadata,file_path FROM audio_uploads WHERE target=?", target).Scan(&metadata, &path)
			var track Track
			json.Unmarshal([]byte(metadata), &track)
			if track.Title != "First song" || track.Artist.Name != "Test artist" || track.Album.Title != "Test album" {
				t.Fatalf("missing tags were not filled: %+v", track)
			}
			expect(t, admin.call("POST", "/api/v2/requests/"+out["id"], `{"decision":"declined","reason":"Not needed"}`, ""), 200)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("declined upload retained: %v", err)
			}
			expect(t, member.call("POST", prefix+"/handover", `{}`, ""), 200)
			expect(t, uploadAudioForTest(member, endpoint, "audio"+format.ext, "finished-upload-key", data), 409)
		})
	}
}
