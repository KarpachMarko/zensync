package app

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"zensync/internal/apple"
)

type importRecordingExample struct {
	Name      string     `json:"name"`
	Source    apple.Song `json:"source"`
	Selected  Track      `json:"selected"`
	Confident bool       `json:"confident"`
	Searches  []struct {
		Query  string  `json:"query"`
		Tracks []Track `json:"tracks"`
	} `json:"searches"`
}

func importRecordingExamples(t *testing.T) []importRecordingExample {
	t.Helper()
	data, err := os.ReadFile("testdata/import_recordings.json")
	if err != nil {
		t.Fatal(err)
	}
	var examples []importRecordingExample
	if err = json.Unmarshal(data, &examples); err != nil {
		t.Fatal(err)
	}
	return examples
}

func TestImportRecordingExamples(t *testing.T) {
	for _, example := range importRecordingExamples(t) {
		t.Run(example.Name, func(t *testing.T) {
			match := assessRecording(example.Source, example.Selected)
			if match.Confident != example.Confident {
				t.Fatalf("confidence = %v, want %v; reasons: %v", match.Confident, example.Confident, match.Reasons)
			}
			if !match.Confident && len(match.Reasons) == 0 {
				t.Fatal("review must explain why metadata needs checking")
			}
		})
	}
}

type recordingSearchCalls struct {
	queries atomic.Int32
	details atomic.Int32
}

func recordingExampleServer(t *testing.T, a *App, examples []importRecordingExample) *recordingSearchCalls {
	t.Helper()
	byQuery := map[string][]Track{}
	byID := map[int64]Track{}
	for _, example := range examples {
		byID[example.Selected.ID] = example.Selected
		for _, search := range example.Searches {
			byQuery[search.Query] = search.Tracks
		}
	}
	calls := &recordingSearchCalls{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/track" {
			calls.queries.Add(1)
			tracks := byQuery[r.URL.Query().Get("q")]
			if tracks == nil {
				tracks = []Track{}
			}
			send(w, 200, map[string]any{"data": tracks})
			return
		}
		calls.details.Add(1)
		for id, track := range byID {
			if r.URL.Path == "/track/"+strconv.FormatInt(id, 10) {
				send(w, 200, track)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(provider.Close)
	a.deezerURL = provider.URL
	return calls
}

func TestImportSearchFindsExamplesAndRanksActualArtistFirst(t *testing.T) {
	examples := importRecordingExamples(t)
	a, _, _ := albumFixture(t)
	calls := recordingExampleServer(t, a, examples)
	for i, example := range examples {
		t.Run(example.Name, func(t *testing.T) {
			calls.queries.Store(0)
			calls.details.Store(0)
			tracks, err := a.searchImportRecordings(context.Background(), example.Source, example.Name)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, track := range tracks {
				found = found || track.ID == example.Selected.ID
			}
			if !found {
				t.Fatalf("selected recording not retrieved after %d searches", calls.queries.Load())
			}
			if calls.queries.Load() > 5 || calls.details.Load() > 2 {
				t.Fatalf("unbounded search: %d queries, %d detail reads", calls.queries.Load(), calls.details.Load())
			}
			candidate := confidentRecording(example.Source, tracks)
			expected := example.Selected.ID
			if i == 1 {
				expected = 484090122
			} // Pigott is not the Lil Pump recording.
			if example.Confident || i == 1 {
				if candidate == nil || candidate.ID != expected {
					t.Fatalf("expected recording %d, got %+v", expected, candidate)
				}
				if tracks[0].ID != expected {
					t.Fatalf("best metadata match was not ranked first: %d", tracks[0].ID)
				}
			} else if candidate != nil {
				t.Fatalf("uncertain recording selected: %+v", candidate)
			}
		})
	}
}

func TestMigrationScanUsesImprovedMatchingAndReviewExplainsDifferences(t *testing.T) {
	a, _, member := albumFixture(t)
	examples := importRecordingExamples(t)
	recordingExampleServer(t, a, examples)
	nav := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"subsonic-response": map[string]any{"status": "ok", "searchResult3": map[string]any{"song": []Song{}}}})
	}))
	defer nav.Close()
	if err := a.seal("navidrome", Credentials{URL: nav.URL, Username: "service", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	indices := []int{1, 2, 11, 13, 17}
	var xml strings.Builder
	xml.WriteString(`<plist><dict><key>Tracks</key><dict>`)
	for _, index := range indices {
		source := examples[index].Source
		fmt.Fprintf(&xml, `<key>%d</key><dict><key>Name</key><string>%s</string><key>Artist</key><string>%s</string><key>Album</key><string>%s</string><key>Total Time</key><integer>%d</integer></dict>`, index, html.EscapeString(source.Title), html.EscapeString(source.Artist), html.EscapeString(source.Album), int(source.Duration*1000))
	}
	xml.WriteString(`</dict></dict></plist>`)
	response := member.call("POST", "/api/v2/imports", xml.String(), "")
	expect(t, response, 201)
	var uploaded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	prefix := "/api/v2/imports/" + uploaded.ID
	expect(t, member.call("POST", prefix+"/commit", `{"playlists":[]}`, ""), 200)
	a.runMigrationTask(context.Background())
	sources, err := a.sources(uploaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.Metadata.Title == examples[17].Source.Title {
			if source.Candidate != nil || source.MatchState != "ambiguous" {
				t.Fatalf("different-length recording matched: %+v", source)
			}
			review := member.call("POST", prefix+"/songs/"+source.ID+"/candidates", mustJSON(map[string]string{"query": examples[17].Name}), "")
			expect(t, review, 200)
			var result struct {
				Matches map[int64]RecordingMatch `json:"deezerMatches"`
			}
			if err := json.Unmarshal(review.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			match := result.Matches[examples[17].Selected.ID]
			if match.Confident || !strings.Contains(strings.Join(match.Reasons, " "), "Duration differs") {
				t.Fatalf("review did not explain uncertainty: %+v", match)
			}
		} else if source.Candidate == nil || source.MatchState != "confident" {
			t.Fatalf("metadata match was not selected: %+v", source)
		}
	}
	var requests int
	if err := a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&requests); err != nil || requests != 0 {
		t.Fatalf("matching created acquisition requests: %d, %v", requests, err)
	}
}

func TestRecordingMatchRejectsDifferentVersionsAndMissingEvidence(t *testing.T) {
	source := apple.Song{Title: "Song", Artist: "Artist", Album: "Album", Duration: 180}
	base := makeTrack(42, "Song")
	base.Artist.Name, base.Album.Title, base.Duration = "Artist", "Album", 180
	for _, name := range []string{"Remix", "Live", "Remastered 2025", "Acoustic", "Instrumental", "Radio Edit", "Sped Up", "Slowed", "Clean"} {
		t.Run(name, func(t *testing.T) {
			track := base
			track.TitleVersion = "(" + name + ")"
			if assessRecording(source, track).Confident {
				t.Fatal("alternate version matched")
			}
		})
	}
	for _, test := range []struct {
		name   string
		source apple.Song
		track  Track
	}{
		{"different artist", source, func() Track { track := base; track.Artist.Name = "Other artist"; return track }()},
		{"additional main artist", source, func() Track { track := base; track.Artist.Name = "Other artist & Artist"; return track }()},
		{"different album", source, func() Track { track := base; track.Album.Title = "Compilation"; return track }()},
		{"different duration", source, func() Track { track := base; track.Duration = 210; return track }()},
		{"missing album", func() apple.Song { s := source; s.Album = ""; return s }(), base},
		{"missing duration", func() apple.Song { s := source; s.Duration = 0; return s }(), base},
		{"short clip", func() apple.Song { s := source; s.Duration = 2; return s }(), func() Track { track := base; track.Duration = 5; return track }()},
		{"different feature", func() apple.Song { s := source; s.Title = "Song (feat. Guest)"; return s }(), func() Track { track := base; track.Title = "Song (feat. Someone else)"; return track }()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if assessRecording(test.source, test.track).Confident {
				t.Fatal("uncertain recording matched")
			}
		})
	}
	for _, title := range []string{"$$$", "?", "infinity (888)"} {
		s := source
		s.Title = title
		track := base
		track.Title = title
		if !assessRecording(s, track).Confident {
			t.Fatalf("meaningful punctuation/numbers lost from %q", title)
		}
	}
	if maskedTitleEqual("side word", "side ****") {
		t.Fatal("fully masked word treated as proof")
	}
}

func TestRecordingMatchGroupsOnlyConfirmedDuplicateRecordings(t *testing.T) {
	source := apple.Song{Title: "Song", Artist: "Artist", Album: "Album", Duration: 180}
	first := makeTrack(42, "Song")
	first.Artist.Name = "Artist"
	first.Album.Title = "Album"
	first.Duration = 180
	second := first
	second.ID = 43
	if confidentRecording(source, []Track{first, second}) != nil {
		t.Fatal("different unknown recordings auto-selected")
	}
	first.ISRC = "US1234567890"
	second.ISRC = first.ISRC
	if confidentRecording(source, []Track{first, second}) == nil {
		t.Fatal("duplicate release IDs forced unnecessary review")
	}
	second.ISRC = "US1234567891"
	if confidentRecording(source, []Track{first, second}) != nil {
		t.Fatal("different ISRCs auto-selected")
	}
	second.ISRC = first.ISRC
	explicit, clean := true, false
	first.ExplicitLyrics, second.ExplicitLyrics = &explicit, &clean
	if confidentRecording(source, []Track{first, second}) != nil {
		t.Fatal("different explicit/clean versions auto-selected")
	}
	unknown := first
	unknown.ID, unknown.ExplicitLyrics = 44, nil
	if confidentRecording(source, []Track{unknown, first, second}) != nil {
		t.Fatal("missing explicit flag hid a conflict between known versions")
	}
}

func TestRecordingRankingPrefersMatchingArtistAndTitleDespiteIncompleteMetadata(t *testing.T) {
	source := apple.Song{Title: "Song", Artist: "Artist", Album: "Album", Duration: 180}
	cover := makeTrack(1, "Song")
	cover.Artist.Name, cover.Album.Title, cover.Duration = "Cover artist", "Album", 180
	likely := makeTrack(2, "Song")
	likely.Artist.Name, likely.Album.Title, likely.Duration = "Artist", "", 0
	tracks := []Track{cover, likely}
	rankRecordings(source, tracks)
	if tracks[0].ID != likely.ID {
		t.Fatal("other artist outranked the matching artist/title")
	}
	if confidentRecording(source, tracks) != nil {
		t.Fatal("ranking turned missing metadata into confidence")
	}
}

func TestImportSearchPreservesCustomQueryAndHasBoundedEmptyFallbacks(t *testing.T) {
	a, _, _ := albumFixture(t)
	queries := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("q"))
		send(w, 200, map[string]any{"data": []Track{}})
	}))
	defer provider.Close()
	a.deezerURL = provider.URL
	source := apple.Song{Title: "Song-title (feat. Guest)", Artist: "Primary & Collaborator", Album: "Album", Duration: 180}
	if tracks, err := a.searchImportRecordings(context.Background(), source, "my edited query"); err != nil || len(tracks) != 0 {
		t.Fatalf("custom query failed: %v", err)
	}
	if len(queries) != 1 || queries[0] != "my edited query" {
		t.Fatalf("edited query was rewritten: %v", queries)
	}
	queries = nil
	if _, err := a.searchImportRecordings(context.Background(), source, source.Artist+" "+source.Title); err != nil {
		t.Fatal(err)
	}
	if len(queries) < 2 || len(queries) > 5 {
		t.Fatalf("fallback count: %v", queries)
	}
}
