package app

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMetadataAudioStreamsOriginalWithSeekingAndPermissions(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	path := "/api/v2/metadata/songs/one/audio"
	expect(t, client{a: a}.call("GET", path, "", ""), 401)
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	expect(t, member.call("GET", path, "", ""), 403)
	original, err := os.ReadFile(filepath.Join(a.musicDir, songs["one"].Path))
	if err != nil {
		t.Fatal(err)
	}
	full := admin.call("GET", path, "", "")
	expect(t, full, 200)
	if !bytes.Equal(full.Body.Bytes(), original) || full.Header().Get("Content-Type") != "audio/flac" || full.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("original audio was not privately streamed")
	}
	req := httptest.NewRequest("GET", admin.baseURL()+path, nil)
	req.AddCookie(admin.cookie)
	req.Header.Set("Range", "bytes=4-15")
	part := httptest.NewRecorder()
	a.Handler().ServeHTTP(part, req)
	expect(t, part, 206)
	if !bytes.Equal(part.Body.Bytes(), original[4:16]) || part.Header().Get("Content-Range") == "" {
		t.Fatal("seeking failed")
	}
	head := admin.call("HEAD", path, "", "")
	expect(t, head, 200)
	if head.Body.Len() != 0 {
		t.Fatal("HEAD streamed a body")
	}
	expect(t, admin.call("GET", "/api/v2/metadata/songs/unknown/audio", "", ""), 404)
	if err := os.Remove(filepath.Join(a.musicDir, songs["one"].Path)); err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("GET", path, "", ""), 409)
}

func TestDuplicateCandidatesIncludeFileQuality(t *testing.T) {
	songs := []Song{{ID: "a", Title: "Song", Artist: "Band", Duration: 120, Size: 1048576, Suffix: "flac", ContentType: "audio/flac", BitRate: 900, BitDepth: 24, SamplingRate: 96000}, {ID: "b", Title: "Song", Artist: "Band", Duration: 120, Size: 500000, Suffix: "mp3", BitRate: 320}}
	for _, group := range metadataDuplicates(songs) {
		if group.Kind == "song" {
			for _, member := range group.Members {
				if member.Song == nil || member.Song.Size <= 0 || member.Song.Suffix == "" || member.Song.BitRate <= 0 {
					t.Fatal("comparison lost file metadata")
				}
			}
			return
		}
	}
	t.Fatal("missing duplicate group")
}
