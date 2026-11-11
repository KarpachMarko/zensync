package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type nativeCredits struct {
	Artists        []string `json:"artists"`
	AlbumArtists   []string `json:"albumartists"`
	ArtistIDs      []string `json:"artistids"`
	AlbumArtistIDs []string `json:"albumartistids"`
}

// Independent native-format inspection, not the production reader/writer.
const nativeCreditTestScript = `
import json,sys,mutagen
from mutagen.id3 import ID3,TPE1,TPE2,TXXX
from mutagen.mp4 import MP4Tags,MP4FreeForm
audio=mutagen.File(sys.argv[1])
if audio.tags is None: audio.add_tags()
tags=audio.tags
fields=["artists","albumartists","artistids","albumartistids"]
desc=["ARTISTS","ALBUM ARTISTS","MusicBrainz Artist Id","MusicBrainz Album Artist Id"]
generic=["artists","albumartists","musicbrainz_artistid","musicbrainz_albumartistid"]
mp4=["----:com.apple.iTunes:ARTISTS","----:com.apple.iTunes:ALBUMARTISTS","----:com.apple.iTunes:MusicBrainz Artist Id","----:com.apple.iTunes:MusicBrainz Album Artist Id"]
if sys.argv[2]=="write":
    data=json.load(sys.stdin)
    for field,d,g,m in zip(fields,desc,generic,mp4):
        values=data[field]
        if isinstance(tags,ID3):
            normal=lambda value:value.lower().replace(" ","").replace("_","")
            for key in list(tags.keys()):
                if key.startswith("TXXX:") and normal(key[5:])==normal(d): del tags[key]
            tags.add(TXXX(encoding=3,desc=d,text=values))
        elif isinstance(tags,MP4Tags): tags[m]=[MP4FreeForm(v.encode()) for v in values]
        else: tags[g]=values
    if isinstance(tags,ID3):
        tags.add(TPE1(encoding=3,text=[" / ".join(data["artists"])]))
        tags.add(TPE2(encoding=3,text=[" / ".join(data["albumartists"])]))
        audio.save(v2_version=4)
    else:
        if isinstance(tags,MP4Tags):
            tags["\xa9ART"]=[" / ".join(data["artists"])]
            tags["aART"]=[" / ".join(data["albumartists"])]
        else:
            tags["artist"]=[" / ".join(data["artists"])]
            tags["albumartist"]=[" / ".join(data["albumartists"])]
            tags["album_artist"]=[" / ".join(data["albumartists"])]
        audio.save()
else:
    result={}
    for field,d,g,m in zip(fields,desc,generic,mp4):
        if isinstance(tags,ID3): values=list(tags["TXXX:"+d].text) if "TXXX:"+d in tags else []
        elif isinstance(tags,MP4Tags): values=[v.decode() for v in tags.get(m,[])]
        else: values=list(tags.get(g,[]))
        result[field]=values
    print(json.dumps(result))
`

func writeNativeCredits(t *testing.T, path string, credits nativeCredits) {
	t.Helper()
	cmd := exec.Command(metadataPythonPath(), "-c", nativeCreditTestScript, path, "write")
	cmd.Stdin = bytes.NewBufferString(mustJSON(credits))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed native credits: %v %s", err, out)
	}
}
func readNativeCredits(t *testing.T, path string) nativeCredits {
	t.Helper()
	out, err := exec.Command(metadataPythonPath(), "-c", nativeCreditTestScript, path, "read").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect native credits: %v %s", err, out)
	}
	var credits nativeCredits
	if err := json.Unmarshal(out, &credits); err != nil {
		t.Fatal(err)
	}
	return credits
}

func TestMultiArtistMergeThenAlbumPreservesIndividualCredits(t *testing.T) {
	const sourceMBID = "00000000-0000-4000-8000-000000000001"
	const guestMBID = "00000000-0000-4000-8000-000000000002"
	const targetMBID = "00000000-0000-4000-8000-000000000003"
	for _, format := range []struct{ ext, codec, rate string }{{".flac", "flac", ""}, {".mp3", "mp3", "192k"}, {".m4a", "aac", "192k"}} {
		for _, order := range [][]string{{"artist", "album"}, {"album", "artist"}} {
			for _, secondary := range []bool{false, true} {
				name := format.ext + "/" + order[0]
				if secondary {
					name += "/secondary"
				}
				t.Run(name, func(t *testing.T) {
					a, admin, songs := metadataLibraryFixtureFormat(t, format.ext, format.codec, format.rate)
					target := songs["one"]
					target.Artist, target.AlbumArtist = "May Wave$", "May Wave$"
					target.Artists = []ArtistCredit{{ID: "artist-one", Name: "May Wave$"}}
					target.AlbumArtists = target.Artists
					songs["one"] = target
					source := songs["two"]
					source.Artist, source.AlbumArtist = "May Waves", "May Waves"
					source.Artists = []ArtistCredit{{ID: "artist-two", Name: "May Waves"}, {ID: "guest", Name: "Collaborator"}}
					before := nativeCredits{[]string{"May Waves", "Collaborator"}, []string{"May Waves"}, []string{sourceMBID, guestMBID}, []string{sourceMBID}}
					expected := nativeCredits{[]string{"May Wave$", "Collaborator"}, []string{"May Wave$"}, []string{targetMBID, guestMBID}, []string{targetMBID}}
					if secondary {
						source.Artist, source.ArtistID = "Collaborator", "guest"
						source.Artists = []ArtistCredit{source.Artists[1], source.Artists[0]}
						before.Artists = []string{"Collaborator", "May Waves"}
						before.ArtistIDs = []string{guestMBID, sourceMBID}
						expected.Artists = []string{"Collaborator", "May Wave$"}
						expected.ArtistIDs = []string{guestMBID, targetMBID}
					}
					if secondary {
						source.Artists = append(source.Artists, ArtistCredit{ID: "guest-two", Name: "Another collaborator"})
						before.Artists = append(before.Artists, "Another collaborator")
						before.ArtistIDs = append(before.ArtistIDs, "00000000-0000-4000-8000-000000000004")
						expected.Artists = append(expected.Artists, "Another collaborator")
						expected.ArtistIDs = append(expected.ArtistIDs, "00000000-0000-4000-8000-000000000004")
					}
					source.AlbumArtists = []ArtistCredit{{ID: "artist-two", Name: "May Waves"}}
					songs["two"] = source
					path := filepath.Join(a.musicDir, source.Path)
					writeNativeCredits(t, path, before)
					writeNativeCredits(t, filepath.Join(a.musicDir, target.Path), nativeCredits{[]string{"May Wave$"}, []string{"May Wave$"}, []string{targetMBID}, []string{targetMBID}})
					audioBefore, err := metadataAudioHash(context.Background(), path)
					if err != nil {
						t.Fatal(err)
					}
					for _, kind := range order {
						expect(t, admin.call("POST", "/api/v2/metadata/merge", mustJSON(map[string]string{"kind": kind, "source": kind + "-two", "target": kind + "-one"}), ""), 202)
						a.runMetadata(context.Background())
						var state, message string
						a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
						if state != "completed" {
							t.Fatalf("%s: %s", state, message)
						}
						got := readNativeCredits(t, path)
						want := expected
						if kind == "album" && order[0] == "album" {
							want.Artists = before.Artists
							want.ArtistIDs = before.ArtistIDs
						}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("After %s merge:\ngot %+v\nwant %+v", kind, got, want)
						}
						// Simulate rescan from the verified individual file credits.
						s := songs["two"]
						s.Artist = got.Artists[0]
						s.AlbumArtist = got.AlbumArtists[0]
						if kind == "artist" {
							for i := range s.Artists {
								if s.Artists[i].ID == "artist-two" {
									s.Artists[i] = ArtistCredit{ID: "artist-one", Name: "May Wave$"}
								}
							}
							if !secondary {
								s.ArtistID = "artist-one"
							}
						}
						s.AlbumArtists = []ArtistCredit{{ID: "artist-one", Name: "May Wave$"}}
						if kind == "album" {
							s.Album = target.Album
							s.AlbumID = target.AlbumID
						}
						songs["two"] = s
						a.navIndexMu.Lock()
						clear(a.navIndexes)
						a.navIndexMu.Unlock()
					}
					audioAfter, err := metadataAudioHash(context.Background(), path)
					if err != nil || audioBefore != audioAfter {
						t.Fatal("Audio changed", err)
					}
				})
			}
		}
	}
}

// These formats also store native multivalue credits; cover the writer directly
// without multiplying the API/worker scenarios above.
func TestNativeMergeCreditsOtherFormats(t *testing.T) {
	for _, format := range []struct{ ext, codec string }{{".ogg", "libvorbis"}, {".opus", "libopus"}, {".wav", "pcm_s16le"}, {".aiff", "pcm_s16be"}} {
		t.Run(format.ext, func(t *testing.T) {
			dir := t.TempDir()
			original, target, output := filepath.Join(dir, "original"+format.ext), filepath.Join(dir, "target"+format.ext), filepath.Join(dir, "output"+format.ext)
			data := nativeFixtureAudio(t, format.codec, "", format.ext)
			for _, path := range []string{original, target, output} {
				if err := os.WriteFile(path, data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			before := nativeCredits{[]string{"May Waves", "Collaborator"}, []string{"May Waves"}, []string{"source", "guest"}, []string{"source"}}
			dest := nativeCredits{[]string{"May Wave$"}, []string{"May Wave$"}, []string{"target"}, []string{"target"}}
			writeNativeCredits(t, original, before)
			writeNativeCredits(t, target, dest)
			change := metadataChange{MergeKind: "artist", Target: &Song{}, ArtistMerge: &artistMergeIdentity{SourceName: "May Waves", TargetName: "May Wave$", ReplaceTrack: true, ReplaceAlbum: true}}
			if err := preserveMergeCredits(context.Background(), &change, original, target, output); err != nil {
				t.Fatal(err)
			}
			beforeAudio, err := metadataAudioHash(context.Background(), original)
			if err != nil {
				t.Fatal(err)
			}
			afterAudio, err := metadataAudioHash(context.Background(), output)
			if err != nil || beforeAudio != afterAudio {
				t.Fatal("Native writer changed audio", err)
			}
			want := nativeCredits{[]string{"May Wave$", "Collaborator"}, []string{"May Wave$"}, []string{"target", "guest"}, []string{"target"}}
			if got := readNativeCredits(t, output); !reflect.DeepEqual(got, want) {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestArtistMergeAlbumRoleAndDestinationCollaborator(t *testing.T) {
	for _, mode := range []string{"primary", "album", "secondary"} {
		albumOnly := mode == "album"
		name := mode + " source, secondary destination"
		t.Run(name, func(t *testing.T) {
			a, admin, songs := metadataLibraryFixture(t)
			target := songs["one"]
			target.ArtistID, target.Artist, target.AlbumArtist = "other", "Other", "Various Artists"
			target.Artists = []ArtistCredit{{ID: "other", Name: "Other"}, {ID: "artist-one", Name: "May Wave$"}}
			target.AlbumArtists = []ArtistCredit{{ID: "various", Name: "Various Artists"}}
			songs["one"] = target
			source := songs["two"]
			source.Artist, source.AlbumArtist = "May Waves", "May Waves"
			source.Artists = []ArtistCredit{{ID: "artist-two", Name: "May Waves"}, {ID: "guest", Name: "Collaborator"}}
			source.AlbumArtists = []ArtistCredit{{ID: "artist-two", Name: "May Waves"}}
			before := nativeCredits{[]string{"May Waves", "Collaborator"}, []string{"May Waves"}, []string{"source", "guest"}, []string{"source"}}
			want := nativeCredits{[]string{"May Wave$", "Collaborator"}, []string{"May Wave$"}, []string{"target", "guest"}, []string{"target"}}
			if albumOnly {
				source.ArtistID, source.Artist = "guest", "Collaborator"
				source.Artists = []ArtistCredit{{ID: "guest", Name: "Collaborator"}}
				before.Artists, before.ArtistIDs = []string{"Collaborator"}, []string{"guest"}
				want.Artists, want.ArtistIDs = before.Artists, before.ArtistIDs
			}
			if mode == "secondary" {
				source.ArtistID, source.Artist = "guest", "Collaborator"
				source.Artists = []ArtistCredit{{ID: "guest", Name: "Collaborator"}, {ID: "artist-two", Name: "May Waves"}}
				source.AlbumArtist = "Various Artists"
				source.AlbumArtists = []ArtistCredit{{ID: "various", Name: "Various Artists"}}
				before = nativeCredits{[]string{"Collaborator", "May Waves"}, []string{"Various Artists"}, []string{"guest", "source"}, []string{"various"}}
				want = nativeCredits{[]string{"Collaborator", "May Wave$"}, []string{"Various Artists"}, []string{"guest", "target"}, []string{"various"}}
			}
			songs["two"] = source
			path := filepath.Join(a.musicDir, source.Path)
			writeNativeCredits(t, path, before)
			writeNativeCredits(t, filepath.Join(a.musicDir, target.Path), nativeCredits{[]string{"Other", "May Wave$"}, []string{"Various Artists"}, []string{"other", "target"}, []string{"various"}})
			expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"artist","source":"artist-two","target":"artist-one"}`, ""), 202)
			a.runMetadata(context.Background())
			var state, message string
			a.DB.QueryRow("SELECT state,message FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &message)
			if state != "completed" {
				t.Fatalf("%s: %s", state, message)
			}
			if got := readNativeCredits(t, path); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v want %+v", got, want)
			}
		})
	}
}

func TestArtistMergeRefusesAmbiguousCombinedCredit(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	source, target := songs["two"], songs["one"]
	source.Artist, source.AlbumArtist = "May Waves", "May Waves"
	source.Artists = []ArtistCredit{{ID: "artist-two", Name: "May Waves"}, {ID: "guest", Name: "Collaborator"}}
	source.AlbumArtists = []ArtistCredit{{ID: "artist-two", Name: "May Waves"}}
	target.Artist, target.AlbumArtist = "May Wave$", "May Wave$"
	songs["one"], songs["two"] = target, source
	path := filepath.Join(a.musicDir, source.Path)
	writeNativeCredits(t, path, nativeCredits{[]string{"May Waves, Collaborator"}, []string{"May Waves"}, []string{"source"}, []string{"source"}})
	writeNativeCredits(t, filepath.Join(a.musicDir, target.Path), nativeCredits{[]string{"May Wave$"}, []string{"May Wave$"}, []string{"target"}, []string{"target"}})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/merge", `{"kind":"artist","source":"artist-two","target":"artist-one"}`, ""), 202)
	a.runMetadata(context.Background())
	var state string
	var done int
	a.DB.QueryRow("SELECT state,done FROM metadata_jobs ORDER BY rowid DESC LIMIT 1").Scan(&state, &done)
	after, err := os.ReadFile(path)
	if err != nil || state != "failed" || done != 0 || !bytes.Equal(before, after) {
		t.Fatalf("Ambiguous credits must leave original intact: %s %d %v", state, done, err)
	}
}

func TestArtistMergeDeduplicatesDestinationCredit(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "original.flac"), filepath.Join(dir, "target.flac"), filepath.Join(dir, "output.flac")}
	data := nativeFixtureAudio(t, "flac", "", ".flac")
	for _, path := range paths {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeNativeCredits(t, paths[0], nativeCredits{[]string{"May Waves", "May Wave$", "Collaborator"}, []string{"May Waves"}, []string{"source", "target", "guest"}, []string{"source"}})
	writeNativeCredits(t, paths[1], nativeCredits{[]string{"May Wave$"}, []string{"May Wave$"}, []string{"target"}, []string{"target"}})
	change := metadataChange{MergeKind: "artist", Target: &Song{}, ArtistMerge: &artistMergeIdentity{SourceName: "May Waves", TargetName: "May Wave$", ReplaceTrack: true, ReplaceAlbum: true}}
	if err := preserveMergeCredits(context.Background(), &change, paths[0], paths[1], paths[2]); err != nil {
		t.Fatal(err)
	}
	want := nativeCredits{[]string{"May Wave$", "Collaborator"}, []string{"May Wave$"}, []string{"target", "guest"}, []string{"target"}}
	if got := readNativeCredits(t, paths[2]); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestArtistMergeUsesStructuredCreditsForLegacyDisplayTag(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "original.flac"), filepath.Join(dir, "target.flac"), filepath.Join(dir, "output.flac")}
	data := nativeFixtureAudio(t, "flac", "", ".flac")
	for _, path := range paths {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeNativeCredits(t, paths[0], nativeCredits{[]string{"May Waves", "Collaborator"}, []string{"May Waves"}, []string{"source", "guest"}, []string{"source"}})
	writeNativeCredits(t, paths[1], nativeCredits{[]string{"May Wave$"}, []string{"May Wave$"}, []string{"target"}, []string{"target"}})
	cmd := exec.Command(metadataPythonPath(), "-c", "from mutagen.flac import FLAC; import sys; a=FLAC(sys.argv[1]); del a['artists']; a.save()", paths[0])
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	change := metadataChange{
		Song:      Song{Artists: []ArtistCredit{{ID: "source", Name: "May Waves"}, {ID: "guest", Name: "Collaborator"}}},
		MergeKind: "artist", Target: &Song{},
		ArtistMerge: &artistMergeIdentity{SourceName: "May Waves", TargetName: "May Wave$", ReplaceTrack: true, ReplaceAlbum: true},
	}
	if err := preserveMergeCredits(context.Background(), &change, paths[0], paths[1], paths[2]); err != nil {
		t.Fatal(err)
	}
	want := nativeCredits{[]string{"May Wave$", "Collaborator"}, []string{"May Wave$"}, []string{"target", "guest"}, []string{"target"}}
	if got := readNativeCredits(t, paths[2]); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}
