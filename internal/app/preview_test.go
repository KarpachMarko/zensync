package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestReviewPreservesDeezerPreviewAndLink(t *testing.T) {
	a, _, member := albumFixture(t)
	const preview = "https://cdnt-preview.dzcdn.net/preview.mp3"
	const link = "https://www.deezer.com/track/42"
	// Raw provider JSON catches fields accidentally dropped during decoding.
	record := `{"id":42,"title":"Preview recording","duration":180,"artist":{"name":"Artist"},"album":{"title":"Album"},"preview":"` + preview + `","link":"` + link + `"}`
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/search/track" {
			w.Write([]byte(`{"data":[` + record + `]}`))
			return
		}
		if r.URL.Path == "/track/42" {
			w.Write([]byte(record))
			return
		}
		http.NotFound(w, r)
	}))
	defer provider.Close()
	a.deezerURL = provider.URL
	data, err := os.ReadFile("../apple/testdata/playlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	w := member.call("POST", "/api/v2/imports", string(data), "")
	expect(t, w, 201)
	var upload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	mid := upload["id"]
	sources, err := a.sources(mid)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "/api/v2/imports/" + mid
	songPrefix := prefix + "/songs/" + sources[0].ID
	for _, query := range []string{`{"query":"Artist Song"}`, `{"query":"https://www.deezer.com/track/42"}`} {
		w = member.call("POST", songPrefix+"/candidates", query, "")
		expect(t, w, 200)
		var candidates struct {
			Deezer []Track `json:"deezer"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &candidates); err != nil {
			t.Fatal(err)
		}
		if len(candidates.Deezer) != 1 || candidates.Deezer[0].Preview != preview || candidates.Deezer[0].Link != link {
			t.Fatalf("lost preview metadata: %s", w.Body.String())
		}
	}
	expect(t, member.call("POST", prefix+"/commit", `{"playlists":[],"favorites":false}`, ""), 200)
	expect(t, member.call("POST", songPrefix+"/resolve", `{"target":"deezer:track:42"}`, ""), 200)
	saved, err := a.sources(mid)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].Candidate == nil || saved[0].Candidate.Preview != preview || saved[0].Candidate.Link != link {
		t.Fatal("selected recording lost preview metadata")
	}
	var requests int
	if err := a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&requests); err != nil || requests != 0 {
		t.Fatalf("review created requests: %d, %v", requests, err)
	}
}
