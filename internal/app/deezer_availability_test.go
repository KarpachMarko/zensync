package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"zensync/internal/apple"
)

func TestSharedRecordingWithPublicReadabilityFalseCanBeMatchedSelectedAndRequested(t *testing.T) {
	a, _, member := albumFixture(t)
	mid, sid := stageAudioMigration(t, member)
	source := apple.Song{Title: "Make It Rain", Artist: "Ed Sheeran", Album: "x (Wembley Edition)", Duration: 404}
	if _, err := a.DB.Exec("UPDATE source_songs SET metadata=? WHERE id=?", mustJSON(source), sid); err != nil {
		t.Fatal(err)
	}
	prefix := "/api/v2/imports/" + mid
	expect(t, member.call("POST", prefix+"/commit", `{"playlists":[]}`, ""), 200)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/track/111645632" {
			t.Errorf("unexpected provider request: %s", r.URL.Path)
		}
		send(w, 200, json.RawMessage(`{"id":111645632,"title":"Make It Rain","duration":404,"artist":{"name":"Ed Sheeran"},"album":{"title":"x (Wembley Edition)"},"readable":false,"available_countries":["FR"]}`))
	}))
	defer provider.Close()
	a.deezerURL = provider.URL
	transport := a.HTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	a.HTTP.Transport = redirectTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "link.deezer.com" {
			return &http.Response{StatusCode: 301, Header: http.Header{"Location": {"https://link.deezer.com/?dest=https%3A%2F%2Fwww.deezer.com%2Ftrack%2F111645632"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return transport.RoundTrip(r)
	})
	endpoint := prefix + "/songs/" + sid
	response := member.call("POST", endpoint+"/candidates", `{"query":"https://link.deezer.com/s/34RlcQCOcl8r6jgNmOWub"}`, "")
	expect(t, response, 200)
	var result struct {
		Deezer  []Track                  `json:"deezer"`
		Matches map[int64]RecordingMatch `json:"deezerMatches"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	match := result.Matches[111645632]
	if len(result.Deezer) != 1 || !match.Confident || len(match.Reasons) != 0 || len(match.Warnings) != 1 {
		t.Fatalf("public readability was confused with recording identity: %s", response.Body.String())
	}
	if !strings.Contains(match.Warnings[0], "account or region") || strings.Contains(response.Body.String(), "Unavailable from Deezer") {
		t.Fatal("misleading availability message")
	}
	wrongArtist := result.Deezer[0]
	wrongArtist.Artist.Name = "Another artist"
	if assessRecording(source, wrongArtist).Confident {
		t.Fatal("availability change weakened artist matching")
	}
	var requests int
	if err := a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&requests); err != nil || requests != 0 {
		t.Fatal("review created a download request")
	}
	expect(t, member.call("POST", endpoint+"/resolve", `{"target":"deezer:track:111645632"}`, ""), 200)
	expect(t, member.call("POST", endpoint+"/acquire", `{}`, ""), 201)
	var decision string
	if err := a.DB.QueryRow("SELECT decision FROM requests WHERE target='deezer:track:111645632'").Scan(&decision); err != nil || decision != "awaiting_approval" {
		t.Fatalf("request approval changed: %s %v", decision, err)
	}
}
