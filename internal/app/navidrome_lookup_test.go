package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type lookupTraffic struct {
	mu        sync.Mutex
	queries   map[string]int
	downloads map[string]int
}

func lookupFixture(t *testing.T) (*App, string, []byte) {
	t.Helper()
	a, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.DB.Close() })
	a.musicDir = t.TempDir()
	rel := "SLAVA MARLOW/SLAVA MARLOW - ЭЛЬФ 1/03 - Я НЕНАВИЖУ ЭТОТ ТРЕК!!! 凸(￣ヘ￣).flac"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(a.musicDir, rel)), 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte("original validated file bytes")
	if err := os.WriteFile(filepath.Join(a.musicDir, rel), data, 0644); err != nil {
		t.Fatal(err)
	}
	return a, rel, data
}

// Models text search independently of browse-all and original-file downloads.
func lookupServer(t *testing.T, songs []Song, originals map[string][]byte, matches func(string, Song) bool, denyDownload bool) (Credentials, *lookupTraffic) {
	t.Helper()
	traffic := &lookupTraffic{queries: map[string]int{}, downloads: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traffic.mu.Lock()
		defer traffic.mu.Unlock()
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		out := map[string]any{"status": "ok", "version": "1.16.1"}
		switch filepath.Base(r.URL.Path) {
		case "search3.view":
			query := r.Form.Get("query")
			traffic.queries[query]++
			filtered := []Song{}
			for _, song := range songs {
				if query == "" || matches(query, song) {
					filtered = append(filtered, song)
				}
			}
			offset, _ := strconv.Atoi(r.Form.Get("songOffset"))
			offset = min(offset, len(filtered))
			out["searchResult3"] = map[string]any{"song": filtered[offset:min(offset+100, len(filtered))]}
		case "download.view":
			sid := r.Form.Get("id")
			traffic.downloads[sid]++
			if denyDownload {
				out["status"] = "failed"
				out["error"] = map[string]any{"code": 50, "message": "Not authorized"}
				break
			}
			w.Header().Set("Content-Type", "audio/flac")
			w.Write(originals[sid])
			return
		case "getScanStatus.view", "startScan.view":
			out["scanStatus"] = map[string]any{"scanning": false}
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		send(w, 200, map[string]any{"subsonic-response": out})
	}))
	t.Cleanup(server.Close)
	return Credentials{URL: server.URL, Username: "service", Password: "fixture"}, traffic
}

func TestPlacedLookupShortensUnicodeSymbolTitleAndIgnoresStaleIndexedSize(t *testing.T) {
	a, rel, data := lookupFixture(t)
	track := makeTrack(42, "Я НЕНАВИЖУ ЭТОТ ТРЕК!!! 凸(￣ヘ￣)")
	song := Song{ID: "indexed", Title: "Я НЕНАВИЖУ ЭТОТ ТРЕК!!! 凸 ( ￣ヘ￣ )", Artist: "Slava Marlow, ЛСП", Album: "ЭЛЬФ 1", Path: "Virtual artist/Virtual album/03 - Song.flac", Size: int64(len(data) + 17)}
	c, traffic := lookupServer(t, []Song{song}, map[string][]byte{song.ID: data}, func(q string, _ Song) bool {
		return q == "я ненавижу этот трек"
	}, false)
	id, err := a.findPlaced(context.Background(), c, rel, mustJSON(track))
	if err != nil || id != song.ID {
		t.Fatalf("lookup: id=%q err=%v", id, err)
	}
	if traffic.queries[track.Title] != 1 || traffic.queries["я ненавижу этот трек"] != 1 || traffic.queries[""] != 0 || traffic.downloads[song.ID] != 1 {
		t.Fatalf("queries=%v downloads=%v", traffic.queries, traffic.downloads)
	}
	// The standalone verifier used by relocation must also distrust stale size.
	if same, err := a.sameRemoteAudio(context.Background(), c, song, rel); err != nil || !same {
		t.Fatalf("stale size rejected original bytes: %v %v", same, err)
	}
}

func TestPlacedLookupCachedPaginatedBrowseFindsRenamedSong(t *testing.T) {
	a, rel, data := lookupFixture(t)
	songs := []Song{}
	for i := range 100 {
		songs = append(songs, Song{ID: fmt.Sprint(i), Title: "Unrelated", Size: 1000})
	}
	songs = append(songs, Song{ID: "last", Title: "Completely changed tags", Path: "Virtual path", Size: int64(len(data))})
	c, traffic := lookupServer(t, songs, map[string][]byte{"last": data}, func(string, Song) bool { return false }, false)
	track := makeTrack(42, "$$$")
	for range 2 {
		id, err := a.findPlaced(context.Background(), c, rel, mustJSON(track))
		if err != nil || id != "last" {
			t.Fatalf("browse lookup: %q %v", id, err)
		}
	}
	if traffic.queries[""] != 2 || len(traffic.downloads) != 1 || traffic.downloads["last"] != 2 {
		t.Fatalf("index not shared or unrelated songs downloaded: %+v", traffic)
	}
	// Credentials and age define the cache boundary; a stale index must refresh.
	c.Password = "changed"
	if _, err := a.findPlaced(context.Background(), c, rel, mustJSON(track)); err != nil {
		t.Fatal(err)
	}
	if traffic.queries[""] != 4 {
		t.Fatal("metadata cache crossed a credential change")
	}
	a.navIndexMu.Lock()
	index := a.navIndexes[digest(mustJSON(c))]
	index.fetched = time.Now().Add(-31 * time.Second)
	a.navIndexes[digest(mustJSON(c))] = index
	a.navIndexMu.Unlock()
	if _, err := a.findPlaced(context.Background(), c, rel, mustJSON(track)); err != nil {
		t.Fatal(err)
	}
	if traffic.queries[""] != 6 {
		t.Fatal("index never refreshed for newly scanned songs")
	}
}

func TestPlacedLookupConcurrentWorkersShareLibraryBrowse(t *testing.T) {
	a, rel, data := lookupFixture(t)
	c, traffic := lookupServer(t, []Song{{ID: "indexed", Title: "Renamed", Size: int64(len(data))}}, map[string][]byte{"indexed": data}, func(string, Song) bool { return false }, false)
	results := make(chan error, 3)
	for range 3 {
		go func() {
			id, err := a.findPlaced(context.Background(), c, rel, mustJSON(makeTrack(42, "Source title")))
			if err == nil && id != "indexed" {
				err = fmt.Errorf("unexpected song ID %q", id)
			}
			results <- err
		}()
	}
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if traffic.queries[""] != 1 {
		t.Fatalf("workers each browsed the library: %v", traffic.queries)
	}
}

func TestPlacedLookupRejectsDifferentBytesAndReportsDownloadPermissions(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprint(denied), func(t *testing.T) {
			a, rel, data := lookupFixture(t)
			track := makeTrack(42, "Same title")
			song := Song{ID: "wrong", Title: track.Title, Artist: track.Artist.Name, Album: track.Album.Title, Duration: track.Duration, Path: rel, Size: int64(len(data))}
			wrong := append([]byte{}, data...)
			wrong[0] ^= 1 // Same size, title, artist, album, duration and path.
			c, traffic := lookupServer(t, []Song{song}, map[string][]byte{song.ID: wrong}, func(string, Song) bool { return true }, denied)
			id, report, err := a.findPlacedWithReport(context.Background(), c, rel, mustJSON(track), false)
			if id != "" {
				t.Fatal("accepted metadata/path instead of verifying original bytes")
			}
			if denied {
				if err == nil || !strings.Contains(err.Error(), "download permission") {
					t.Fatalf("permission error: %v", err)
				}
			} else if err != nil || !strings.Contains(report.reason(), "served the retained file's original bytes") {
				t.Fatalf("mismatch diagnostic: %+v %v", report, err)
			}
			if traffic.downloads[song.ID] != 1 {
				t.Fatal("repeated original download across query variants/browse")
			}
		})
	}
}

func TestPlacedLookupRelocationStillRequiresUniqueByteMatch(t *testing.T) {
	a, rel, data := lookupFixture(t)
	track := makeTrack(42, "A song (feat. Someone)")
	songs := []Song{{ID: "a", Title: track.Title, Size: int64(len(data))}, {ID: "b", Title: "Renamed duplicate", Size: int64(len(data))}}
	c, traffic := lookupServer(t, songs, map[string][]byte{"a": data, "b": data}, func(_ string, s Song) bool { return s.ID == "a" }, false)
	if id, err := a.findPlaced(context.Background(), c, rel, mustJSON(track)); err != nil || id != "a" {
		t.Fatalf("ordinary verification: %q %v", id, err)
	}
	traffic.downloads = map[string]int{}
	if id, err := a.findPlaced(context.Background(), c, rel, mustJSON(track), true); id != "" || err == nil || !strings.Contains(err.Error(), "Multiple Navidrome songs") {
		t.Fatalf("ambiguous relocation allowed: %q %v", id, err)
	}
	if traffic.downloads["a"] != 1 || traffic.downloads["b"] != 1 {
		t.Fatalf("duplicate candidate verification: %v", traffic.downloads)
	}
}

func TestPlacedLookupSkipsMissingIndexedFiles(t *testing.T) {
	for _, missing := range []string{"404", "410", "api-70"} {
		t.Run(missing, func(t *testing.T) {
			a, rel, data := lookupFixture(t)
			track := makeTrack(42, "Song")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				if strings.HasSuffix(r.URL.Path, "/search3.view") {
					songs := []Song{{ID: "stale", Title: track.Title}, {ID: "current", Title: track.Title}}
					send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "searchResult3": map[string]any{"song": songs}}})
					return
				}
				if r.Form.Get("id") == "stale" {
					if missing == "api-70" {
						send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "failed", "error": map[string]int{"code": 70}}})
					} else {
						status, _ := strconv.Atoi(missing)
						http.Error(w, "No longer exists", status)
					}
					return
				}
				w.Header().Set("Content-Type", "audio/flac")
				w.Write(data)
			}))
			defer server.Close()
			c := Credentials{URL: server.URL, Username: "service", Password: "fixture"}
			if id, err := a.findPlaced(context.Background(), c, rel, mustJSON(track)); err != nil || id != "current" {
				t.Fatalf("missing candidate prevented verification: %q %v", id, err)
			}
		})
	}
}

func TestPlacedLookupSafetyLimitCannotEstablishUniqueIdentity(t *testing.T) {
	a, rel, data := lookupFixture(t)
	song := Song{ID: "a", Size: int64(len(data))}
	c, _ := lookupServer(t, nil, map[string][]byte{"a": data}, func(string, Song) bool { return false }, false)
	a.navIndexes = map[string]navSongIndex{digest(mustJSON(c)): {songs: []Song{song}, limited: true, fetched: time.Now()}}
	if id, err := a.findPlaced(context.Background(), c, rel, mustJSON(makeTrack(42, "Song")), true); id != "" || err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("truncated library accepted as unique: %q %v", id, err)
	}
}

func TestAcquisitionRetryVerifiesRetainedFileWithoutProviderOrDownloader(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg required")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe required")
	}
	a, rel, _ := lookupFixture(t)
	track := makeTrack(42, "Я НЕНАВИЖУ ЭТОТ ТРЕК!!! 凸(￣ヘ￣)")
	file := filepath.Join(a.musicDir, rel)
	if output, err := exec.Command(ffmpeg, "-y", "-v", "error", "-f", "lavfi", "-i", "sine=duration=2", "-c:a", "flac", "-metadata", "SOURCEID=42", "-metadata", "title="+track.Title, "-metadata", "artist="+track.Artist.Name, "-metadata", "album="+track.Album.Title, file).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", output, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	c, traffic := lookupServer(t, []Song{{ID: "indexed", Title: "Changed indexed tags", Size: int64(len(data))}}, map[string][]byte{"indexed": data}, func(string, Song) bool { return false }, false)
	if err := a.seal("navidrome", c); err != nil {
		t.Fatal(err)
	}
	a.createAdmin("admin", "test-password-long")
	admin := login(t, a, "admin")
	for _, statement := range []string{
		`INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) SELECT 'request',id,'retained-file','deezer:track:42','Song','Artist',0,'["lossless"]','approved' FROM users WHERE username='admin'`,
		`INSERT INTO jobs(id,request_id,state) VALUES('job','request','failed')`,
	} {
		if _, err := a.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.DB.Exec("INSERT INTO assets(target,file_path,quality,metadata) VALUES('deezer:track:42',?,'lossless',?)", rel, mustJSON(track)); err != nil {
		t.Fatal(err)
	}
	a.deezerURL = "http://127.0.0.1:1" // No provider re-fetch or downloader is allowed.
	a.dfiPath = filepath.Join(t.TempDir(), "no-downloader")
	expect(t, admin.call("POST", "/api/v2/tasks/job", "", ""), 200)
	a.runAcquisition(context.Background())
	var state, stage, server string
	if err := a.DB.QueryRow("SELECT state,stage FROM jobs WHERE id='job'").Scan(&state, &stage); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.QueryRow("SELECT server_id FROM assets WHERE target='deezer:track:42'").Scan(&server); err != nil {
		t.Fatal(err)
	}
	if state != "available" || server != "indexed" || traffic.downloads["indexed"] != 1 {
		t.Fatalf("retained-file retry: state=%s stage=%s server=%s", state, stage, server)
	}
	var attempts int
	if err := a.DB.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("retry ran downloader: attempts=%d err=%v", attempts, err)
	}
}

func TestRealNavidromeAssetVerificationUnicode(t *testing.T) {
	base := os.Getenv("ZENSYNC_TEST_NAV_URL")
	if base == "" {
		t.Skip("run scripts/test-navidrome.py")
	}
	a, rel, _ := lookupFixture(t)
	a.musicDir = os.Getenv("ZENSYNC_TEST_MUSIC")
	c := Credentials{URL: base, Username: "navadmin", Password: os.Getenv("ZENSYNC_TEST_NAV_PASSWORD")}
	file := filepath.Join(a.musicDir, rel)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	track := makeTrack(42, "Я НЕНАВИЖУ ЭТОТ ТРЕК!!! 凸(￣ヘ￣)")
	if output, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=2", "-metadata", "title="+track.Title, "-metadata", "artist=Slava Marlow", "-metadata", "album=ЭЛЬФ 1", file).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", output, err)
	}
	defer os.Remove(file)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := a.nav(ctx, c, "startScan", nil); err != nil {
		t.Fatal(err)
	}
	var expected string
	for ctx.Err() == nil {
		songs, err := a.navSearch(ctx, c, "я ненавижу", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(songs) == 1 {
			expected = songs[0].ID
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if expected == "" {
		t.Fatal("Navidrome did not index fixture")
	}
	fullTitle, err := a.navSearch(ctx, c, track.Title, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Navidrome full decorated title returned %d songs", len(fullTitle))
	if id, err := a.findPlaced(ctx, c, rel, mustJSON(track)); err != nil || id != expected {
		t.Fatalf("real Unicode lookup: %q %v", id, err)
	}
	// Force a text-search miss to exercise browse-all against the real server.
	track.Title = "Provider tags completely unlike the indexed tags"
	if id, err := a.findPlaced(ctx, c, rel, mustJSON(track)); err != nil || id != expected {
		t.Fatalf("real metadata fallback: %q %v", id, err)
	}
}
