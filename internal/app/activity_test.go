package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestCancelObsoleteMigrationTaskPreservesImportedLibrary(t *testing.T) {
	a, admin, member := albumFixture(t)
	mid, sid := stageAudioMigration(t, member)
	prefix := "/api/v2/imports/" + mid
	expect(t, member.call("POST", prefix+"/commit", `{"playlists":[]}`, ""), 200)
	var tid string
	if err := a.DB.QueryRow("SELECT id FROM migration_tasks WHERE migration_id=?", mid).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	a.DB.Exec("UPDATE migration_tasks SET state='failed',message='Old connection error' WHERE id=?", tid)
	a.DB.Exec("UPDATE migrations SET error='Old connection error' WHERE id=?", mid)
	a.DB.Exec("UPDATE source_songs SET server_id='existing-song' WHERE id=?", sid)
	a.DB.Exec("INSERT INTO migration_task_wakeups VALUES(?,'match')", mid)
	expect(t, member.call("POST", prefix+"/handover", `{}`, ""), 200)
	before, _ := a.sources(mid)
	playlistsBefore, _ := a.playlists(mid)
	endpoint := "/api/v2/tasks/" + tid + "/cancel"
	expect(t, admin.call("POST", "/api/v2/users", `{"username":"other","password":"test-password-long"}`, ""), 201)
	other := login(t, a, "other")
	expect(t, other.call("POST", endpoint, `{}`, ""), 404)
	activity := member.call("GET", "/api/v2/activity", "", "")
	expect(t, activity, 200)
	var rows []struct {
		ID        string `json:"id"`
		CanCancel bool   `json:"canCancel"`
	}
	if err := json.Unmarshal(activity.Body.Bytes(), &rows); err != nil || len(rows) != 1 || !rows[0].CanCancel {
		t.Fatalf("missing cancellation capability: %s %v", activity.Body.String(), err)
	}
	expect(t, member.call("POST", endpoint, `{}`, ""), 200)
	expect(t, member.call("POST", endpoint, `{}`, ""), 200)
	a.runMigrationTask(context.Background())
	var state string
	a.DB.QueryRow("SELECT state FROM migration_tasks WHERE id=?", tid).Scan(&state)
	if state != "cancelled" {
		t.Fatalf("obsolete task restarted: %s", state)
	}
	after, _ := a.sources(mid)
	playlistsAfter, _ := a.playlists(mid)
	if mustJSON(before) != mustJSON(after) || mustJSON(playlistsBefore) != mustJSON(playlistsAfter) {
		t.Fatal("cancelling a task changed imported songs or playlists")
	}
	var owner string
	a.DB.QueryRow("SELECT owner FROM migrations WHERE id=?", mid).Scan(&owner)
	migration, err := a.getMigration(mid, owner)
	if err != nil || migration.State != "handed_over" || migration.Error != "" {
		t.Fatalf("migration result changed or obsolete error remained: %+v %v", migration, err)
	}
	var wakeups int
	a.DB.QueryRow("SELECT count(*) FROM migration_task_wakeups WHERE migration_id=?", mid).Scan(&wakeups)
	if wakeups != 0 {
		t.Fatal("cancelled task retained a pending rerun")
	}
}

func TestCancelMigrationTaskStateGuards(t *testing.T) {
	for _, state := range []string{"queued", "running", "completed"} {
		t.Run(state, func(t *testing.T) {
			a, admin, member := albumFixture(t)
			mid, _ := stageAudioMigration(t, member)
			expect(t, member.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[]}`, ""), 200)
			var tid string
			a.DB.QueryRow("SELECT id FROM migration_tasks WHERE migration_id=?", mid).Scan(&tid)
			a.DB.Exec("UPDATE migration_tasks SET state=? WHERE id=?", state, tid)
			want := 409
			if state == "queued" {
				want = 200
			}
			expect(t, admin.call("POST", "/api/v2/tasks/"+tid+"/cancel", `{}`, ""), want)
			var current string
			a.DB.QueryRow("SELECT state FROM migration_tasks WHERE id=?", tid).Scan(&current)
			if state == "queued" {
				a.runMigrationTask(context.Background())
				current = ""
				a.DB.QueryRow("SELECT state FROM migration_tasks WHERE id=?", tid).Scan(&current)
				state = "cancelled"
			}
			if current != state {
				t.Fatalf("state=%s, want %s", current, state)
			}
		})
	}
}

func TestActivityPrioritizesActiveStagesBeforeLimitingLargeQueue(t *testing.T) {
	a, _, member := albumFixture(t)
	var owner string
	if err := a.DB.QueryRow("SELECT id FROM users WHERE username='member'").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	add := func(job, state, created string) {
		t.Helper()
		if _, err := a.DB.Exec(`INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) VALUES(?,?,?,?,?,'Artist',0,'["128"]','approved')`, job, owner, job, "deezer:track:42", job); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec("INSERT INTO jobs(id,request_id,state,stage,created) VALUES(?,?,?,?,?)", job, job, state, state, created); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 210; i++ {
		add(fmt.Sprintf("queue-%03d", i), "queued", "2026-11-04 12:00:00")
	}
	// Newer queued work and finished history must not hide older active work.
	for _, state := range []string{"downloading", "awaiting_scan", "importing", "validating", "blocked", "available"} {
		add(state, state, "2000-01-01 00:00:00")
	}
	response := member.call("GET", "/api/v2/activity", "", "")
	expect(t, response, 200)
	var rows []struct {
		ID        string `json:"id"`
		State     string `json:"state"`
		CanCancel bool   `json:"canCancel"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 200 {
		t.Fatalf("unexpected activity limit: %d", len(rows))
	}
	want := []string{"downloading", "awaiting_scan", "importing", "validating", "blocked", "queued"}
	for i, state := range want {
		if rows[i].State != state {
			t.Fatalf("row %d state=%q, want %q", i, rows[i].State, state)
		}
	}
	if rows[len(rows)-1].State != "queued" {
		t.Fatal("completed history displaced unfinished queued work")
	}
	for _, row := range rows {
		if row.CanCancel {
			t.Fatal("acquisition job advertised migration cancellation")
		}
	}
	expect(t, member.call("POST", "/api/v2/tasks/downloading/cancel", `{}`, ""), 404)
}

func TestCancellingOldFailurePreservesNewerMigrationTaskAndWakeup(t *testing.T) {
	a, _, member := albumFixture(t)
	mid, _ := stageAudioMigration(t, member)
	expect(t, member.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[]}`, ""), 200)
	var tid, owner string
	a.DB.QueryRow("SELECT id,owner FROM migration_tasks WHERE migration_id=?", mid).Scan(&tid, &owner)
	a.DB.Exec("UPDATE migration_tasks SET state='failed' WHERE id=?", tid)
	newTask, err := a.queueMigration(owner, mid, "match")
	if err != nil {
		t.Fatal(err)
	}
	a.DB.Exec("INSERT INTO migration_task_wakeups VALUES(?,'match')", mid)
	expect(t, member.call("POST", "/api/v2/tasks/"+tid+"/cancel", `{}`, ""), 200)
	var state string
	a.DB.QueryRow("SELECT state FROM migration_tasks WHERE id=?", newTask).Scan(&state)
	var wakeups int
	a.DB.QueryRow("SELECT count(*) FROM migration_task_wakeups WHERE migration_id=?", mid).Scan(&wakeups)
	if state != "queued" || wakeups != 1 {
		t.Fatalf("newer task/rerun changed: %s %d", state, wakeups)
	}
}
