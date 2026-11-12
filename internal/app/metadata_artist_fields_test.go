package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMetadataArtistRowsPreserveIndividualCredits(t *testing.T) {
	for _, format := range []struct{ ext, codec, rate string }{{".flac", "flac", ""}, {".mp3", "mp3", "192k"}, {".m4a", "aac", "192k"}} {
		t.Run(format.ext, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixtureFormat(t, format.ext, format.codec, format.rate)
			song := songs["one"]
			song.Artist = "May Waves / Guest • Composer"
			song.Artists = []ArtistCredit{{ID: "may", Name: "May Waves"}, {ID: "guest", Name: "Guest"}}
			song.AlbumArtist = "May Waves"
			song.AlbumArtists = []ArtistCredit{{ID: "may", Name: "May Waves"}}
			songs["one"] = song
			path := filepath.Join(a.musicDir, song.Path)
			writeNativeCredits(t, path, nativeCredits{[]string{"May Waves", "Guest"}, []string{"May Waves"}, []string{"may-id", "guest-id"}, []string{"may-id"}})
			nativeComposers(t, path, []string{"Composer One", "Composer Two"})
			audio, err := metadataAudioHash(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			response := admin.call("GET", "/api/v2/metadata/songs/one", "", "")
			expect(t, response, 200)
			var data struct{ Edit MetadataEdit }
			if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(data.Edit.ArtistNames, []string{"May Waves", "Guest"}) {
				t.Fatalf("Joined display used as a single field: %+v", data.Edit)
			}
			data.Edit.Title = "Corrected title"
			expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(data.Edit), ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			original := readNativeCredits(t, path)
			if !reflect.DeepEqual(original.Artists, []string{"May Waves", "Guest"}) || !reflect.DeepEqual(original.ArtistIDs, []string{"may-id", "guest-id"}) {
				t.Fatal("Title edit collapsed credits", original)
			}
			// Each row remains one literal name, including punctuation and slash.
			data.Edit.ArtistNames = []string{"Guest", "May Wave$", "AC/DC"}
			data.Edit.AlbumArtistNames = []string{"May Wave$", "Guest"}
			expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(data.Edit), ""), 202)
			a.runMetadata(context.Background())
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s %s", state, message)
			}
			got := readNativeCredits(t, path)
			if !reflect.DeepEqual(got.Artists, data.Edit.ArtistNames) || !reflect.DeepEqual(got.ArtistIDs, []string{"guest-id", "", ""}) || !reflect.DeepEqual(got.AlbumArtists, data.Edit.AlbumArtistNames) {
				t.Fatalf("Wrong row identities/order: %+v", got)
			}
			data.Edit.AlbumArtistNames = []string{}
			expect(t, admin.call("PUT", "/api/v2/metadata/songs/one", mustJSON(data.Edit), ""), 202)
			a.runMetadata(context.Background())
			cleared := readNativeCredits(t, path)
			if len(cleared.AlbumArtists) != 0 || len(cleared.AlbumArtistIDs) != 0 {
				t.Fatal("Removing album artist rows did not clear native credits", cleared)
			}
			if got := nativeComposers(t, path, nil); len(got) != 2 {
				t.Fatal("Composer metadata changed", got)
			}
			after, err := metadataAudioHash(context.Background(), path)
			if err != nil || audio != after {
				t.Fatal("Audio changed", err)
			}
			assertNoMetadataCopies(t, a)
		})
	}
}

func testRealNavidromeArtistRows(t *testing.T, a *App, admin client, c Credentials, song Song) Song {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	response := admin.call("GET", "/api/v2/metadata/songs/"+song.ID, "", "")
	expect(t, response, 200)
	var current struct{ Edit MetadataEdit }
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	expected := []string{"Vacio", "AC/DC", "ЛСП"}
	current.Edit.ArtistNames = expected
	expect(t, admin.call("PUT", "/api/v2/metadata/songs/"+song.ID, mustJSON(current.Edit), ""), 202)
	a.runMetadata(ctx)
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
	if state != "completed" {
		t.Fatalf("Artist rows: %s %s", state, message)
	}
	for ctx.Err() == nil {
		remote, err := a.nav(ctx, c, "getSong", mapValues("id", song.ID))
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(creditNames(remote.Song.Artists), expected) {
			return remote.Song
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Navidrome did not index AC/DC as one individual artist row")
	return Song{}
}
