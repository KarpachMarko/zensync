package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"zensync/internal/apple"
)

type losslessRoutingTransport struct{ target *url.URL }

func (tr losslessRoutingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "lossless.am" {
		return (losslessTransport{tr.target}).RoundTrip(r)
	}
	return http.DefaultTransport.RoundTrip(r)
}
func TestLosslessMigrationReviewResolveAndAcquire(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		t.Run(map[bool]string{false: "individual", true: "bulk"}[bulk], func(t *testing.T) {
			a, _, member := albumFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/search" && r.URL.Path != "/api/track/42" {
					t.Errorf("unexpected provider path %s", r.URL.Path)
				}
				io.WriteString(w, losslessSearchFixture)
			}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			a.HTTP = &http.Client{Transport: losslessRoutingTransport{u}}
			if err := a.seal("lossless", "jwt_access=fixture; jwt_refresh=fixture"); err != nil {
				t.Fatal(err)
			}
			mid, sid := stageAudioMigration(t, member)
			expect(t, member.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[]}`, ""), 200)
			endpoint := "/api/v2/imports/" + mid + "/songs/" + sid
			result := member.call("POST", endpoint+"/candidates", `{"query":"Song"}`, "")
			expect(t, result, 200)
			var response struct {
				Providers []struct {
					ID     string  `json:"id"`
					Tracks []Track `json:"tracks"`
				} `json:"providers"`
			}
			if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Providers) != 2 || response.Providers[1].ID != "lossless" || len(response.Providers[1].Tracks) != 1 {
				t.Fatalf("missing provider results: %s", result.Body)
			}
			// Same numeric ID in Deezer must not replace the reviewed lossless recording.
			a.rememberReviewTracks([]Track{makeTrack(42, "Other recording")})
			expect(t, member.call("POST", endpoint+"/resolve", `{"target":"lossless:track:main:42"}`, ""), 200)
			var metadata string
			a.DB.QueryRow("SELECT candidate FROM source_songs WHERE id=?", sid).Scan(&metadata)
			var chosen Track
			json.Unmarshal([]byte(metadata), &chosen)
			if chosen.Provider != "lossless" {
				t.Fatal("provider identity lost")
			}
			if bulk {
				expect(t, member.call("POST", "/api/v2/imports/"+mid+"/acquire", mustJSON(map[string]any{"sourceIds": []string{sid}}), "lossless-batch"), 201)
			} else {
				expect(t, member.call("POST", endpoint+"/acquire", `{}`, ""), 201)
			}
			var target, decision string
			if err := a.DB.QueryRow("SELECT target,decision FROM requests WHERE id IN (SELECT request_id FROM request_sources WHERE source_id=?)", sid).Scan(&target, &decision); err != nil {
				t.Fatal(err)
			}
			if target != "lossless:track:main:42" || decision != "awaiting_approval" {
				t.Fatalf("wrong request %s %s", target, decision)
			}
			direct := member.call("POST", endpoint+"/candidates", `{"query":"https://lossless.am/app/track/main/42?l=ru"}`, "")
			expect(t, direct, 200)
			if !strings.Contains(direct.Body.String(), `"provider":"lossless"`) {
				t.Fatal(direct.Body.String())
			}
		})
	}
}
func TestLosslessImportMatchingSurvivesDeezerFailure(t *testing.T) {
	a, _, _ := albumFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, losslessSearchFixture) }))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	a.HTTP = &http.Client{Transport: losslessRoutingTransport{u}}
	a.seal("lossless", "jwt_refresh=fixture")
	source := apple.Song{Title: "Song", Artist: "Artist", Album: "Album", Duration: 2}
	tracks, err := a.searchImportRecordings(context.Background(), source, "Artist Song")
	if err != nil {
		t.Fatal(err)
	}
	selected := confidentRecording(source, tracks)
	if selected == nil || selected.Provider != "lossless" {
		t.Fatalf("lossless match missing: %+v", tracks)
	}
}
func TestLosslessURLsAreStrict(t *testing.T) {
	good := "https://lossless.am/app/track/main/104040942?l=ru"
	if target, err := losslessTarget(good); err != nil || target != "lossless:track:main:104040942" {
		t.Fatalf("%s %v", target, err)
	}
	for _, bad := range []string{"https://evil.test/app/track/main/42", "https://lossless.am@evil.test/app/track/main/42", "https://lossless.am/app/album/main/42", "https://lossless.am/app/track/../../42"} {
		if _, err := losslessTarget(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestFulfillmentKeepsProviderAndSourceIdentitiesSeparate(t *testing.T) {
	a, _, member := albumFixture(t)
	mid, _ := stageAudioMigration(t, member)
	base := makeTrack(42, "Song")
	for _, test := range []struct{ key, provider, source string }{{"deezer", "", ""}, {"main", "lossless", "main"}, {"other", "lossless", "other"}} {
		track := base
		track.Provider = test.provider
		track.Source = test.source
		if _, err := a.DB.Exec("INSERT INTO source_songs(id,migration_id,source_id,metadata,candidate,match_state) VALUES(?,?,?,?,?,'manual')", test.key, mid, test.key, mustJSON(apple.Song{Title: "Song"}), mustJSON(track)); err != nil {
			t.Fatal(err)
		}
	}
	a.fulfill("unused-job", "unused-request", "lossless:track:main:42", "nav-main")
	for _, key := range []string{"deezer", "main", "other"} {
		var song string
		a.DB.QueryRow("SELECT server_id FROM source_songs WHERE id=?", key).Scan(&song)
		if (key == "main" && song != "nav-main") || (key != "main" && song != "") {
			t.Fatalf("wrong fulfillment for %s: %s", key, song)
		}
	}
	a.fulfill("unused-job", "unused-request", "deezer:track:42", "nav-deezer")
	var song string
	a.DB.QueryRow("SELECT server_id FROM source_songs WHERE id='other'").Scan(&song)
	if song != "" {
		t.Fatal("Deezer fulfilled a different provider")
	}
}

func TestImportResultsRankAcrossProviderBoundaries(t *testing.T) {
	a, _, member := albumFixture(t)
	lossless := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, losslessSearchFixture) }))
	defer lossless.Close()
	u, _ := url.Parse(lossless.URL)
	a.HTTP = &http.Client{Transport: losslessRoutingTransport{u}}
	a.seal("lossless", "jwt_access=fixture; jwt_refresh=fixture")
	deezer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"data": []Track{makeTrack(42, "Different recording")}})
	}))
	defer deezer.Close()
	a.deezerURL = deezer.URL
	mid, sid := stageAudioMigration(t, member)
	a.DB.Exec("UPDATE source_songs SET metadata=? WHERE id=?", mustJSON(apple.Song{Title: "Song", Artist: "Artist", Album: "Album", Duration: 2}), sid)
	w := member.call("POST", "/api/v2/imports/"+mid+"/songs/"+sid+"/candidates", `{"query":"Song"}`, "")
	expect(t, w, 200)
	var response struct {
		Recordings []Track                   `json:"recordings"`
		Matches    map[string]RecordingMatch `json:"recordingMatches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Recordings) != 2 || response.Recordings[0].Provider != "lossless" {
		t.Fatalf("provider grouping defeated match ordering: %s", w.Body)
	}
	if !response.Matches["lossless:track:main:42"].Confident || response.Matches["deezer:track:42"].Confident {
		t.Fatal("numeric ID collision corrupted comparisons")
	}
}
