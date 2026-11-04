package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func failedSourceRequest(t *testing.T, a *App, sid string) string {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) SELECT 'old',id,'old','deezer:track:111599580','Make It Rain','Ed Sheeran',0,'["lossless"]','approved' FROM users WHERE username='member'`,
		`INSERT INTO jobs(id,request_id,state,stage) VALUES('old-job','old','failed','Deezer denied this recording for the configured account country')`,
	} {
		if _, e := a.DB.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	if sid != "" {
		if _, e := a.DB.Exec("INSERT INTO request_sources(source_id,request_id) VALUES(?,'old')", sid); e != nil {
			t.Fatal(e)
		}
	}
	return "old-job"
}
func TestUploadedReplacementSupersedesFailureAndPreventsProviderRetry(t *testing.T) {
	a, admin, member := albumFixture(t)
	a.musicDir = t.TempDir()
	mid, sid := stageAudioMigration(t, member)
	expect(t, member.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[]}`, ""), 200)
	jid := failedSourceRequest(t, a, sid)
	audio := audioFixture(t, t.TempDir(), ".flac", "flac", true)
	w := uploadAudioForTest(member, "/api/v2/imports/"+mid+"/songs/"+sid+"/upload", "own.flac", "replacement-upload", audio)
	expect(t, w, 201)
	var replacement struct {
		ID string `json:"id"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &replacement); e != nil {
		t.Fatal(e)
	}
	var state, stage string
	if e := a.DB.QueryRow("SELECT state,stage FROM jobs WHERE id=?", jid).Scan(&state, &stage); e != nil {
		t.Fatal(e)
	}
	if state != "superseded" || !strings.Contains(stage, "account country") {
		t.Fatalf("original failure not retired/preserved: %s %s", state, stage)
	}
	expect(t, member.call("POST", "/api/v2/tasks/"+jid, "", ""), 409)
	// Index the uploaded file using a different display title and virtual path.
	nav := &navFixture{songs: map[string]Song{}, music: a.musicDir}
	server := httptest.NewServer(http.HandlerFunc(nav.handler))
	defer server.Close()
	if e := a.seal("navidrome", Credentials{URL: server.URL, Username: "service", Password: "fixture"}); e != nil {
		t.Fatal(e)
	}
	a.deezerURL = "http://127.0.0.1:1"
	expect(t, admin.call("POST", "/api/v2/requests/"+replacement.ID, `{"decision":"approved"}`, ""), 200)
	a.runAcquisition(context.Background())
	var serverID string
	if e := a.DB.QueryRow("SELECT server_id FROM source_songs WHERE id=?", sid).Scan(&serverID); e != nil || serverID != "B" {
		t.Fatalf("upload not indexed: %s %v", serverID, e)
	}
	activity := member.call("GET", "/api/v2/activity", "", "")
	expect(t, activity, 200)
	var tasks []struct {
		ID, State, Stage      string
		CanReview, CanDismiss bool
	}
	if e := json.Unmarshal(activity.Body.Bytes(), &tasks); e != nil {
		t.Fatal(e)
	}
	for _, task := range tasks {
		if task.ID == jid && (task.State != "superseded" || task.CanReview || task.CanDismiss || !strings.Contains(task.Stage, "account country")) {
			t.Fatalf("old failure still actionable: %+v", task)
		}
	}
	// Simulate a queued claim left by an older build. It must not download or
	// overwrite the uploaded source, nor mark the Deezer target itself available.
	if _, e := a.DB.Exec("UPDATE jobs SET state='queued' WHERE id=?", jid); e != nil {
		t.Fatal(e)
	}
	a.runAcquisition(context.Background())
	if e := a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jid).Scan(&state); e != nil || state != "superseded" {
		t.Fatalf("stale retry was executed: %s %v", state, e)
	}
	var count int
	if e := a.DB.QueryRow("SELECT count(*) FROM assets WHERE target='deezer:track:111599580'").Scan(&count); e != nil || count != 0 {
		t.Fatal("upload misrepresented as the original Deezer recording")
	}
	if e := a.DB.QueryRow("SELECT server_id FROM source_songs WHERE id=?", sid).Scan(&serverID); e != nil || serverID != "B" {
		t.Fatal("retry changed replacement binding")
	}
	var requests []Request
	w = member.call("GET", "/api/v2/requests", "", "")
	expect(t, w, 200)
	if e := json.Unmarshal(w.Body.Bytes(), &requests); e != nil {
		t.Fatal(e)
	}
	for _, request := range requests {
		if request.ID == "old" && request.State != "superseded" {
			t.Fatalf("request still blocked: %+v", request)
		}
	}
}
func TestReplacingOneSharedSourceDoesNotRetireRemainingIntent(t *testing.T) {
	a, _, member := albumFixture(t)
	_, sid := stageAudioMigration(t, member)
	jid := failedSourceRequest(t, a, sid)
	if _, e := a.DB.Exec(`INSERT INTO source_songs(id,migration_id,source_id,metadata) SELECT 'other-source',migration_id,'other','{}' FROM source_songs WHERE id=?`, sid); e != nil {
		t.Fatal(e)
	}
	if _, e := a.DB.Exec("INSERT INTO request_sources VALUES('other-source','old')"); e != nil {
		t.Fatal(e)
	}
	if e := a.resolveSource(sid, "uploaded-song", ""); e != nil {
		t.Fatal(e)
	}
	var state string
	if e := a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jid).Scan(&state); e != nil || state != "failed" {
		t.Fatal("still-needed request was retired")
	}
	if e := a.resolveSource("other-source", "uploaded-song", ""); e != nil {
		t.Fatal(e)
	}
	if e := a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jid).Scan(&state); e != nil || state != "superseded" {
		t.Fatal("request not retired after last source was replaced")
	}
}
func TestDismissLegacyOrphanFailurePreservesMusicAndIsAuthorized(t *testing.T) {
	a, admin, member := albumFixture(t)
	jid := failedSourceRequest(t, a, "")
	music := filepath.Join(t.TempDir(), "already-uploaded.flac")
	if e := os.WriteFile(music, []byte("keep this music"), 0600); e != nil {
		t.Fatal(e)
	}
	endpoint := "/api/v2/tasks/" + jid + "/dismiss"
	// The failing request belongs to member; another ordinary user cannot dismiss it.
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"other","password":"test-password-long"}`, ""), 201)
	other := login(t, a, "other")
	expect(t, other.call("POST", endpoint, "", ""), 404)
	expect(t, member.call("POST", endpoint, "", ""), 200)
	expect(t, member.call("POST", endpoint, "", ""), 200)
	expect(t, member.call("POST", "/api/v2/tasks/"+jid, "", ""), 409)
	var state, stage string
	if e := a.DB.QueryRow("SELECT state,stage FROM jobs WHERE id=?", jid).Scan(&state, &stage); e != nil || state != "cancelled" || !strings.Contains(stage, "account country") {
		t.Fatalf("dismissed history: %s %s %v", state, stage, e)
	}
	if data, e := os.ReadFile(music); e != nil || string(data) != "keep this music" {
		t.Fatal("dismiss changed library file")
	}
	if _, e := a.DB.Exec("UPDATE jobs SET state='downloading' WHERE id=?", jid); e != nil {
		t.Fatal(e)
	}
	expect(t, member.call("POST", endpoint, "", ""), 409)
}
