package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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
