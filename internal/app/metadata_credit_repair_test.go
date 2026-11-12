package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSkateSeparateKnownArtists(t *testing.T) {
	for _, format := range []struct{ ext, codec, rate string }{{".flac", "flac", ""}, {".mp3", "mp3", "192k"}, {".m4a", "aac", "192k"}} {
		for _, role := range []string{"artist", "album artist"} {
			t.Run(format.ext+"/"+role, func(t *testing.T) {
				a, admin, songs := metadataLibraryFixtureFormat(t, format.ext, format.codec, format.rate)
				known := songs["one"]
				known.ArtistID, known.Artist = "vacio", "Vacio / ЛСП"
				known.Artists = []ArtistCredit{{ID: "vacio", Name: "Vacio"}, {ID: "lsp", Name: "ЛСП"}}
				songs["one"] = known
				s := songs["two"]
				s.Title, s.Album = "Skate", "Skate"
				s.ArtistID, s.Artist = "combined", "Vacio, ЛСП / Other collaborator"
				s.Artists = []ArtistCredit{{ID: "combined", Name: "Vacio, ЛСП"}, {ID: "guest", Name: "Other collaborator"}}
				s.AlbumArtist = "Various Artists"
				s.AlbumArtists = []ArtistCredit{{ID: "various", Name: "Various Artists"}}
				before := nativeCredits{[]string{"Vacio, ЛСП", "Other collaborator"}, []string{"Various Artists"}, []string{"compound", "guest"}, []string{"various"}}
				want := nativeCredits{[]string{"Vacio", "ЛСП", "Other collaborator"}, []string{"Various Artists"}, []string{"", "", "guest"}, []string{"various"}}
				if role == "album artist" {
					s.ArtistID, s.Artist = "guest", "Other collaborator"
					s.Artists = []ArtistCredit{{ID: "guest", Name: "Other collaborator"}}
					s.AlbumArtist = "Vacio, ЛСП / Other album collaborator"
					s.AlbumArtists = []ArtistCredit{{ID: "combined", Name: "Vacio, ЛСП"}, {ID: "album-guest", Name: "Other album collaborator"}}
					before = nativeCredits{[]string{"Other collaborator"}, []string{"Vacio, ЛСП", "Other album collaborator"}, []string{"guest"}, []string{"compound", "album-guest"}}
					want = nativeCredits{[]string{"Other collaborator"}, []string{"Vacio", "ЛСП", "Other album collaborator"}, []string{"guest"}, []string{"", "", "album-guest"}}
				}
				songs["two"] = s
				path := filepath.Join(a.musicDir, s.Path)
				tmp := path + format.ext
				const composer = "Васильев Николай Михайлович, Олег Вадимович Савченко"
				cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-c", "copy", "-metadata", "title=Skate", "-metadata", "album=Skate", "-metadata", "composer="+composer, "-metadata", "lyrics=Original lyrics", tmp)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v %s", err, out)
				}
				if err := os.Rename(tmp, path); err != nil {
					t.Fatal(err)
				}
				writeNativeCredits(t, path, before)
				composers := []string{"Васильев Николай Михайлович", "Олег Вадимович Савченко"}
				nativeComposers(t, path, composers)
				originalAudio, err := metadataAudioHash(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}
				otherBefore, err := os.ReadFile(filepath.Join(a.musicDir, known.Path))
				if err != nil {
					t.Fatal(err)
				}
				var actor string
				if err := a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&actor); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec("INSERT INTO metadata_edits(song_id,metadata,actor) VALUES(?,?,?)", s.ID, mustJSON(MetadataEdit{Title: "Unrelated pending title", Artist: "Unrelated artist", Album: "Unrelated pending album", Lyrics: "Unrelated pending lyrics"}), actor); err != nil {
					t.Fatal(err)
				}
				payload := map[string]any{"source": "combined", "role": role, "name": "Vacio, ЛСП", "artists": []string{"Vacio", "ЛСП"}}
				// Invalid input and stale reviews must not queue writes.
				expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(map[string]any{"source": "combined", "role": role, "name": "Vacio, ЛСП", "artists": []string{"Vacio", "vacio"}}), ""), 400)
				expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(map[string]any{"source": "stale", "role": role, "name": "Vacio, ЛСП", "artists": []string{"Vacio", "ЛСП"}}), ""), 409)
				expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(payload), ""), 202)
				// A second click cannot create concurrent writes.
				expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(payload), ""), 409)
				a.runMetadata(context.Background())
				var state, message string
				a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
				if state != "completed" {
					t.Fatalf("%s: %s", state, message)
				}
				if got := readNativeCredits(t, path); !reflect.DeepEqual(got, want) {
					t.Fatalf("got %+v want %+v", got, want)
				}
				p, err := a.probeUpload(path, uploadFormat(format.ext))
				if err != nil || uploadTags(p)["album"] != "Skate" || uploadTags(p)["title"] != "Skate" || uploadTags(p)["lyrics"] != "Original lyrics" {
					t.Fatal("Unrelated metadata changed", err, uploadTags(p))
				}
				if got := nativeComposers(t, path, nil); !reflect.DeepEqual(got, composers) {
					t.Fatalf("Composer credits changed: %v", got)
				}
				audioAfter, err := metadataAudioHash(context.Background(), path)
				if err != nil || audioAfter != originalAudio {
					t.Fatal("Audio changed", err)
				}
				otherAfter, err := os.ReadFile(filepath.Join(a.musicDir, known.Path))
				if err != nil || string(otherBefore) != string(otherAfter) {
					t.Fatal("Another artist's song changed", err)
				}
				if !strings.Contains(uploadTags(p)["artist"], "collaborator") {
					t.Fatal("Another collaborator was removed")
				}
			})
		}
	}
}

func TestRealNavidromeSeparateKnownSkateArtists(t *testing.T) {
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
	dir, err := os.MkdirTemp(a.musicDir, "skate-separation-")
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
	for i, name := range []string{"Vacio", "ЛСП", "Vacio, ЛСП"} {
		path := filepath.Join(dir, fmt.Sprintf("%d.flac", i))
		albumArtist := name
		if i == 2 {
			albumArtist = "Various Artists"
		}
		out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=1", "-metadata", fmt.Sprintf("title=Skate separation proof %d", i), "-metadata", "album=Skate", "-metadata", "artist="+name, "-metadata", "album_artist="+albumArtist, path).CombinedOutput()
		if err != nil {
			t.Fatalf("%v %s", err, out)
		}
		writeNativeCredits(t, path, nativeCredits{[]string{name}, []string{albumArtist}, []string{}, []string{}})
		if i == 2 {
			coverFile := filepath.Join(dir, "test-cover.png")
			file, err := os.Create(coverFile)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 64, 64))); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			withCover := path + ".flac"
			output, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-i", coverFile, "-map", "0:a", "-map", "1:v", "-c", "copy", "-disposition:v:0", "attached_pic", withCover).CombinedOutput()
			if err != nil {
				t.Fatalf("Artwork fixture: %v %s", err, output)
			}
			if err := os.Rename(withCover, path); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := a.nav(ctx, c, "startScan", mapValues("fullScan", "true")); err != nil {
		t.Fatal(err)
	}
	var source Song
	known := map[string]string{}
	for ctx.Err() == nil {
		songs, err := a.navSearch(ctx, c, "Skate separation proof", 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, song := range songs {
			if song.Title == "Skate separation proof 2" {
				source = song
			} else {
				known[song.Artist] = song.ArtistID
			}
		}
		if source.ID != "" && known["Vacio"] != "" && known["ЛСП"] != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if source.ID == "" {
		t.Fatal("Skate fixtures were not indexed")
	}
	artwork := admin.call("GET", "/api/v2/metadata/songs/"+source.ID+"/cover", "", "")
	expect(t, artwork, 200)
	if _, _, err := image.DecodeConfig(bytes.NewReader(artwork.Body.Bytes())); err != nil {
		t.Fatalf("Current artwork did not decode: %v", err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(map[string]any{"source": source.ArtistID, "role": "artist", "name": "Vacio, ЛСП", "artists": []string{"Vacio", "ЛСП"}}), ""), 202)
	a.runMetadata(ctx)
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s: %s", state, message)
	}
	for ctx.Err() == nil {
		songs, err := a.navSearch(ctx, c, "Skate separation proof 2", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(songs) == 1 && len(songs[0].Artists) == 2 && songs[0].Artists[0].ID == known["Vacio"] && songs[0].Artists[1].ID == known["ЛСП"] && songs[0].AlbumArtist == "Various Artists" {
			report := admin.call("GET", "/api/v2/metadata/duplicates?refresh=true", "", "")
			expect(t, report, 200)
			var data struct{ CreditIssues []creditIssue }
			if err := json.Unmarshal(report.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			for _, issue := range data.CreditIssues {
				if issue.Role == "artist" && issue.Name == "Vacio, ЛСП" {
					t.Fatal("Resolved Skate issue remains in review")
				}
			}
			updated := testRealNavidromeArtistRows(t, a, admin, c, songs[0])
			testRealNavidromeCoverChanges(t, a, admin, c, updated)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Navidrome did not connect Skate to the two existing artist entries")
}

func nativeComposers(t *testing.T, path string, names []string) []string {
	t.Helper()
	const script = `
import sys,json,mutagen
from mutagen.id3 import ID3,TCOM
from mutagen.mp4 import MP4Tags
a=mutagen.File(sys.argv[1])
names=json.loads(sys.argv[2])
if names is not None:
    if isinstance(a.tags,ID3):
        a.tags.add(TCOM(encoding=3,text=names))
        a.save(v2_version=4)
    else:
        a.tags["\xa9wrt" if isinstance(a.tags,MP4Tags) else "composer"]=names
        a.save()
if isinstance(a.tags,ID3): names=list(a.tags["TCOM"].text)
else: names=list(a.tags["\xa9wrt" if isinstance(a.tags,MP4Tags) else "composer"])
print(json.dumps(names))
`
	out, err := exec.Command(metadataPythonPath(), "-c", script, path, mustJSON(names)).CombinedOutput()
	if err != nil {
		t.Fatalf("Composer fixture: %v %s", err, out)
	}
	var result []string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBulkKnownCreditsPreservesBothRoles(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	known := songs["one"]
	known.Artists = []ArtistCredit{{ID: "vacio", Name: "Vacio"}, {ID: "lsp", Name: "ЛСП"}}
	songs["one"] = known
	song := songs["two"]
	song.Artist = "Vacio, ЛСП"
	song.ArtistID = "combined"
	song.Artists = []ArtistCredit{{ID: "combined", Name: "Vacio, ЛСП"}, {ID: "guest", Name: "Guest"}}
	song.AlbumArtist = "Vacio, ЛСП"
	song.AlbumArtists = []ArtistCredit{{ID: "combined", Name: "Vacio, ЛСП"}, {ID: "guest", Name: "Guest"}}
	songs["two"] = song
	path := filepath.Join(a.musicDir, song.Path)
	writeNativeCredits(t, path, nativeCredits{[]string{"Vacio, ЛСП", "Guest"}, []string{"Vacio, ЛСП", "Guest"}, []string{"compound", "guest"}, []string{"compound", "guest"}})
	// An incomplete match must stay available for manual review.
	unknown := known
	unknown.ID = "unknown"
	unknown.Artists = []ArtistCredit{{ID: "partial", Name: "Vacio, Unknown"}}
	songs["unknown"] = unknown
	response := admin.call("POST", "/api/v2/metadata/credits/separate-known", "", "")
	expect(t, response, 202)
	var result struct{ Songs, Issues int }
	json.Unmarshal(response.Body.Bytes(), &result)
	if result.Songs != 1 || result.Issues != 2 {
		t.Fatalf("Wrong batch: %s", response.Body)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", "", ""), 409)
	var getSongs, downloads atomic.Int32
	transport := a.HTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clientCopy := *a.HTTP
	clientCopy.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "getSong.view") {
			getSongs.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "download.view") {
			downloads.Add(1)
		}
		return transport.RoundTrip(r)
	})
	a.HTTP = &clientCopy
	// Credit-only writes must verify native tags in the existing Python
	// process, without launching ffprobe or looking up a merge destination.
	a.ffprobePath = filepath.Join(t.TempDir(), "unexpected-ffprobe")
	if err := os.WriteFile(a.ffprobePath, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	a.runMetadata(context.Background())
	t.Logf("Credit write: %s, getSong=%d, downloads=%d, ffprobe=0", time.Since(started), getSongs.Load(), downloads.Load())
	if getSongs.Load() != 1 || downloads.Load() != 1 {
		t.Fatalf("Repeated verification: getSong=%d download=%d", getSongs.Load(), downloads.Load())
	}

	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s: %s", state, message)
	}
	got := readNativeCredits(t, path)
	if !reflect.DeepEqual(got.Artists, []string{"Vacio", "ЛСП", "Guest"}) || !reflect.DeepEqual(got.AlbumArtists, []string{"Vacio", "ЛСП", "Guest"}) {
		t.Fatalf("Lost a role: %+v", got)
	}
}

func TestBulkKnownCreditsAlreadySeparatedNativeTags(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			a, admin, songs := metadataLibraryFixture(t)
			known := songs["one"]
			known.Artists = []ArtistCredit{{ID: "flesh", Name: "FLESH"}, {ID: "lizer", Name: "LIZER"}}
			songs["one"] = known
			song := songs["two"]
			song.Artist = "FLESH, LIZER"
			song.ArtistID = "combined"
			song.Artists = []ArtistCredit{{ID: "combined", Name: "FLESH, LIZER"}}
			song.AlbumArtist = "FLESH, LIZER"
			song.AlbumArtists = []ArtistCredit{{ID: "combined", Name: "FLESH, LIZER"}}
			songs["two"] = song
			path := filepath.Join(a.musicDir, song.Path)
			artists := []string{"FLESH", "LIZER", "Guest"}
			if partial {
				artists = []string{"FLESH", "Guest"}
			}
			before := nativeCredits{artists, []string{"FLESH, LIZER", "Album guest"}, []string{"flesh", "lizer", "guest"}[:len(artists)], []string{"compound", "album-guest"}}
			writeNativeCredits(t, path, before)
			audio, err := metadataAudioHash(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", "", ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			got := readNativeCredits(t, path)
			if partial {
				if state != "failed" || !reflect.DeepEqual(got, before) {
					t.Fatalf("Unsafe partial match: %s %s %+v", state, message, got)
				}
				return
			}
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			if !reflect.DeepEqual(got.Artists, artists) || !reflect.DeepEqual(got.ArtistIDs, before.ArtistIDs) || !reflect.DeepEqual(got.AlbumArtists, []string{"FLESH", "LIZER", "Album guest"}) {
				t.Fatalf("Lost credits: %+v", got)
			}
			after, err := metadataAudioHash(context.Background(), path)
			if err != nil || after != audio {
				t.Fatal("Audio changed", err)
			}
		})
	}
}

func TestCreditRepairRetainsNativeAlbumWithDifferentNavidromeDisplay(t *testing.T) {
	for _, album := range []string{"З4ПР4ВК4 КИД (ДЕЛЮКС П4К)", ""} {
		t.Run(album, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixture(t)
			known := songs["one"]
			known.Artists = []ArtistCredit{{ID: "mayot", Name: "MAYOT"}, {ID: "vosk", Name: "Voskresenskii"}}
			songs["one"] = known
			song := songs["two"]
			song.Artist = "MAYOT feat. Voskresenskii"
			song.Artists = []ArtistCredit{{ID: "combined", Name: song.Artist}}
			song.Album = "Navidrome display album"
			songs["two"] = song
			path := filepath.Join(a.musicDir, song.Path)
			tmp := path + ".flac"
			output, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-c", "copy", "-metadata", "album="+album, tmp).CombinedOutput()
			if err != nil {
				t.Fatalf("%v %s", err, output)
			}
			if err := os.Rename(tmp, path); err != nil {
				t.Fatal(err)
			}
			writeNativeCredits(t, path, nativeCredits{[]string{song.Artist, "Guest"}, []string{"Band"}, []string{"compound", "guest"}, []string{"band"}})
			expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", "", ""), 202)
			a.runMetadata(context.Background())
			var jid, state, message string
			a.DB.QueryRow("SELECT id,state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&jid, &state, &message)
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			probe, err := a.probeUpload(path, uploadFormat(".flac"))
			if err != nil || uploadTags(probe)["album"] != album {
				t.Fatalf("Native album changed: %v %v", uploadTags(probe), err)
			}
			// A retry after a restart must recognize the already published file.
			a.DB.Exec("UPDATE metadata_jobs SET state='failed',done=0 WHERE id=?", jid)
			expect(t, admin.call("POST", "/api/v2/metadata/jobs", mustJSON(map[string]string{"id": jid}), ""), 200)
			a.runMetadata(context.Background())
			var done int
			a.DB.QueryRow("SELECT state,done FROM metadata_jobs WHERE id=?", jid).Scan(&state, &done)
			if state != "completed" || done != 1 {
				t.Fatalf("Retry did not resume: %s %d", state, done)
			}
		})
	}
}

func TestBulkCreditRepairIncludesUnknownOnlyWhenSelected(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	known := songs["one"]
	known.Artists = []ArtistCredit{{ID: "known", Name: "Vacio"}}
	songs["one"] = known
	song := songs["two"]
	song.Artist = "Vacio, New Artist"
	song.Artists = []ArtistCredit{{ID: "combined", Name: song.Artist}}
	songs["two"] = song
	path := filepath.Join(a.musicDir, song.Path)
	writeNativeCredits(t, path, nativeCredits{[]string{song.Artist, "Guest"}, []string{"Band"}, []string{"combined", "guest"}, []string{"band"}})
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", "", ""), 409)
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", `{"includeUnknown":true}`, ""), 202)
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s %s", state, message)
	}
	got := readNativeCredits(t, path)
	if !reflect.DeepEqual(got.Artists, []string{"Vacio", "New Artist", "Guest"}) || !reflect.DeepEqual(got.ArtistIDs, []string{"", "", "guest"}) {
		t.Fatalf("Incorrect new credits: %+v", got)
	}
}

func TestSeparateCollapsedAlbumCreditWithMultipleMBIDs(t *testing.T) {
	a, admin, songs := metadataLibraryFixtureFormat(t, ".flac", "flac", "")
	known := songs["one"]
	known.Artists = []ArtistCredit{{ID: "solo", Name: "163ONMYNECK"}}
	songs["one"] = known
	song := songs["two"]
	song.AlbumArtist = "163ONMYNECK & Polyana"
	song.AlbumArtists = []ArtistCredit{{ID: "combined", Name: song.AlbumArtist}}
	songs["two"] = song
	path := filepath.Join(a.musicDir, song.Path)
	before := nativeCredits{[]string{song.Artist}, []string{song.AlbumArtist}, []string{"track-id"}, []string{"first-id", "second-id"}}
	writeNativeCredits(t, path, before)
	audio, err := metadataAudioHash(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(map[string]any{"source": "combined", "name": song.AlbumArtist, "role": "album artist", "artists": []string{"163ONMYNECK", "Polyana"}}), ""), 202)
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("%s: %s", state, message)
	}
	want := nativeCredits{before.Artists, []string{"163ONMYNECK", "Polyana"}, before.ArtistIDs, nil}
	got := readNativeCredits(t, path)
	if !reflect.DeepEqual(got.Artists, want.Artists) || !reflect.DeepEqual(got.AlbumArtists, want.AlbumArtists) || !reflect.DeepEqual(got.ArtistIDs, want.ArtistIDs) || len(got.AlbumArtistIDs) != 0 {
		t.Fatalf("Unexpected credits: %+v", got)
	}
	after, err := metadataAudioHash(context.Background(), path)
	if err != nil || audio != after {
		t.Fatalf("Audio changed: %v", err)
	}
}

func TestSeparateMixedCreditsRejectsAmbiguousMBIDs(t *testing.T) {
	a, admin, songs := metadataLibraryFixtureFormat(t, ".flac", "flac", "")
	known := songs["one"]
	known.Artists = []ArtistCredit{{ID: "solo", Name: "163ONMYNECK"}}
	songs["one"] = known
	song := songs["two"]
	song.AlbumArtist = "163ONMYNECK & Polyana / Guest"
	song.AlbumArtists = []ArtistCredit{{ID: "combined", Name: "163ONMYNECK & Polyana"}, {ID: "guest", Name: "Guest"}}
	songs["two"] = song
	path := filepath.Join(a.musicDir, song.Path)
	writeNativeCredits(t, path, nativeCredits{[]string{song.Artist}, []string{"163ONMYNECK & Polyana", "Guest"}, []string{"track-id"}, []string{"first-id", "second-id", "guest-id"}})
	before, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(map[string]any{"source": "combined", "name": "163ONMYNECK & Polyana", "role": "album artist", "artists": []string{"163ONMYNECK", "Polyana"}}), ""), 202)
	a.runMetadata(context.Background())
	var state string
	a.DB.QueryRow("SELECT state FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state)
	after, err := hashFile(path)
	if state != "failed" || err != nil || before != after {
		t.Fatalf("Ambiguous IDs must leave original intact: state=%s err=%v", state, err)
	}
}
