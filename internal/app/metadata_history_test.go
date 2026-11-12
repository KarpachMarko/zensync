package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMetadataActivityHistoryAndRetry(t *testing.T) {
	a, admin, songs := metadataLibraryFixture(t)
	var actor string
	a.DB.QueryRow("SELECT id FROM users WHERE username='admin'").Scan(&actor)
	changes := []metadataChange{
		{Song: songs["two"], Edit: MetadataEdit{Title: "Skate", Artist: "Vacio", Album: "Skate"}},
	}
	for _, state := range []string{"queued", "running", "completed", "failed"} {
		message := "Tags updated; scan requested"
		if state == "failed" {
			message = "0/1 songs updated: Could not find source"
		}
		_, err := a.DB.Exec("INSERT INTO metadata_jobs(id,actor,changes,server_url,state,message,done) VALUES(?,?,?,?,?,?,?)", "metadata-"+state, actor, mustJSON(changes), "server", state, message, 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	// A damaged historical payload must not break all Activity rows.
	if _, err := a.DB.Exec("INSERT INTO metadata_jobs(id,actor,changes,server_url,state) VALUES('metadata-damaged',?,'not json','server','failed')", actor); err != nil {
		t.Fatal(err)
	}
	result := admin.call("GET", "/api/v2/activity", "", "")
	expect(t, result, 200)
	var rows []struct {
		ID, Title, State, Stage, Kind    string
		CanCancel, CanReview, CanDismiss bool
	}
	if err := json.Unmarshal(result.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.ID] = true
		if row.Kind != "metadata" || row.CanCancel || row.CanReview || row.CanDismiss {
			t.Fatalf("Wrong metadata actions: %+v", row)
		}
		if row.ID == "metadata-failed" && strings.Count(row.Stage, "0/1") != 1 {
			t.Fatalf("Duplicate progress: %s", row.Stage)
		}
		if row.ID != "metadata-damaged" && row.Title != "Edit metadata / Skate" {
			t.Fatalf("Missing operation title: %+v", row)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("Missing metadata history: %+v", rows)
	}
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, ""), 201)
	member := login(t, a, "member")
	result = member.call("GET", "/api/v2/activity", "", "")
	expect(t, result, 200)
	if strings.Contains(result.Body.String(), "metadata-") {
		t.Fatal("Metadata history exposed to a non-admin")
	}
	// Keep metadata retries on their own endpoint and preserve the busy guard.
	expect(t, admin.call("POST", "/api/v2/metadata/jobs", `{"id":"metadata-failed"}`, ""), 409)
	a.DB.Exec("UPDATE metadata_jobs SET state='completed' WHERE state IN ('queued','running')")
	expect(t, admin.call("POST", "/api/v2/metadata/jobs", `{"id":"metadata-failed"}`, ""), 200)
	var state string
	a.DB.QueryRow("SELECT state FROM metadata_jobs WHERE id='metadata-failed'").Scan(&state)
	if state != "queued" {
		t.Fatal("Retry was not queued")
	}
}
