package app

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleArtistDismissalSurvivesRefreshAndBlocksQueuedRepairs(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	known := songs["one"]
	known.Artists = []ArtistCredit{{ID: "v-dollar", Name: "V $"}, {ID: "prince", Name: "V PRiNCE"}}
	songs["one"] = known
	song := songs["two"]
	song.Artist = "V $ X V PRiNCE"
	song.Artists = []ArtistCredit{{ID: "literal", Name: song.Artist}}
	song.AlbumArtist = song.Artist
	song.AlbumArtists = []ArtistCredit{{ID: "literal", Name: song.Artist}}
	songs["two"] = song
	path := filepath.Join(a.musicDir, song.Path)
	writeNativeCredits(t, path, nativeCredits{[]string{song.Artist}, []string{song.Artist}, []string{"literal"}, []string{"literal"}})
	before, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	report := func() int {
		t.Helper()
		r := admin.call("GET", "/api/v2/metadata/duplicates?refresh=true", "", "")
		expect(t, r, 200)
		var out struct{ CreditIssues []creditIssue }
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return len(out.CreditIssues)
	}
	if report() != 2 {
		t.Fatal("Screenshot literal-name fixture not detected in both roles")
	}
	// Also protect a batch that was queued before the user dismissed this name.
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", "", ""), 202)
	body := mustJSON(map[string]any{"kind": "credit", "name": song.Artist})
	expect(t, admin.call("POST", "/api/v2/metadata/dismissals", body, ""), 200)
	if report() != 0 {
		t.Fatal("Dismissed artist returned on refresh")
	}
	a.runMetadata(context.Background())
	after, err := hashFile(path)
	if err != nil || after != before {
		t.Fatal("Dismissed literal artist was split", err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", `{"includeUnknown":true}`, ""), 409)
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate", mustJSON(map[string]any{"source": "literal", "role": "artist", "name": song.Artist, "artists": []string{"V $", "V PRiNCE"}}), ""), 409)
	// Database state, rather than an in-memory report cache, owns the decision.
	reopened, err := Open(filepath.Join(a.dataDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	c, err := a.shared()
	if err != nil {
		t.Fatal(err)
	}
	ignored, err := reopened.metadataDismissals(c)
	if err != nil || !ignored[metadataCreditDismissalKey(song.Artist)] {
		t.Fatalf("Dismissal not persisted: %v %v", ignored, err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/dismissals", mustJSON(map[string]any{"kind": "credit", "name": song.Artist, "restore": true}), ""), 200)
	if report() != 2 {
		t.Fatal("Undo did not restore suggestions")
	}
}

func TestDuplicateDismissalsAreScopedAndValidatePermissions(t *testing.T) {
	a, admin, _ := metadataLibraryFixture(t)
	for _, kind := range []string{"song", "album", "artist"} {
		ids := []string{"two", "one"}
		if kind != "song" {
			ids = []string{kind + "-two", kind + "-one"}
		}
		expect(t, admin.call("POST", "/api/v2/metadata/dismissals", mustJSON(map[string]any{"kind": kind, "ids": ids}), ""), 200)
		response := admin.call("GET", "/api/v2/metadata/duplicates", "", "")
		expect(t, response, 200)
		var report struct{ Groups []duplicateGroup }
		if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		for _, group := range report.Groups {
			if group.Kind == kind {
				t.Fatal("Dismissed group returned", kind)
			}
		}
		c, _ := a.shared()
		rules, err := a.metadataDismissals(c)
		if err != nil || !rules[metadataGroupDismissalKey(kind, []string{ids[1], ids[0]})] {
			t.Fatal(kind, rules, err)
		}
		c.URL = "https://another-library.example"
		rules, err = a.metadataDismissals(c)
		if err != nil || len(rules) != 0 {
			t.Fatal("Dismissal leaked across servers", rules, err)
		}
	}
	expect(t, admin.call("POST", "/api/v2/metadata/dismissals", `{"kind":"album","ids":["one","one"]}`, ""), 400)
	expect(t, admin.call("POST", "/api/v2/metadata/dismissals", `{"kind":"credit","name":""}`, ""), 400)
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	expect(t, member.call("POST", "/api/v2/metadata/dismissals", `{"kind":"credit","name":"V $ X V PRiNCE"}`, ""), 403)
	// This fixture's album IDs are album-one/album-two; dismiss that exact group.
	expect(t, admin.call("POST", "/api/v2/metadata/dismissals", `{"kind":"album","ids":["album-two","album-one"]}`, ""), 200)
	r := admin.call("GET", "/api/v2/metadata/duplicates", "", "")
	expect(t, r, 200)
	var out struct{ Groups []duplicateGroup }
	json.Unmarshal(r.Body.Bytes(), &out)
	for _, group := range out.Groups {
		if group.Kind == "album" {
			t.Fatal("Dismissed album group remained")
		}
	}
	// A changed membership represents new evidence, and should be reviewed again.
	if metadataGroupDismissalKey("album", []string{"one", "two"}) == metadataGroupDismissalKey("album", []string{"one", "two", "three"}) {
		t.Fatal("New member hidden")
	}
	if strings.Contains(r.Body.String(), "metadata_dismissals") {
		t.Fatal("Internal rule data exposed")
	}
}

func TestDismissalDuringCreditPreparationPreventsPublication(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	known := songs["one"]
	known.Artists = []ArtistCredit{{ID: "first", Name: "V $"}, {ID: "second", Name: "V PRiNCE"}}
	songs["one"] = known
	song := songs["two"]
	song.Artist = "V $ X V PRiNCE"
	song.Artists = []ArtistCredit{{ID: "literal", Name: song.Artist}}
	songs["two"] = song
	path := filepath.Join(a.musicDir, song.Path)
	writeNativeCredits(t, path, nativeCredits{[]string{song.Artist}, []string{"Band"}, []string{"literal"}, []string{"band"}})
	before, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", "/api/v2/metadata/credits/separate-known", "", ""), 202)
	original := a.HTTP.Transport
	if original == nil {
		original = http.DefaultTransport
	}
	copied := *a.HTTP
	dismissed := false
	copied.Transport = metadataTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "getSong.view") && !dismissed {
			dismissed = true
			expect(t, admin.call("POST", "/api/v2/metadata/dismissals", mustJSON(map[string]string{"kind": "credit", "name": song.Artist}), ""), 200)
		}
		return original.RoundTrip(r)
	})
	a.HTTP = &copied
	a.runMetadata(context.Background())
	after, err := hashFile(path)
	if err != nil || !dismissed || after != before {
		t.Fatal("In-flight split published after dismissal", err)
	}
	assertNoMetadataCopies(t, a)
}
