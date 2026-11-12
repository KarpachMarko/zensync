package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetadataDuplicateCandidates(t *testing.T) {
	songs := []Song{
		{ID: "1", ArtistID: "a", AlbumID: "x", Artist: "The Band", Album: "The Album", Title: "Song!", Duration: 120, Path: "/private/music/song.mp3"},
		{ID: "2", ArtistID: "b", AlbumID: "y", Artist: "the band", Album: "The Album!", Title: "song", Duration: 121.5},
		{ID: "3", ArtistID: "a", AlbumID: "x", Artist: "The Band", Album: "The Album", Title: "Song (Live)", Duration: 121},
		{ID: "4", ArtistID: "a", AlbumID: "x", Artist: "The Band", Album: "The Album", Title: "Song", Duration: 180},
	}
	groups := metadataDuplicates(songs)
	if len(groups) != 3 {
		t.Fatalf("groups: %+v", groups)
	}
	for _, g := range groups {
		if g.Kind == "album" {
			for _, member := range g.Members {
				if len(member.Tracks) != member.Count {
					t.Fatalf("missing album comparison: %+v", member)
				}
				for _, track := range member.Tracks {
					if track.Path != "" {
						t.Fatal("comparison leaked a filesystem path")
					}
				}
			}
		}
		if len(g.Members) != 2 {
			t.Fatalf("members: %+v", g)
		}
	}
	if got := metadataDuplicates(append(songs[:1], songs[0])); len(got) != 0 {
		t.Fatalf("Repeated identity: %+v", got)
	}
	if got := metadataDuplicates([]Song{{ID: "a", Title: "Song", Artist: "Band"}, {ID: "b", Title: "Song", Artist: "Band"}}); len(got) != 0 {
		t.Fatal("Unknown durations must not match")
	}
}
func TestMetadataReviewPersistenceAndPermissions(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	admin := login(t, a, "admin")
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	for _, path := range []string{"/metadata/duplicates", "/metadata/lyrics?q=test", "/metadata/covers?q=test", "/metadata/songs/one", "/metadata/songs/one/cover", "/metadata/credits/separate"} {
		expect(t, member.call("GET", "/api/v2"+path, "", ""), 403)
	}
	expect(t, member.call("POST", "/api/v2/metadata/credits/separate", "{}", ""), 403)
	var artwork bytes.Buffer
	if err := png.Encode(&artwork, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "getCoverArt") {
			w.Header().Set("Content-Type", "image/png")
			r.ParseForm()
			if r.Form.Get("id") != "mf-one" {
				t.Errorf("Wrong artwork identity: %s", r.Form.Get("id"))
			}
			w.Write(artwork.Bytes())
			return
		}
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "song": Song{ID: "one", CoverArt: "mf-one", Title: "Original", Artist: "Band", Album: "Album"}}})
	}))
	defer nav.Close()
	if e = a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "password"}); e != nil {
		t.Fatal(e)
	}
	path := "/api/v2/metadata/songs/one"
	cover := admin.call("GET", path+"/cover", "", "")
	expect(t, cover, 200)
	if !bytes.Equal(cover.Body.Bytes(), artwork.Bytes()) || cover.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("current artwork was not privately proxied")
	}
	expect(t, admin.call("PUT", path, `{"title":"","artist":"Band"}`, ""), 400)
	expect(t, admin.call("PUT", path, `{"title":"Edited","artist":"Band","coverUrl":"javascript:alert(1)"}`, ""), 400)
	body := `{"title":"Edited","artist":"Band","album":"Album","lyrics":"Words","lyricsSource":"LRCLIB:123"}`
	expect(t, member.call("PUT", path, body, ""), 403)
	longEdit := MetadataEdit{Title: "Edited", Artist: "Band", Album: "Album", Lyrics: strings.Repeat("words ", 4000)}
	expect(t, admin.call("PUT", path, mustJSON(longEdit), ""), 202)
	expect(t, admin.call("DELETE", path, "", ""), 409)
	expect(t, admin.call("PUT", path, body, ""), 409)
	if _, e = a.DB.Exec("UPDATE metadata_jobs SET state='failed'"); e != nil {
		t.Fatal(e)
	}
	expect(t, admin.call("PUT", path, body, ""), 202)
	if _, e = a.DB.Exec("UPDATE metadata_jobs SET state='failed'"); e != nil {
		t.Fatal(e)
	}
	got := admin.call("GET", path, "", "")
	expect(t, got, 200)
	if !strings.Contains(got.Body.String(), `"title":"Edited"`) || !strings.Contains(got.Body.String(), `"saved":true`) {
		t.Fatal(got.Body)
	}
	expect(t, admin.call("DELETE", path, "", ""), 200)
	got = admin.call("GET", path, "", "")
	if !strings.Contains(got.Body.String(), `"title":"Original"`) {
		t.Fatal(got.Body)
	}
	noCSRF := admin
	noCSRF.csrf = "wrong"
	expect(t, noCSRF.call("PUT", path, body, ""), 403)
}

type metadataTransport func(*http.Request) (*http.Response, error)

func (f metadataTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMetadataProviders(t *testing.T) {
	a, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer a.DB.Close()
	if e = a.createAdmin("admin", "test-password-long"); e != nil {
		t.Fatal(e)
	}
	admin := login(t, a, "admin")
	id := "12345678-1234-1234-1234-123456789012"
	a.HTTP = &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.Header.Get("User-Agent"), "ZenSync/") {
			t.Error("Missing provider identification")
		}
		body := ""
		status := 200
		switch r.URL.Host {
		case "lrclib.net":
			if r.URL.Query().Get("q") != "Band & Song" {
				t.Error("Query encoding")
			}
			body = `[{"id":1,"trackName":"Song","artistName":"Band","plainLyrics":"Words","syncedLyrics":"[00:01]Words"}]`
		case "musicbrainz.org":
			body = `{"releases":[{"id":"` + id + `","title":"Album","artist-credit":[{"name":"Band"}]}]}`
		case "coverartarchive.org":
			body = `{"images":[{"front":true,"thumbnails":{"500":"https://archive.org/cover.jpg"}}]}`
		default:
			status = 500
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	lyrics := admin.call("GET", "/api/v2/metadata/lyrics?q=Band%20%26%20Song", "", "")
	expect(t, lyrics, 200)
	var rows []lyricCandidate
	if e = json.Unmarshal(lyrics.Body.Bytes(), &rows); e != nil || len(rows) != 1 || rows[0].SyncedLyrics == "" {
		t.Fatal(lyrics.Body)
	}
	covers := admin.call("GET", "/api/v2/metadata/covers?q=Band", "", "")
	expect(t, covers, 200)
	if !strings.Contains(covers.Body.String(), "https://archive.org/cover.jpg") {
		t.Fatal(covers.Body)
	}
}
func TestMetadataProviderCooldownAndMissingArt(t *testing.T) {
	a := &App{HTTP: &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader("busy"))}, nil
	})}}
	if e := a.metadataJSON(context.Background(), "https://lrclib.net/api/search?q=x", &struct{}{}); e == nil {
		t.Fatal("Expected rate limit error")
	}
	if time.Until(a.metadataNext) < 119*time.Second {
		t.Fatal("Retry-After was ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := a.metadataJSON(ctx, "https://lrclib.net", &struct{}{}); e != context.Canceled {
		t.Fatalf("Cancellation: %v", e)
	}
	a.metadataNext = time.Time{}
	a.HTTP.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("missing"))}, nil
	})
	if e := a.metadataJSON(context.Background(), "https://coverartarchive.org/release/missing", &struct{}{}); e != sql.ErrNoRows {
		t.Fatalf("404: %v", e)
	}
}

func TestSongReadsOpenSubsonicAlbumArtist(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"id":"one","artist":"Guest","displayAlbumArtist":"Various Artists","albumArtists":[{"name":"Various Artists"}]}`, "Various Artists"},
		{`{"id":"one","artist":"Guest","albumArtists":[{"name":"First"},{"name":"Second"}]}`, "First / Second"},
		{`{"id":"one","artist":"Guest","albumArtist":"Legacy","displayAlbumArtist":"Other"}`, "Legacy"},
	} {
		var song Song
		if err := json.Unmarshal([]byte(tc.raw), &song); err != nil {
			t.Fatal(err)
		}
		if song.AlbumArtist != tc.want || song.ID != "one" || song.Artist != "Guest" {
			t.Fatalf("album artist decode: %+v", song)
		}
	}
}

func TestScreenshotAlbumUsesAlbumArtist(t *testing.T) {
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
	songs := []Song{
		{ID: "one", AlbumID: "elf-upper", Album: fixture.AlbumTitle, AlbumArtist: fixture.AlbumArtists[0], Artist: fixture.CombinedArtists[0], ArtistID: "combined", Title: "Synthetic first track"},
		{ID: "two", AlbumID: "elf-title", Album: fixture.AlbumTitle, AlbumArtist: fixture.AlbumArtists[1], Artist: fixture.StandaloneArtist, ArtistID: "slava", Title: "Synthetic second track"},
		{ID: "three", AlbumID: "unrelated", Album: fixture.AlbumTitle, AlbumArtist: "Unrelated artist", Artist: fixture.StandaloneArtist, ArtistID: "slava", Title: "Unrelated release"},
	}
	var albums []duplicateGroup
	for _, group := range metadataDuplicates(songs) {
		if group.Kind == "album" {
			albums = append(albums, group)
		}
	}
	if len(albums) != 1 || len(albums[0].Members) != 2 || albums[0].Members[0].ID != "elf-title" || albums[0].Members[1].ID != "elf-upper" {
		t.Fatalf("Expected the two screenshot editions, excluding unrelated album: %+v", albums)
	}
	if !strings.Contains(albums[0].Members[1].Label, "SLAVA MARLOW") {
		t.Fatal("Album label must use album artist")
	}
}

func TestScreenshotCombinedCredits(t *testing.T) {
	var fixture struct {
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
	songs := []Song{{ID: "standalone", ArtistID: "slava", Artist: fixture.StandaloneArtist}}
	for i, name := range fixture.CombinedArtists {
		songs = append(songs, Song{ID: fmt.Sprintf("combined-%d", i), ArtistID: fmt.Sprintf("combined-%d", i), Artist: name, Path: "/private/file.mp3"})
	}
	// A collaboration already modelled as individual identities is valid.
	songs = append(songs, Song{ID: "valid", Artist: "Slava Marlow, Aarne", Artists: []ArtistCredit{{ID: "slava", Name: "Slava Marlow"}, {ID: "aarne", Name: "Aarne"}}})
	songs = append(songs, Song{ID: "band", Artist: "Earth, Wind & Fire"})
	issues := metadataCreditIssues(songs)
	if len(issues) != len(fixture.CombinedArtists) {
		t.Fatalf("Expected all %d screenshot combinations only, got %+v", len(fixture.CombinedArtists), issues)
	}
	for _, issue := range issues {
		if len(issue.KnownArtists) == 0 || len(issue.Parts) < 2 || len(issue.Tracks) != 1 || issue.Tracks[0].Path != "" || issue.Tracks[0].ID == "valid" || issue.Tracks[0].ID == "band" {
			t.Fatalf("Invalid review evidence: %+v", issue)
		}
	}
	// No stage-name/legal-name equivalence is inferred.
	for _, group := range metadataDuplicates(songs) {
		if group.Kind == "artist" {
			t.Fatalf("Combinations must not be merged with individuals: %+v", group)
		}
	}
}

func TestCreditReviewIncludesAlbumArtistAndIsStable(t *testing.T) {
	songs := []Song{
		{ID: "known", Artist: "Slava Marlow", ArtistID: "slava"},
		{ID: "album-only", Artist: "Guest", ArtistID: "guest", AlbumArtist: "Slava Marlow, Aarne", AlbumArtists: []ArtistCredit{{ID: "collapsed", Name: "Slava Marlow, Aarne"}}},
		{ID: "valid", Artist: "Guest", ArtistID: "guest", AlbumArtist: "Slava Marlow, Aarne", AlbumArtists: []ArtistCredit{{ID: "slava", Name: "Slava Marlow"}, {ID: "aarne", Name: "Aarne"}}},
	}
	issues := metadataCreditIssues(songs)
	if len(issues) != 1 || issues[0].Role != "album artist" || issues[0].ID != "collapsed" {
		t.Fatalf("Album credit review: %+v", issues)
	}
	reversed := []Song{songs[2], songs[1], songs[0]}
	if mustJSON(issues) != mustJSON(metadataCreditIssues(reversed)) {
		t.Fatal("Review must be deterministic")
	}
}

func TestAlbumCandidatesFallbackWithoutAlbumArtist(t *testing.T) {
	groups := metadataDuplicates([]Song{
		{ID: "one", AlbumID: "one", Album: "Эльф 1", Artist: "SLAVA MARLOW"},
		{ID: "two", AlbumID: "two", Album: "ЭЛЬФ 1", Artist: "Slava Marlow"},
	})
	if len(groups) != 1 || groups[0].Kind != "album" {
		t.Fatalf("Legacy fallback: %+v", groups)
	}
}

func TestScreenshotReviewAPI(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	first := songs["one"]
	first.Artist, first.AlbumArtist, first.Album = "Slava Marlow, Aarne", "SLAVA MARLOW", "ЭЛЬФ 1"
	first.Artists = []ArtistCredit{{ID: first.ArtistID, Name: first.Artist}}
	first.AlbumArtists = []ArtistCredit{{ID: "slava", Name: first.AlbumArtist}}
	songs["one"] = first
	second := songs["two"]
	second.Artist, second.AlbumArtist, second.Album = "Slava Marlow", "Slava Marlow", "ЭЛЬФ 1"
	second.Artists = []ArtistCredit{{ID: "slava", Name: second.Artist}}
	second.AlbumArtists = []ArtistCredit{{ID: "slava", Name: second.AlbumArtist}}
	songs["two"] = second
	response := admin.call("GET", "/api/v2/metadata/duplicates", "", "")
	expect(t, response, 200)
	var report struct {
		Groups       []duplicateGroup
		CreditIssues []creditIssue
	}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 1 || report.Groups[0].Kind != "album" || len(report.Groups[0].Members) != 2 {
		t.Fatalf("Album report: %+v", report.Groups)
	}
	if len(report.CreditIssues) != 1 || report.CreditIssues[0].Name != first.Artist {
		t.Fatalf("Credit report: %+v", report.CreditIssues)
	}
	if report.CreditIssues[0].Tracks[0].Path != "" || report.Groups[0].Members[0].Tracks[0].Path != "" {
		t.Fatal("Report exposed filesystem paths")
	}
	var jobs int
	if err := a.DB.QueryRow("SELECT count(*) FROM metadata_jobs").Scan(&jobs); err != nil || jobs != 0 {
		t.Fatal("Review must not queue metadata writes", err)
	}
}

func TestCreditReviewAmpersandSeparator(t *testing.T) {
	for _, role := range []string{"artist", "album artist"} {
		t.Run(role, func(t *testing.T) {
			combined := Song{ID: "combined", ArtistID: "combined", Artist: "163ONMYNECK & Polyana"}
			if role == "album artist" {
				combined.Artist = "Guest"
				combined.AlbumArtists = []ArtistCredit{{ID: "combined", Name: "163ONMYNECK & Polyana"}}
			}
			// Punctuation alone must not be enough to suggest a split.
			if issues := metadataCreditIssues([]Song{combined}); len(issues) != 0 {
				t.Fatalf("Unexpected suggestion without known artists: %+v", issues)
			}
			songs := []Song{
				{ID: "known", ArtistID: "solo", Artist: "163ONMYNECK"},
				combined,
				{ID: "band", Artist: "Earth, Wind & Fire"},
				{ID: "unspaced", Artist: "163ONMYNECK&Polyana"},
				{ID: "separate", Artists: []ArtistCredit{{ID: "solo", Name: "163ONMYNECK"}, {ID: "polyana", Name: "Polyana"}}},
			}
			issues := metadataCreditIssues(songs)
			if len(issues) != 1 || issues[0].Role != role || mustJSON(issues[0].Parts) != mustJSON([]string{"163ONMYNECK", "Polyana"}) {
				t.Fatalf("Ampersand credit review: %+v", issues)
			}
		})
	}
}
