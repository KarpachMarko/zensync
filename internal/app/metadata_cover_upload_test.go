package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCoverUploadThenRemovePreservesSong(t *testing.T) {
	for _, format := range []struct{ ext, codec, rate string }{{".flac", "flac", ""}, {".mp3", "mp3", "192k"}, {".m4a", "aac", "192k"}} {
		t.Run(format.ext, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixtureFormat(t, format.ext, format.codec, format.rate)
			var picture bytes.Buffer
			newCover := image.NewRGBA(image.Rect(0, 0, 64, 64))
			draw.Draw(newCover, newCover.Bounds(), image.NewUniform(color.RGBA{B: 255, A: 255}), image.Point{}, draw.Src)
			png.Encode(&picture, newCover)
			var actor string
			a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&actor)
			// The normal client helper sends JSON strings; make a real raw upload request.
			call := func(data []byte) *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", admin.baseURL()+"/api/v2/metadata/songs/one/cover-upload", bytes.NewReader(data))
				req.Header.Set("Origin", admin.baseURL())
				req.Header.Set("X-CSRF-Token", admin.csrf)
				if admin.cookie != nil {
					req.AddCookie(admin.cookie)
				}
				w := httptest.NewRecorder()
				a.Handler().ServeHTTP(w, req)
				return w
			}
			expect(t, call([]byte("not an image")), 400)
			uploaded := call(picture.Bytes())
			expect(t, uploaded, 201)
			var token struct{ ID string }
			if err := json.Unmarshal(uploaded.Body.Bytes(), &token); err != nil {
				t.Fatal(err)
			}
			expect(t, admin.call("GET", "/api/v2/metadata/songs/two/cover-upload/"+token.ID, "", ""), 404)
			preview := admin.call("GET", "/api/v2/metadata/songs/one/cover-upload/"+token.ID, "", "")
			expect(t, preview, 200)
			if !bytes.Equal(preview.Body.Bytes(), picture.Bytes()) {
				t.Fatal("Upload preview differed")
			}
			s := songs["one"]
			path := filepath.Join(a.musicDir, s.Path)
			// Start with an existing, visibly different cover. Merely checking
			// for an attached picture cannot prove that replacement worked.
			oldCover := image.NewRGBA(image.Rect(0, 0, 64, 64))
			draw.Draw(oldCover, oldCover.Bounds(), image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
			var oldPNG bytes.Buffer
			png.Encode(&oldPNG, oldCover)
			oldPath := filepath.Join(t.TempDir(), "old.png")
			if err := os.WriteFile(oldPath, oldPNG.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			prepared := path + format.ext
			if out, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-i", oldPath, "-map", "0:a", "-map", "1:v:0", "-c", "copy", "-disposition:v:0", "attached_pic", prepared).CombinedOutput(); err != nil {
				t.Fatalf("seed old cover: %v %s", err, out)
			}
			if err := os.Rename(prepared, path); err != nil {
				t.Fatal(err)
			}
			before, err := metadataAudioHash(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			writeNativeCredits(t, path, nativeCredits{[]string{"Band", "Collaborator"}, []string{"Band"}, []string{"band", "guest"}, []string{"band"}})
			composers := []string{"Composer One", "Composer Two"}
			nativeComposers(t, path, composers)
			// Navidrome fixture must report the same full credits.
			s.Artist = "Band / Collaborator"
			s.Artists = []ArtistCredit{{ID: s.ArtistID, Name: "Band"}, {ID: "guest", Name: "Collaborator"}}
			songs["one"] = s
			edit := MetadataEdit{Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumArtist: s.AlbumArtist, CoverUploadID: token.ID}
			expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(edit), ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			p, err := a.probeUpload(path, uploadFormat(format.ext))
			if err != nil || uploadedCoverStream(p) < 0 {
				t.Fatal("Uploaded cover missing", err)
			}
			covers := 0
			for _, stream := range p.Streams {
				if stream.Disposition.AttachedPic == 1 {
					covers++
				}
			}
			if covers != 1 {
				t.Fatalf("old cover retained alongside replacement: %d pictures", covers)
			}
			encoded, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-map", "0:v:0", "-frames:v", "1", "-c:v", "png", "-f", "image2pipe", "pipe:1").Output()
			if err != nil {
				t.Fatal(err)
			}
			written, err := png.Decode(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			r, _, b, _ := written.At(32, 32).RGBA()
			if b < 60000 || r > 5000 {
				t.Fatal("embedded artwork is not the selected blue replacement")
			}
			if got := readNativeCredits(t, path); len(got.Artists) != 2 || got.Artists[1] != "Collaborator" {
				t.Fatalf("Credits changed: %+v", got)
			}
			if got := nativeComposers(t, path, nil); len(got) != 2 {
				t.Fatalf("Composers flattened: %v", got)
			}
			edit.CoverUploadID = ""
			edit.RemoveCover = true
			expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(edit), ""), 202)
			a.runMetadata(context.Background())
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			p, err = a.probeUpload(path, uploadFormat(format.ext))
			if err != nil || uploadedCoverStream(p) >= 0 {
				t.Fatal("Cover was not removed", err)
			}
			after, err := metadataAudioHash(context.Background(), path)
			if err != nil || before != after {
				t.Fatal("Audio changed", err)
			}
			assertNoMetadataCopies(t, a)
		})
	}
}
func TestPublicArtworkURLsAndRedirects(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "100.64.0.1", "192.168.1.1"} {
		if publicArtworkIP(netip.MustParseAddr(ip)) {
			t.Fatal("Private address allowed", ip)
		}
	}
	var cover bytes.Buffer
	png.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	a := &App{HTTP: &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/redirect" {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://127.0.0.1/private"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(cover.Bytes()))}, nil
	})}}
	path := filepath.Join(t.TempDir(), "cover")
	if err := a.downloadMetadataCover(context.Background(), "https://1.1.1.1/cover.png", path); err != nil {
		t.Fatal(err)
	}
	if err := a.downloadMetadataCover(context.Background(), "https://1.1.1.1/redirect", path); err == nil {
		t.Fatal("Private redirect allowed")
	}
	u, _ := url.Parse("http://1.1.1.1/cover.png")
	if validateArtworkURL(context.Background(), u) == nil {
		t.Fatal("HTTP allowed")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// Exercise the real server scan as well as the bytes on disk: image changes
// must retain the two independently indexed artists established by the repair.
func testRealNavidromeCoverChanges(t *testing.T, a *App, admin client, c Credentials, song Song) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	remote, err := a.nav(ctx, c, "getSong", url.Values{"id": {song.ID}, "realPath": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	relative, _, err := a.deletionPath(ctx, c, remote.Song)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.musicDir, relative)
	before, err := metadataAudioHash(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 96, 96))); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", admin.baseURL()+"/api/v2/metadata/songs/"+song.ID+"/cover-upload", bytes.NewReader(picture.Bytes()))
	req.Header.Set("Origin", admin.baseURL())
	req.Header.Set("X-CSRF-Token", admin.csrf)
	req.AddCookie(admin.cookie)
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, req)
	expect(t, response, 201)
	var token struct{ ID string }
	if err := json.Unmarshal(response.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	edit := MetadataEdit{Title: song.Title, Artist: song.Artist, Album: song.Album, AlbumArtist: song.AlbumArtist, CoverUploadID: token.ID}
	for _, remove := range []bool{false, true} {
		edit.RemoveCover = remove
		if remove {
			edit.CoverUploadID = ""
		}
		expect(t, admin.call("PUT", "/api/v2/metadata/songs/"+song.ID, mustJSON(edit), ""), 202)
		a.runMetadata(ctx)
		var state, message string
		a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
		if state != "completed" {
			t.Fatalf("Artwork remove=%v: %s %s", remove, state, message)
		}
		probe, err := a.probeUpload(path, uploadFormat(".flac"))
		if err != nil || (uploadedCoverStream(probe) >= 0) == remove {
			t.Fatalf("Incorrect artwork remove=%v: %v", remove, err)
		}
		if !remove {
			output := admin.call("GET", "/api/v2/metadata/songs/"+song.ID+"/cover", "", "")
			expect(t, output, 200)
			if _, _, err := image.DecodeConfig(bytes.NewReader(output.Body.Bytes())); err != nil {
				t.Fatal(err)
			}
		}
		// The metadata worker requests the scan; wait for it to finish before
		// checking Navidrome identities and starting the following operation.
		indexed := false
		for ctx.Err() == nil {
			scan, err := a.nav(ctx, c, "getScanStatus", nil)
			if err != nil {
				t.Fatal(err)
			}
			if !scan.Scan.Scanning {
				got, err := a.nav(ctx, c, "getSong", mapValues("id", song.ID))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got.Song.Artists, song.Artists) || got.Song.AlbumArtist != song.AlbumArtist {
					t.Fatalf("Artwork changed indexed credits: %+v", got.Song)
				}
				indexed = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !indexed {
			t.Fatalf("Navidrome scan did not finish, remove=%v", remove)
		}
	}
	after, err := metadataAudioHash(ctx, path)
	if err != nil || before != after {
		t.Fatal("Artwork changed audio", err)
	}
}

func TestCoverSearchKeepsAvailableArtwork(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	a.HTTP = &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"releases":[{"id":"12345678-1234-1234-1234-123456789012","title":"Unavailable"},{"id":"12345678-1234-1234-1234-123456789013","title":"Available"}]}`
		status := 200
		if r.URL.Host == "coverartarchive.org" {
			if strings.HasSuffix(r.URL.Path, "12") {
				status = 500
				body = "unavailable"
			} else {
				body = `{"images":[{"front":true,"thumbnails":{"500":"https://archive.org/cover.jpg"}}]}`
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	result := admin.call("GET", "/api/v2/metadata/covers?q=Album", "", "")
	expect(t, result, 200)
	var covers []coverCandidate
	if err := json.Unmarshal(result.Body.Bytes(), &covers); err != nil || len(covers) != 1 || covers[0].Title != "Available" {
		t.Fatalf("Available cover was lost: %s (%v)", result.Body, err)
	}
}
