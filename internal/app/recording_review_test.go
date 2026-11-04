package app

import (
	"encoding/json"
	"testing"
)

type recordingReviewContext struct {
	Migration Migration `json:"migration"`
	Source    Source    `json:"source"`
}

func fetchRecordingContext(t *testing.T, c client, jid string) recordingReviewContext {
	t.Helper()
	w := c.call("POST", "/api/v2/tasks/"+jid+"/recording-review", "", "")
	expect(t, w, 200)
	var result recordingReviewContext
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	return result
}
func TestRecordingReviewReusesPrivateImportWithoutNavigationOrNewSnapshot(t *testing.T) {
	a, _, member := albumFixture(t)
	mid, sid := stageAudioMigration(t, member)
	expect(t, member.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[]}`, ""), 200)
	jid := failedSourceRequest(t, a, sid)
	result := fetchRecordingContext(t, member, jid)
	if result.Migration.ID != mid || result.Source.ID != sid || result.Source.JobState != "failed" {
		t.Fatalf("wrong review context: %+v", result)
	}
	var count int
	if e := a.DB.QueryRow("SELECT count(*) FROM migrations").Scan(&count); e != nil || count != 1 {
		t.Fatalf("duplicated import: %d %v", count, e)
	}
}
func TestOrphanRecordingReviewCreatesIdempotentPrivateEditableContext(t *testing.T) {
	a, admin, member := albumFixture(t)
	jid := failedSourceRequest(t, a, "")
	track := makeTrack(111599580, "Make It Rain")
	track.Artist.Name = "Ed Sheeran"
	track.Duration = 404
	if _, e := a.DB.Exec("INSERT INTO request_metadata(request_id,metadata) VALUES('old',?)", mustJSON(track)); e != nil {
		t.Fatal(e)
	}
	ctx := fetchRecordingContext(t, member, jid)
	if ctx.Migration.State != "open" || ctx.Source.Metadata.Title != "Make It Rain" || ctx.Source.RequestID != "old" || ctx.Source.Candidate == nil || ctx.Source.Candidate.ID != track.ID {
		t.Fatalf("unusable orphan context: %+v", ctx)
	}
	second := fetchRecordingContext(t, member, jid)
	if second.Source.ID != ctx.Source.ID || second.Migration.ID != ctx.Migration.ID {
		t.Fatal("reopening created duplicate workspace")
	}
	imports := member.call("GET", "/api/v2/imports", "", "")
	expect(t, imports, 200)
	var list []Migration
	if e := json.Unmarshal(imports.Body.Bytes(), &list); e != nil || len(list) != 0 {
		t.Fatalf("request review cluttered imports: %s %v", imports.Body, e)
	}
	// The normal import upload route works from this same context and preserves
	// the usual approval requirement for a member's replacement upload.
	audio := audioFixture(t, t.TempDir(), ".flac", "flac", true)
	upload := uploadAudioForTest(member, "/api/v2/imports/"+ctx.Migration.ID+"/songs/"+ctx.Source.ID+"/upload", "replacement.flac", "context-upload-key", audio)
	expect(t, upload, 201)
	var uploaded struct{ ID, Decision string }
	if e := json.Unmarshal(upload.Body.Bytes(), &uploaded); e != nil || uploaded.Decision != "awaiting_approval" {
		t.Fatalf("upload bypassed approval: %s %v", upload.Body, e)
	}
	var state string
	if e := a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jid).Scan(&state); e != nil || state != "superseded" {
		t.Fatalf("orphan failure not superseded: %s %v", state, e)
	}
	// A different ordinary user cannot open or modify the private context.
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"other","password":"test-password-long"}`, ""), 201)
	other := login(t, a, "other")
	expect(t, other.call("POST", "/api/v2/tasks/"+jid+"/recording-review", "", ""), 404)
	expect(t, other.call("GET", "/api/v2/imports/"+ctx.Migration.ID, "", ""), 404)
	expect(t, other.call("POST", "/api/v2/imports/"+ctx.Migration.ID+"/songs/"+ctx.Source.ID+"/candidates", `{"query":"Make It Rain"}`, ""), 404)
}

func TestOrphanRecordingReviewCanRequestAnotherDeezerRecording(t *testing.T) {
	a, _, member := albumFixture(t)
	jid := failedSourceRequest(t, a, "")
	ctx := fetchRecordingContext(t, member, jid)
	prefix := "/api/v2/imports/" + ctx.Migration.ID + "/songs/" + ctx.Source.ID
	expect(t, member.call("POST", prefix+"/resolve", `{"target":"deezer:track:1"}`, ""), 200)
	result := member.call("POST", prefix+"/acquire", "", "")
	expect(t, result, 201)
	var created struct {
		ID string `json:"id"`
	}
	if e := json.Unmarshal(result.Body.Bytes(), &created); e != nil {
		t.Fatal(e)
	}
	var decision, target string
	if e := a.DB.QueryRow("SELECT decision,target FROM requests WHERE id=?", created.ID).Scan(&decision, &target); e != nil || decision != "awaiting_approval" || target != "deezer:track:1" {
		t.Fatalf("replacement request: %s %s %v", decision, target, e)
	}
	var state string
	if e := a.DB.QueryRow("SELECT state FROM jobs WHERE id=?", jid).Scan(&state); e != nil || state != "superseded" {
		t.Fatalf("old failure still active: %s %v", state, e)
	}
}
