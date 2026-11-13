package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAlbumRenamePreservesReleaseIdentityAcrossOriginalTitles(t *testing.T) {
	// Navidrome can group files with different stored titles by release ID.
	// Renaming that album must preserve the identity on every member, including
	// members whose original title already matches the requested name.
	for _, format := range []struct{ ext, codec, rate string }{{".flac", "flac", ""}, {".mp3", "mp3", "192k"}, {".m4a", "aac", "192k"}} {
		t.Run(format.ext, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixtureFormat(t, format.ext, format.codec, format.rate)
			second := songs["two"]
			second.AlbumID = songs["one"].AlbumID
			songs["two"] = second
			const script = `
import json,sys,mutagen
from mutagen.id3 import ID3,TXXX
from mutagen.mp4 import MP4Tags,MP4FreeForm
audio=mutagen.File(sys.argv[1])
fields=[("musicbrainz_albumid","MusicBrainz Album Id"),("musicbrainz_releasegroupid","MusicBrainz Release Group Id")]
expected=["00000000-0000-4000-8000-000000000001","00000000-0000-4000-8000-000000000002"]
if sys.argv[2]=="write":
    for (key,desc),value in zip(fields,expected):
        if isinstance(audio.tags,ID3): audio.tags.add(TXXX(encoding=3,desc=desc,text=[value]))
        elif isinstance(audio.tags,MP4Tags): audio.tags["----:com.apple.iTunes:"+desc]=[MP4FreeForm(value.encode())]
        else: audio.tags[key]=[value]
    audio.save()
else:
    result=[]
    for key,desc in fields:
        if isinstance(audio.tags,ID3): values=list(audio.tags["TXXX:"+desc].text) if "TXXX:"+desc in audio.tags else []
        elif isinstance(audio.tags,MP4Tags): values=[v.decode() for v in audio.tags.get("----:com.apple.iTunes:"+desc,[])]
        else: values=list(audio.tags.get(key,[]))
        result.append(values)
    print(json.dumps(result))
`
			for _, s := range songs {
				if out, err := exec.Command(metadataPythonPath(), "-c", script, filepath.Join(a.musicDir, s.Path), "write").CombinedOutput(); err != nil {
					t.Fatalf("seed release identity: %v %s", err, out)
				}
			}
			expect(t, admin.call("PUT", "/api/v2/metadata/albums/album-one", `{"album":"Album!"}`, ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			if err := a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message); err != nil || state != "completed" {
				t.Fatal(state, message, err)
			}
			for _, s := range songs {
				path := filepath.Join(a.musicDir, s.Path)
				out, err := exec.Command(metadataPythonPath(), "-c", script, path, "read").CombinedOutput()
				if err != nil {
					t.Fatalf("read release identity: %v %s", err, out)
				}
				var ids [][]string
				if err := json.Unmarshal(out, &ids); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(ids, [][]string{{"00000000-0000-4000-8000-000000000001"}, {"00000000-0000-4000-8000-000000000002"}}) {
					t.Errorf("rename changed release identity for %s (original album %q): %v", s.ID, s.Album, ids)
				}
				probe, err := a.probeUpload(path, uploadFormat(format.ext))
				if err != nil {
					t.Fatal(err)
				}
				tags := uploadTags(probe)
				if tags["album"] != "Album!" || tags["artist"] != s.Artist {
					t.Fatalf("wrong renamed album tags: %v", tags)
				}
			}
		})
	}
}

func TestAlbumMetadataEditUpdatesCollectionWithoutChangingSongs(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	other := songs["two"]
	other.AlbumID = songs["one"].AlbumID
	songs["two"] = other
	expect(t, admin.call("GET", "/api/v2/metadata/albums/album-one", "", ""), 200)
	var cover bytes.Buffer
	if err := png.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	upload := admin.call("POST", "/api/v2/metadata/songs/one/cover-upload", cover.String(), "")
	expect(t, upload, 201)
	var uploaded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("PUT", "/api/v2/metadata/albums/album-one", mustJSON(MetadataEdit{Album: "Renamed album", AlbumArtistNames: []string{"Album artist"}, Title: "Must not replace song titles", Artist: "Must not replace credits", Lyrics: "Must not replace lyrics", CoverUploadID: uploaded.ID}), ""), 202)
	a.runMetadata(context.Background())
	var state, message string
	if err := a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message); err != nil || state != "completed" {
		t.Fatal(state, message, err)
	}
	for _, s := range songs {
		p, err := a.probeUpload(filepath.Join(a.musicDir, s.Path), "flac")
		if err != nil {
			t.Fatal(err)
		}
		hasCover := false
		for _, stream := range p.Streams {
			if stream.Disposition.AttachedPic == 1 {
				hasCover = true
			}
		}
		if !hasCover {
			t.Fatal("album artwork was not embedded in every file")
		}
		tags := uploadTags(p)
		if tags["album"] != "Renamed album" || tags["artist"] != s.Artist || tags["lyrics"] != "" || tags["title"] == "Must not replace song titles" {
			t.Fatalf("bulk edit changed song fields: %+v", tags)
		}
	}
	expect(t, admin.call("GET", "/api/v2/metadata/jobs", "", ""), 200)
	expect(t, admin.call("GET", "/api/v2/activity", "", ""), 200)
}

func TestArtistRenameTargetsCreditsAndPreservesCollaborators(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	first := songs["one"]
	first.Artists = []ArtistCredit{{ID: "artist-one", Name: "Band"}, {ID: "guest", Name: "Guest"}}
	first.AlbumArtists = []ArtistCredit{{ID: "artist-one", Name: "Band"}}
	songs["one"] = first
	second := songs["two"]
	second.Artists = []ArtistCredit{{ID: "other", Name: "band"}}
	second.AlbumArtists = []ArtistCredit{{ID: "artist-one", Name: "Band"}, {ID: "guest", Name: "Guest"}}
	songs["two"] = second
	writeNativeCredits(t, filepath.Join(a.musicDir, first.Path), nativeCredits{Artists: []string{"Band", "Guest"}, AlbumArtists: []string{"Band"}, ArtistIDs: []string{"band-id", "guest-id"}, AlbumArtistIDs: []string{"band-id"}})
	writeNativeCredits(t, filepath.Join(a.musicDir, second.Path), nativeCredits{Artists: []string{"band"}, AlbumArtists: []string{"Band", "Guest"}, ArtistIDs: []string{"other-id"}, AlbumArtistIDs: []string{"band-id", "guest-id"}})
	expect(t, admin.call("PUT", "/api/v2/metadata/artists/artist-one", `{"name":"New artist"}`, ""), 202)
	var raw string
	if err := a.DB.QueryRow("SELECT changes FROM metadata_jobs").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var changes []metadataChange
	if err := json.Unmarshal([]byte(raw), &changes); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatal("rename missed album-artist-only credit", len(changes))
	}
	if changes[0].Edit.Artist != "New artist / Guest" || changes[0].Edit.AlbumArtist != "New artist" {
		t.Fatalf("wrong credits: %+v", changes[0])
	}
	if changes[1].Edit.Artist != "band" || changes[1].Edit.AlbumArtist != "New artist / Guest" {
		t.Fatalf("unrelated artist changed: %+v", changes[1])
	}
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message)
	if state != "completed" {
		t.Fatal(state, message)
	}
	for _, song := range []Song{first, second} {
		actual := readNativeCredits(t, filepath.Join(a.musicDir, song.Path))
		if song.ID == first.ID && (len(actual.Artists) != 2 || actual.Artists[0] != "New artist" || actual.Artists[1] != "Guest" || len(actual.ArtistIDs) != 2 || actual.ArtistIDs[1] != "guest-id") {
			t.Fatalf("collaborator lost: %+v", actual)
		}
		if song.ID == second.ID && (len(actual.AlbumArtists) != 2 || actual.AlbumArtists[0] != "New artist" || actual.AlbumArtists[1] != "Guest" || actual.ArtistIDs[0] != "other-id") {
			t.Fatalf("unrelated credit lost: %+v", actual)
		}
	}

}

func TestArtistRenameWritesTags(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	expect(t, admin.call("PUT", "/api/v2/metadata/artists/artist-one", `{"name":"Renamed"}`, ""), 202)
	a.runMetadata(context.Background())
	var state, message string
	a.DB.QueryRow("SELECT state,message FROM metadata_jobs").Scan(&state, &message)
	if state != "completed" {
		t.Fatal(state, message)
	}
	p, err := a.probeUpload(filepath.Join(a.musicDir, songs["one"].Path), "flac")
	if err != nil {
		t.Fatal(err)
	}
	tags := uploadTags(p)
	if tags["artist"] != "Renamed" || tags["album"] != "Album" {
		t.Fatal(tags)
	}
	p, err = a.probeUpload(filepath.Join(a.musicDir, songs["two"].Path), "flac")
	if err != nil {
		t.Fatal(err)
	}
	if uploadTags(p)["artist"] != "band" {
		t.Fatal("unrelated ID with same name was renamed")
	}
}

func TestCollectionMetadataPermissionsAndValidation(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	for _, path := range []string{"/api/v2/metadata/albums/album-one", "/api/v2/metadata/artists/artist-one"} {
		expect(t, member.call("GET", path, "", ""), 403)
		expect(t, member.call("PUT", path, `{}`, ""), 403)
	}
	expect(t, admin.call("PUT", "/api/v2/metadata/artists/artist-one", `{"name":" "}`, ""), 400)
	expect(t, admin.call("PUT", "/api/v2/metadata/albums/album-one", `{"album":""}`, ""), 400)
	expect(t, admin.call("PUT", "/api/v2/metadata/albums/album-one", `{"album":"Album","albumArtistNames":["A","A"]}`, ""), 400)
}

func TestAlbumUnchangedFieldsPreservePerSongCredits(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	second := songs["two"]
	second.AlbumID = songs["one"].AlbumID
	songs["two"] = second
	expect(t, admin.call("PUT", "/api/v2/metadata/albums/album-one", `{"album":"Album","albumArtistNames":["Band"]}`, ""), 202)
	var raw string
	if err := a.DB.QueryRow("SELECT changes FROM metadata_jobs").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var changes []metadataChange
	if err := json.Unmarshal([]byte(raw), &changes); err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if change.Edit.Album != change.Song.Album || change.Edit.AlbumArtist != change.Song.AlbumArtist || change.Edit.AlbumArtistNames != nil {
			t.Fatalf("unchanged collection field overwrote song tags: %+v", change)
		}
	}
}
