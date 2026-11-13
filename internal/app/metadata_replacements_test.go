package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadedRecordingReplacementIsPrivateAndRetriedWithoutAcquisition(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	original := filepath.Join(a.musicDir, songs["two"].Path)
	before, _ := hashFile(original)
	data := nativeFixtureAudio(t, "libmp3lame", "128k", ".mp3")
	endpoint := "/api/v2/metadata/replacement-upload?source=two"
	expect(t, uploadAudioForTest(client{a: a}, endpoint, "song.mp3", "upload-recording", data), 401)
	expect(t, uploadAudioForTest(admin, endpoint, "song.mp3", "upload-recording", []byte("bad audio")), 400)
	expect(t, uploadAudioForTest(admin, endpoint, "song.exe", "upload-recording", data), 400)
	response := uploadAudioForTest(admin, endpoint, "song.mp3", "upload-recording", data)
	expect(t, response, 201)
	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	expect(t, admin.call("GET", "/api/v2/metadata/replacement-inputs/"+result.ID, "", ""), 200)
	expect(t, admin.call("GET", "/api/v2/metadata/replacement-inputs/"+result.ID+"/audio", "", ""), 200)
	var requests, assets int
	a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&requests)
	a.DB.QueryRow("SELECT count(*) FROM assets").Scan(&assets)
	if requests != 0 || assets != 0 {
		t.Fatal("staging created acquisition records")
	}
	after, _ := hashFile(original)
	if before != after {
		t.Fatal("upload changed original")
	}
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"other","password":"test-password-long"}`, ""), 201)
	_, err := a.DB.Exec("UPDATE users SET role='admin' WHERE username='other'")
	if err != nil {
		t.Fatal(err)
	}
	other := login(t, a, "other")
	expect(t, other.call("GET", "/api/v2/metadata/replacement-inputs/"+result.ID+"/audio", "", ""), 404)
	post := func(source string, allow bool) string {
		return mustJSON(map[string]any{"kind": "song", "source": source, "uploadId": result.ID, "allowFormatChange": allow})
	}
	expect(t, other.call("POST", "/api/v2/metadata/merge", post("two", true), ""), 404)
	expect(t, admin.call("POST", "/api/v2/metadata/merge", post("one", true), ""), 404)
	expect(t, admin.call("POST", "/api/v2/metadata/merge", post("two", false), ""), 409)
	expect(t, admin.call("POST", "/api/v2/metadata/merge", post("two", true), ""), 202)
	var staged string
	a.DB.QueryRow("SELECT path FROM metadata_audio_inputs WHERE id=?", result.ID).Scan(&staged)
	want, _ := metadataAudioHash(context.Background(), staged)
	// A failed write retains validated staging; retry can resume without reupload.
	oldPython := metadataPythonPath()
	bad := filepath.Join(t.TempDir(), "writer")
	os.WriteFile(bad, []byte("#!/bin/sh\nexit 1\n"), 0700)
	t.Setenv("ZENSYNC_PYTHON", bad)
	a.runMetadata(context.Background())
	var jid, state, message string
	a.DB.QueryRow("SELECT id,state,message FROM metadata_jobs").Scan(&jid, &state, &message)
	if state != "failed" {
		t.Fatal(state, message)
	}
	if actual, _ := hashFile(original); actual != before {
		t.Fatal("failed replacement changed original")
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("retry input removed", err)
	}
	t.Setenv("ZENSYNC_PYTHON", oldPython)
	expect(t, admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": jid}), ""), 200)
	a.runMetadata(context.Background())
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", jid).Scan(&state, &message)
	if state != "completed" {
		t.Fatal(state, message)
	}
	got, err := metadataAudioHash(context.Background(), strings.TrimSuffix(original, ".flac")+".mp3")
	if err != nil || want != got {
		t.Fatal("replacement bytes differ", err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatal("completed staging retained", err)
	}
}

func TestProviderRecordingReplacementUsesNativeDownloadWithoutPublishingDonor(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	original := filepath.Join(a.musicDir, songs["two"].Path)
	data := nativeFixtureAudio(t, "libmp3lame", "128k", ".mp3")
	downloads := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/track/42":
			io.WriteString(w, strings.ReplaceAll(losslessSearchFixture, `"duration":2`, `"duration":1`))
		case "/api/download/track":
			downloads++
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write(data)
		default:
			t.Errorf("Unexpected provider endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	target, _ := url.Parse(provider.URL)
	a.HTTP = &http.Client{Transport: losslessRoutingTransport{target}}
	response := admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","source":"two","providerTarget":"lossless:track:main:42","allowFormatChange":true}`, "")
	expect(t, response, 202)
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message)
	if state != "completed" {
		t.Fatal(state, message)
	}
	if downloads != 1 {
		t.Fatal("wrong download count", downloads)
	}
	var count int
	a.DB.QueryRow("SELECT count(*) FROM assets").Scan(&count)
	if count != 0 {
		t.Fatal("donor published as a separate asset")
	}
	a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&count)
	if count != 0 {
		t.Fatal("replacement created music request")
	}
	donor := filepath.Join(t.TempDir(), "expected.mp3")
	os.WriteFile(donor, data, 0600)
	want, _ := metadataAudioHash(context.Background(), donor)
	got, err := metadataAudioHash(context.Background(), strings.TrimSuffix(original, ".flac")+".mp3")
	if err != nil || want != got {
		t.Fatal("provider audio differs", err)
	}
}

func TestFormatChangeRecoversAfterNewFilePublished(t *testing.T) {
	for _, oldExists := range []bool{true, false} {
		t.Run(map[bool]string{true: "before removal", false: "after removal"}[oldExists], func(t *testing.T) {
			a, admin, songs := metadataLibraryFixture(t)
			expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"song","source":"two","target":"one"}`, ""), 202)
			var jid string
			a.DB.QueryRow("SELECT id FROM metadata_jobs").Scan(&jid)
			oldRel, newRel := songs["two"].Path, "two.mp3"
			oldPath, newPath := filepath.Join(a.musicDir, oldRel), filepath.Join(a.musicDir, newRel)
			before, _ := hashFile(oldPath)
			data := nativeFixtureAudio(t, "libmp3lame", "128k", ".mp3")
			os.WriteFile(newPath, data, 0644)
			after, _ := hashFile(newPath)
			a.DB.Exec("INSERT INTO metadata_files(job_id,song_id,file_path,before_hash,after_hash) VALUES(?,?,?,?,?)", jid, "two", oldRel, before, after)
			a.DB.Exec("INSERT INTO metadata_format_changes(job_id,song_id,old_path,new_path) VALUES(?,?,?,?)", jid, "two", oldRel, newRel)
			a.DB.Exec("INSERT INTO assets(target,file_path,quality,metadata,server_id) VALUES('deezer:track:2',?,'lossless','{}','two')", oldRel)
			if !oldExists {
				os.Remove(oldPath)
			}
			a.DB.Exec("UPDATE metadata_jobs SET state='failed' WHERE id=?", jid)
			expect(t, admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": jid}), ""), 200)
			complete, err := a.reconcileFormatChange(jid, "two")
			if err != nil || !complete {
				t.Fatal(complete, err)
			}
			if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
				t.Fatal("old path remains", err)
			}
			got, _ := os.ReadFile(newPath)
			if !bytes.Equal(data, got) {
				t.Fatal("recovery changed new audio")
			}
			var assetPath, quality, server string
			a.DB.QueryRow("SELECT file_path,quality,server_id FROM assets").Scan(&assetPath, &quality, &server)
			if assetPath != newRel || quality != "" || server != "" {
				t.Fatal(assetPath, quality, server)
			}
			complete, err = a.reconcileFormatChange(jid, "two")
			if err != nil || !complete {
				t.Fatal("recovery is not idempotent", err)
			}
		})
	}
}
