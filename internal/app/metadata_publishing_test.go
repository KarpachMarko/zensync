package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func metadataLibraryFixture(t *testing.T) (*App, client, map[string]Song) {
	return metadataLibraryFixtureFormat(t, ".flac", "flac", "")
}
func metadataLibraryFixtureFormat(t *testing.T, ext, codec, rate string) (*App, client, map[string]Song) {
	t.Helper()
	a, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.DB.Close() })
	a.musicDir = t.TempDir()
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	admin := login(t, a, "admin")
	data := nativeFixtureAudio(t, codec, rate, ext)
	songs := map[string]Song{}
	for _, sid := range []string{"one", "two"} {
		if e = os.WriteFile(filepath.Join(a.musicDir, sid+ext), data, 0644); e != nil {
			t.Fatal(e)
		}
		artist, album := "Band", "Album"
		if sid == "two" {
			artist = "band"
			album = "Album!"
		}
		songs[sid] = Song{ID: sid, ArtistID: "artist-" + sid, AlbumID: "album-" + sid, Title: "Song", Artist: artist, Album: album, AlbumArtist: artist, Duration: 1, Path: sid + ext, Size: int64(len(data))}
	}
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		out := map[string]any{"status": "ok"}
		switch strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/"), ".view") {
		case "getSong":
			s := songs[r.Form.Get("id")]
			if info, e := os.Stat(filepath.Join(a.musicDir, s.Path)); e == nil {
				s.Size = info.Size()
			}
			out["song"] = s
		case "download":
			data, e := os.ReadFile(filepath.Join(a.musicDir, songs[r.Form.Get("id")].Path))
			if e != nil {
				http.Error(w, "missing", 404)
				return
			}
			w.Write(data)
			return
		case "search3":
			list := []Song{}
			for _, sid := range []string{"one", "two"} {
				list = append(list, songs[sid])
			}
			out["searchResult3"] = map[string]any{"song": list}
		case "startScan":
		case "getAlbum":
			out["album"] = map[string]any{"id": "album-one", "name": "Album", "song": []Song{songs["one"]}}
		case "getArtist":
			out["artist"] = map[string]any{"name": "Band", "album": []map[string]any{{"id": "album-one", "name": "Album", "songCount": 1}}}
		case "getArtists":
			out["artists"] = map[string]any{"index": []map[string]any{{"artist": []map[string]any{{"id": "artist-one", "name": "Band", "albumCount": 1}}}}}
		case "getAlbumList2":
			out["albumList2"] = map[string]any{"album": []map[string]any{{"id": "album-one", "name": "Album", "songCount": 1}}}
		case "getPlaylists":
			out["playlists"] = map[string]any{"playlist": []RemotePlaylist{{ID: "playlist", Name: "Favorites", SongCount: 1}}}
		case "getPlaylist":
			out["playlist"] = RemotePlaylist{ID: "playlist", Name: "Favorites", Entries: []Song{songs["one"]}}
		default:
			t.Errorf("Unexpected nav endpoint %s", r.URL.Path)
		}
		send(w, 200, map[string]any{"subsonic-response": out})
	}))
	t.Cleanup(nav.Close)
	if e = a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "password"}); e != nil {
		t.Fatal(e)
	}
	var uid string
	if e = a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&uid); e != nil {
		t.Fatal(e)
	}
	if e = a.seal("navidrome:"+uid, Credentials{Username: "service", Password: "password"}); e != nil {
		t.Fatal(e)
	}
	return a, admin, songs
}
func TestMetadataPublishesTagsAndResumesAfterReplacement(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	source := songs["one"]
	path := filepath.Join(a.musicDir, source.Path)
	before, e := hashFile(path)
	if e != nil {
		t.Fatal(e)
	}
	audio, e := metadataAudioHash(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	edit := MetadataEdit{Title: "Corrected", Artist: "Canonical artist", Album: "Canonical album", Lyrics: "Words", SyncedLyrics: "[00:01]Words"}
	response := admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(edit), "")
	expect(t, response, 202)
	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	a.runMetadata(context.Background())
	var state, message string
	if e = a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", result.ID).Scan(&state, &message); e != nil {
		t.Fatal(e)
	}
	if state != "completed" {
		t.Fatalf("%s: %s", state, message)
	}
	p, e := a.probeUpload(path, "flac")
	if e != nil {
		t.Fatal(e)
	}
	tags := uploadTags(p)
	if tags["title"] != edit.Title || tags["artist"] != edit.Artist || tags["album"] != edit.Album || tags["lyrics"] != "[00:01]Words" {
		t.Fatalf("tags: %+v", tags)
	}
	after, e := metadataAudioHash(context.Background(), path)
	if e != nil || audio != after {
		t.Fatal("Audio changed")
	}
	var savedBefore string
	if e = a.DB.QueryRow("SELECT before_hash FROM metadata_files WHERE job_id=?", result.ID).Scan(&savedBefore); e != nil || savedBefore != before {
		t.Fatal("Missing recovery journal")
	}
	// Simulate a crash after replacement, before progress was recorded.
	if _, e = a.DB.Exec("UPDATE metadata_jobs SET state='running',done=0 WHERE id=?", result.ID); e != nil {
		t.Fatal(e)
	}
	a.runMetadata(context.Background())
	if e = a.DB.QueryRow("SELECT state FROM metadata_jobs WHERE id=?", result.ID).Scan(&state); e != nil || state != "completed" {
		t.Fatalf("Resume failed: %s %v", state, e)
	}
	files, _ := filepath.Glob(filepath.Join(a.dataDir, "metadata-backups", "*", "original.flac"))
	if len(files) != 1 {
		t.Fatal("Recovery copy missing")
	}
	backupHash, e := hashFile(files[0])
	if e != nil || backupHash != before {
		t.Fatal("Original backup changed")
	}
}
func TestMetadataAlbumAndArtistMerges(t *testing.T) {
	for _, kind := range []string{"album", "artist"} {
		t.Run(kind, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixture(t)
			// Destination identity fields must propagate even when the source's name is equal.
			targetPath := filepath.Join(a.musicDir, songs["one"].Path)
			tmp := targetPath + ".flac"
			cmd := exec.Command("ffmpeg", "-v", "error", "-i", targetPath, "-c", "copy", "-metadata", "album=Album", "-metadata", "album_artist=Band", "-metadata", "MUSICBRAINZ_ALBUMID=canonical-release", "-metadata", "date=2024", "-metadata", "MUSICBRAINZ_ARTISTID=canonical-artist", tmp)
			if out, e := cmd.CombinedOutput(); e != nil {
				t.Fatalf("fixture: %v %s", e, out)
			}
			if e := os.Rename(tmp, targetPath); e != nil {
				t.Fatal(e)
			}
			response := admin.call("POST", "/api/v2/metadata/merge", mustJSON(map[string]string{"kind": kind, "source": kind + "-two", "target": kind + "-one"}), "")
			expect(t, response, 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			p, e := a.probeUpload(filepath.Join(a.musicDir, songs["two"].Path), "flac")
			if e != nil {
				t.Fatal(e)
			}
			tags := uploadTags(p)
			if kind == "album" && (tags["album"] != "Album" || tags["musicbrainz_albumid"] != "canonical-release" || tags["date"] != "2024") {
				t.Fatalf("Album grouping tags: %+v", tags)
			}
			if kind == "artist" && (tags["artist"] != "Band" || tags["artists"] != "Band" || tags["musicbrainz_artistid"] != "canonical-artist") {
				t.Fatalf("Artist grouping tags: %+v", tags)
			}
		})
	}
}
func TestLibraryBrowseAndSuggestions(t *testing.T) {
	_, admin, _ := metadataLibraryFixture(t)
	for _, view := range []string{"playlists", "artists", "albums"} {
		expect(t, admin.call("GET", "/api/v2/library/browse?view="+view, "", ""), 200)
	}
	for _, path := range []string{"playlists&id=playlist", "albums&id=album-one"} {
		got := admin.call("GET", "/api/v2/library/browse?view="+path, "", "")
		expect(t, got, 200)
		if !strings.Contains(got.Body.String(), `"id":"one"`) {
			t.Fatal(got.Body)
		}
	}
	got := admin.call("GET", "/api/v2/library/browse?view=artists&id=artist-one", "", "")
	expect(t, got, 200)
	if !strings.Contains(got.Body.String(), `"id":"album-one"`) {
		t.Fatal(got.Body)
	}
	got = admin.call("GET", "/api/v2/metadata/suggestions?field=artist&q=BAN", "", "")
	expect(t, got, 200)
	if !strings.Contains(got.Body.String(), "Band") {
		t.Fatal(got.Body)
	}
}
func TestMetadataProviderFollows307AndRejectsUnsafeRedirects(t *testing.T) {
	for _, destination := range []string{"https://archive.org/download/cover/index.json", "http://127.0.0.1/private", "https://evil.example/private"} {
		t.Run(destination, func(t *testing.T) {
			calls := 0
			a := &App{HTTP: &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host == "coverartarchive.org" {
					return &http.Response{StatusCode: 307, Header: http.Header{"Location": {destination}}, Body: io.NopCloser(strings.NewReader("redirect"))}, nil
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"images":[]}`))}, nil
			})}}
			var art struct {
				Images []string `json:"images"`
			}
			e := a.metadataJSON(context.Background(), "https://coverartarchive.org/release/id", &art)
			if strings.HasPrefix(destination, "https://archive.org/") {
				if e != nil || calls != 2 {
					t.Fatalf("redirect: %v calls=%d", e, calls)
				}
			} else if e == nil || calls != 1 {
				t.Fatalf("Unsafe redirect followed: %v calls=%d", e, calls)
			}
		})
	}
}

func TestMetadataCoverEmbeddingPreservesAudio(t *testing.T) {
	for _, format := range []struct{ ext, codec, rate string }{{".mp3", "libmp3lame", "192k"}, {".flac", "flac", ""}, {".m4a", "aac", "192k"}} {
		t.Run(format.ext, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixtureFormat(t, format.ext, format.codec, format.rate)
			var cover bytes.Buffer
			if e := png.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 32, 32))); e != nil {
				t.Fatal(e)
			}
			a.HTTP = &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "archive.org" {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(cover.Bytes()))}, nil
				}
				return http.DefaultTransport.RoundTrip(r)
			})}
			source := songs["one"]
			path := filepath.Join(a.musicDir, source.Path)
			audio, e := metadataAudioHash(context.Background(), path)
			if e != nil {
				t.Fatal(e)
			}
			edit := MetadataEdit{Title: source.Title, Artist: source.Artist, Album: source.Album, CoverURL: "https://archive.org/cover.png", Lyrics: "Words"}
			expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(edit), ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			p, e := a.probeUpload(path, uploadFormat(format.ext))
			if e != nil {
				t.Fatal(e)
			}
			if uploadedCoverStream(p) < 0 {
				t.Fatal("Selected cover was not embedded")
			}
			after, e := metadataAudioHash(context.Background(), path)
			if e != nil || after != audio {
				t.Fatal("Audio changed")
			}
		})
	}
}
func TestMetadataRejectsChangedSourceDuringRecovery(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	source := songs["one"]
	edit := MetadataEdit{Title: "Changed", Artist: source.Artist, Album: source.Album}
	response := admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(edit), "")
	expect(t, response, 202)
	var result struct {
		ID string `json:"id"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	a.runMetadata(context.Background())
	path := filepath.Join(a.musicDir, source.Path)
	if e := os.WriteFile(path, []byte("external edit"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := a.DB.Exec("UPDATE metadata_jobs SET state='running',done=0 WHERE id=?", result.ID); e != nil {
		t.Fatal(e)
	}
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs WHERE id=?", result.ID).Scan(&state, &message)
	if state != "failed" || !strings.Contains(message, "Source changed") {
		t.Fatalf("%s %s", state, message)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "external edit" {
		t.Fatal("External edit overwritten")
	}
}

func TestMetadataMergesInEitherOrder(t *testing.T) {
	for _, order := range [][]string{{"artist", "album"}, {"album", "artist"}} {
		t.Run(strings.Join(order, "-"), func(t *testing.T) {
			a, admin, songs := metadataLibraryFixture(t)
			for _, sid := range []string{"one", "two"} {
				path := filepath.Join(a.musicDir, songs[sid].Path)
				tmp := path + ".flac"
				release := "release-" + sid
				cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-c", "copy",
					"-metadata", "artist="+songs[sid].Artist,
					"-metadata", "album="+songs[sid].Album,
					"-metadata", "album_artist="+songs[sid].Artist,
					"-metadata", "MUSICBRAINZ_ARTISTID=artist-"+sid,
					"-metadata", "MUSICBRAINZ_ALBUMARTISTID=artist-"+sid,
					"-metadata", "MUSICBRAINZ_ALBUMID="+release, tmp)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, out)
				}
				if err := os.Rename(tmp, path); err != nil {
					t.Fatal(err)
				}
			}
			for _, kind := range order {
				expect(t, admin.call("POST", "/api/v2/metadata/merge", mustJSON(map[string]string{"kind": kind, "source": kind + "-two", "target": kind + "-one"}), ""), 202)
				a.runMetadata(context.Background())
				var state, message string
				a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
				if state != "completed" {
					t.Fatalf("%s: %s", state, message)
				}
				p, err := a.probeUpload(filepath.Join(a.musicDir, songs["two"].Path), "flac")
				if err != nil {
					t.Fatal(err)
				}
				tags := uploadTags(p)
				if kind == "artist" && order[0] == "artist" && tags["musicbrainz_albumid"] != "release-two" {
					t.Fatalf("Artist merge changed album edition: %+v", tags)
				}
				if kind == "album" && order[0] == "album" && tags["artist"] != "band" {
					t.Fatalf("Album merge changed track artist: %+v", tags)
				}
				// Simulate the Navidrome rescan from the actual resulting file tags.
				s := songs["two"]
				s.Artist, s.Album, s.AlbumArtist = tags["artist"], tags["album"], tags["album_artist"]
				if kind == "artist" {
					s.ArtistID = "artist-one"
				}
				if kind == "album" {
					s.AlbumID = "album-one"
				}
				songs["two"] = s
				a.navIndexMu.Lock()
				clear(a.navIndexes)
				a.navIndexMu.Unlock()
			}
			p, err := a.probeUpload(filepath.Join(a.musicDir, songs["two"].Path), "flac")
			if err != nil {
				t.Fatal(err)
			}
			tags := uploadTags(p)
			if tags["artist"] != "Band" || tags["album"] != "Album" || tags["album_artist"] != "Band" || tags["musicbrainz_albumid"] != "release-one" || tags["musicbrainz_artistid"] != "artist-one" || tags["musicbrainz_albumartistid"] != "artist-one" {
				t.Fatalf("Merge order lost relationships: %+v", tags)
			}
		})
	}
}

func TestMetadataArtistMergePreservesCompilation(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	for _, sid := range []string{"one", "two"} {
		s := songs[sid]
		s.AlbumArtist = "Various Artists"
		songs[sid] = s
		path := filepath.Join(a.musicDir, s.Path)
		tmp := path + ".flac"
		cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-c", "copy",
			"-metadata", "artist="+s.Artist, "-metadata", "album="+s.Album,
			"-metadata", "album_artist=Various Artists", "-metadata", "albumartists=Various Artists",
			"-metadata", "MUSICBRAINZ_ARTISTID=artist-"+sid,
			"-metadata", "MUSICBRAINZ_ALBUMARTISTID=various-artists",
			"-metadata", "MUSICBRAINZ_ALBUMID=release-"+sid, tmp)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if err := os.Rename(tmp, path); err != nil {
			t.Fatal(err)
		}
	}
	expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"artist","source":"artist-two","target":"artist-one"}`, ""), 202)
	a.runMetadata(context.Background())
	p, err := a.probeUpload(filepath.Join(a.musicDir, songs["two"].Path), "flac")
	if err != nil {
		t.Fatal(err)
	}
	tags := uploadTags(p)
	if tags["artist"] != "Band" || tags["musicbrainz_artistid"] != "artist-one" || tags["album_artist"] != "Various Artists" || tags["musicbrainz_albumartistid"] != "various-artists" || tags["musicbrainz_albumid"] != "release-two" {
		t.Fatalf("Compilation relationships changed: %+v", tags)
	}
}

func TestMetadataArtistMergeReplacesGroupingAliases(t *testing.T) {
	for _, format := range []struct{ ext, codec string }{{".mp3", "mp3"}, {".flac", "flac"}} {
		t.Run(format.codec, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixtureFormat(t, format.ext, format.codec, "")
			for _, sid := range []string{"one", "two"} {
				s := songs[sid]
				s.Artist, s.AlbumArtist = "May Wave$", "May Wave$"
				if sid == "two" {
					s.Artist, s.AlbumArtist = "May Waves", "May Waves"
				}
				songs[sid] = s
				path := filepath.Join(a.musicDir, s.Path)
				tmp := path + format.ext
				cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-c", "copy",
					"-metadata", "artist="+s.Artist, "-metadata", "artists="+s.Artist,
					"-metadata", "album_artist="+s.Artist, "-metadata", "albumartist="+s.Artist,
					"-metadata", "album artist="+s.Artist, "-metadata", "album artists="+s.Artist,
					"-metadata", "albumartists="+s.Artist, tmp)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, out)
				}
				if err := os.Rename(tmp, path); err != nil {
					t.Fatal(err)
				}
			}
			expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"artist","source":"artist-two","target":"artist-one"}`, ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s: %s", state, message)
			}
			p, err := a.probeUpload(filepath.Join(a.musicDir, songs["two"].Path), format.codec)
			if err != nil {
				t.Fatal(err)
			}
			tags := uploadTags(p)
			for _, key := range []string{"artist", "artists", "album_artist", "albumartist", "album artist", "album artists", "albumartists"} {
				if value, ok := tags[key]; ok && value != "May Wave$" {
					t.Errorf("stale grouping alias %s=%q", key, value)
				}
			}
		})
	}
}

// Opt-in integration regression: checks Navidrome's interpretation of the
// written aliases, rather than only ffprobe's view of the resulting file.
func TestRealNavidromeArtistMerge(t *testing.T) {
	base := os.Getenv("ZENSYNC_TEST_NAV_URL")
	if base == "" {
		t.Skip("run scripts/test-navidrome.py")
	}
	a, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	if err := a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	a.musicDir = os.Getenv("ZENSYNC_TEST_MUSIC")
	dir, err := os.MkdirTemp(a.musicDir, "merge-regression-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	c := Credentials{URL: base, Username: "navadmin", Password: os.Getenv("ZENSYNC_TEST_NAV_PASSWORD")}
	if err := a.seal("navidrome", c); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, f := range []struct{ file, artist string }{{"source.mp3", "May Waves"}, {"target.flac", "May Wave$"}} {
		cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=1",
			"-metadata", "title=Merge regression "+f.file, "-metadata", "album=Merge regression "+f.file,
			"-metadata", "artist="+f.artist, "-metadata", "artists="+f.artist,
			"-metadata", "album_artist="+f.artist, "-metadata", "album artist="+f.artist,
			"-metadata", "albumartists="+f.artist, "-metadata", "album artists="+f.artist, filepath.Join(dir, f.file))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	if _, err := a.nav(ctx, c, "startScan", mapValues("fullScan", "true")); err != nil {
		t.Fatal(err)
	}
	var source, target Song
	for ctx.Err() == nil {
		songs, err := a.navSearch(ctx, c, "Merge regression", 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range songs {
			if s.Artist == "May Waves" {
				source = s
			}
			if s.Artist == "May Wave$" {
				target = s
			}
		}
		if source.ID != "" && target.ID != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if source.ID == "" || target.ID == "" {
		t.Fatal("Fixtures not indexed")
	}
	expect(t, admin.call("POST", "/api/v2/metadata/merge", mustJSON(map[string]string{"kind": "artist", "source": source.ArtistID, "target": target.ArtistID}), ""), 202)
	a.runMetadata(ctx)
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s: %s", state, message)
	}
	for ctx.Err() == nil {
		songs, err := a.navSearch(ctx, c, "Merge regression", 0)
		if err != nil {
			t.Fatal(err)
		}
		correct := len(songs) == 2
		for _, s := range songs {
			correct = correct && s.Artist == "May Wave$" && s.AlbumArtist == "May Wave$" && s.ArtistID == target.ArtistID
		}
		artists, err := a.nav(ctx, c, "getArtists", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, index := range artists.Artists.Index {
			for _, artist := range index.Artists {
				if artist.Name == "May Waves" {
					correct = false
				}
			}
		}
		if correct {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Navidrome still exposes old artist after merge and scan")
}

func TestRealNavidromeScreenshotReview(t *testing.T) {
	base := os.Getenv("ZENSYNC_TEST_NAV_URL")
	if base == "" {
		t.Skip("run scripts/test-navidrome.py")
	}
	var fixture struct {
		AlbumTitle       string
		AlbumArtists     []string
		CombinedArtists  []string
		StandaloneArtist string
	}
	raw, err := os.ReadFile("testdata/screenshot-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	a, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	music := os.Getenv("ZENSYNC_TEST_MUSIC")
	dir, err := os.MkdirTemp(music, "screenshot-review-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for i, albumArtist := range fixture.AlbumArtists {
		artist := fixture.StandaloneArtist
		if i == 0 {
			artist = fixture.CombinedArtists[0]
		}
		cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=1",
			"-metadata", "title=Screenshot regression "+fmt.Sprint(i),
			"-metadata", "album="+fixture.AlbumTitle,
			// Distinct release IDs model the hidden grouping difference; the
			// screenshots do not reveal actual MusicBrainz IDs or edition tags.
			"-metadata", fmt.Sprintf("musicbrainz_albumid=00000000-0000-4000-8000-%012d", i+1),
			"-metadata", "artist="+artist, "-metadata", "artists="+artist,
			"-metadata", "album_artist="+albumArtist, "-metadata", "albumartists="+albumArtist,
			filepath.Join(dir, fmt.Sprintf("%d.flac", i)))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, output)
		}
	}
	c := Credentials{URL: base, Username: "navadmin", Password: os.Getenv("ZENSYNC_TEST_NAV_PASSWORD")}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := a.nav(ctx, c, "startScan", mapValues("fullScan", "true")); err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		songs, err := a.navSearch(ctx, c, "Screenshot regression", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(songs) == 2 {
			var albumGroups []duplicateGroup
			for _, group := range metadataDuplicates(songs) {
				if group.Kind == "album" {
					albumGroups = append(albumGroups, group)
				}
			}
			if len(albumGroups) != 1 || len(albumGroups[0].Members) != 2 {
				t.Fatalf("Real Navidrome screenshot album pair missed: %+v", songs)
			}
			issues := metadataCreditIssues(songs)
			if len(issues) != 1 || issues[0].Name != fixture.CombinedArtists[0] || issues[0].Role != "artist" {
				t.Fatalf("Real Navidrome combined credit missed: %+v / %+v", songs, issues)
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Screenshot fixtures were not indexed")
}

func TestMetadataMergeUpdatesExactCopyOnly(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	original := filepath.Join(a.musicDir, songs["two"].Path)
	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"Artist/Album/two.flac", "Album/two.flac"} {
		path := filepath.Join(a.musicDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Report an exact Navidrome path while retaining the fixture's original
	// download endpoint. All three local candidates initially have equal bytes.
	transport := a.HTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	a.HTTP.Transport = metadataTransport(func(req *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(req)
		if err != nil || !strings.HasSuffix(req.URL.Path, "/getSong.view") {
			return response, err
		}
		var remote struct {
			Response NavResponse `json:"subsonic-response"`
		}
		err = json.NewDecoder(response.Body).Decode(&remote)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		rel := remote.Response.Song.Path
		if remote.Response.Song.ID == "two" {
			rel = "Artist/Album/two.flac"
		}
		remote.Response.Song.Path = filepath.Join(a.musicDir, rel)
		raw, err := json.Marshal(remote)
		if err != nil {
			return nil, err
		}
		response.Body = io.NopCloser(bytes.NewReader(raw))
		response.ContentLength = int64(len(raw))
		return response, nil
	})
	expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"album","source":"album-two","target":"album-one"}`, ""), 202)
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s: %s", state, message)
	}
	for _, rel := range []string{"two.flac", "Album/two.flac"} {
		unchanged, err := os.ReadFile(filepath.Join(a.musicDir, rel))
		if err != nil || !bytes.Equal(unchanged, data) {
			t.Fatalf("Another copy was changed: %s %v", rel, err)
		}
	}
	p, err := a.probeUpload(filepath.Join(a.musicDir, "Artist/Album/two.flac"), "flac")
	if err != nil {
		t.Fatal(err)
	}
	if uploadTags(p)["album"] != "Album" {
		t.Fatal("Exact copy was not updated")
	}
}
