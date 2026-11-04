package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBulkMigrationRequestsPersistFor3000SongsDespiteDisconnectedClient(t *testing.T) {
	a, admin, member := albumFixture(t)
	mid, sources := largeImportFixture(t, admin)
	track := makeTrack(42, "Song")
	track.Artist.Name = "Artist"
	track.Album.Title = "Album"
	if _, err := a.DB.Exec(`UPDATE source_songs SET candidate=json_set(?,'$.id',CAST(source_id AS INTEGER),'$.title',json_extract(metadata,'$.title')) WHERE migration_id=?`, mustJSON(track), mid); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, source := range sources {
		ids = append(ids, source.ID)
	}
	ids = append(ids, ids[0]) // Playlist occurrences must not create extra requests.
	body := mustJSON(map[string]any{"sourceIds": ids})
	if len(body) <= 16384 {
		t.Fatal("fixture does not exercise large request bodies")
	}
	endpoint := "/api/v2/imports/" + mid + "/acquire"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest("POST", admin.baseURL()+endpoint, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Origin", admin.baseURL())
	request.Header.Set("X-CSRF-Token", admin.csrf)
	request.Header.Set("Idempotency-Key", "bulk-3000-key")
	request.AddCookie(admin.cookie)
	response := httptest.NewRecorder()
	// The body has arrived, but the browser connection is gone. Enqueuing must
	// still finish independently and atomically on the server.
	a.Handler().ServeHTTP(response, request)
	expect(t, response, 201)
	var result struct {
		Bulk migrationAcquisitionResult `json:"bulk"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Bulk.Created != 3000 || result.Bulk.Existing != 0 || result.Bulk.Skipped != 0 {
		t.Fatalf("incomplete batch: %s %v", response.Body.String(), err)
	}
	if response.Body.Len() > 1024 {
		t.Fatal("bulk response returned the entire song catalog")
	}
	for _, query := range []string{"SELECT count(*) FROM requests", "SELECT count(*) FROM request_metadata", "SELECT count(*) FROM request_sources", "SELECT count(*) FROM jobs WHERE state='queued'"} {
		var count int
		if err := a.DB.QueryRow(query).Scan(&count); err != nil || count != 3000 {
			t.Fatalf("batch not durable: %s = %d, %v", query, count, err)
		}
	}
	expect(t, member.call("POST", endpoint, body, "foreign-batch-key"), 404)
	expect(t, admin.call("POST", endpoint, body, "bulk-3000-key"), 200)
	// A new submission skips requests already saved by a different submission.
	second := admin.call("POST", endpoint, body, "another-bulk-key")
	expect(t, second, 201)
	json.Unmarshal(second.Body.Bytes(), &result)
	if result.Bulk.Created != 0 || result.Bulk.Existing != 3000 {
		t.Fatalf("existing requests duplicated: %s", second.Body.String())
	}
	// Even after a decision changes, replaying an accepted batch cannot submit
	// a fresh request for that source.
	if _, err := a.DB.Exec("UPDATE requests SET decision='declined' WHERE rowid=(SELECT min(rowid) FROM requests)"); err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("POST", endpoint, body, "bulk-3000-key"), 200)
	var count int
	a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&count)
	if count != 3000 {
		t.Fatal("replayed batch created requests after a decision change")
	}
	expect(t, admin.call("POST", endpoint, mustJSON(map[string]any{"sourceIds": ids[:1]}), "bulk-3000-key"), 409)
	var seq int
	var name, path string
	if err := a.DB.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	if err = reopened.DB.QueryRow("SELECT count(*) FROM jobs WHERE state='queued'").Scan(&count); err != nil || count != 3000 {
		t.Fatalf("queue lost after reopening database: %d %v", count, err)
	}
}

func TestBulkMigrationRequestsValidateAtomicallyAndPreserveApproval(t *testing.T) {
	a, admin, member := albumFixture(t)
	mid, sid := stageAudioMigration(t, member)
	endpoint := "/api/v2/imports/" + mid + "/acquire"
	track := makeTrack(42, "Selected recording")
	a.DB.Exec("UPDATE source_songs SET candidate=? WHERE id=?", mustJSON(track), sid)
	body := mustJSON(map[string]any{"sourceIds": []string{sid}})
	expect(t, member.call("POST", endpoint, body, "staged-bulk-key"), 409)
	expect(t, member.call("POST", "/api/v2/imports/"+mid+"/commit", `{"playlists":[]}`, ""), 200)
	expect(t, member.call("POST", endpoint, body, ""), 400)
	invalid := mustJSON(map[string]any{"sourceIds": []string{sid, "zz-not-a-source"}})
	expect(t, member.call("POST", endpoint, invalid, "invalid-bulk-key"), 404)
	var count int
	a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&count)
	if count != 0 {
		t.Fatal("invalid selection left a partial batch")
	}
	a.DB.QueryRow("SELECT count(*) FROM migration_acquisition_batches").Scan(&count)
	if count != 0 {
		t.Fatal("invalid batch retained an accepted receipt")
	}
	// Available, deferred and unresolved songs changed since selection are
	// skipped without acquiring anything on the user's behalf.
	for i, values := range []string{"server_id='already-available'", "server_id='',deferred=1", "deferred=0,candidate=''"} {
		a.DB.Exec("UPDATE source_songs SET "+values+" WHERE id=?", sid)
		response := member.call("POST", endpoint, body, []string{"available-bulk-key", "deferred-bulk-key", "unresolved-bulk-key"}[i])
		expect(t, response, 201)
		var result struct {
			Bulk migrationAcquisitionResult `json:"bulk"`
		}
		json.Unmarshal(response.Body.Bytes(), &result)
		if result.Bulk.Created != 0 || result.Bulk.Skipped != 1 {
			t.Fatalf("unready source requested: %s", response.Body.String())
		}
	}
	a.DB.Exec("UPDATE source_songs SET candidate=? WHERE id=?", mustJSON(track), sid)
	response := member.call("POST", endpoint, body, "member-bulk-key")
	expect(t, response, 201)
	var decision, quality string
	a.DB.QueryRow("SELECT decision,quality FROM requests").Scan(&decision, &quality)
	if decision != "awaiting_approval" {
		t.Fatalf("member bypassed approval: %s", decision)
	}
	a.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&count)
	if count != 0 {
		t.Fatal("member batch created jobs before approval")
	}
	var ownerQuality string
	a.DB.QueryRow("SELECT quality FROM users WHERE username='member'").Scan(&ownerQuality)
	if quality != ownerQuality {
		t.Fatal("batch changed the requested quality snapshot")
	}
	var rid string
	a.DB.QueryRow("SELECT id FROM requests").Scan(&rid)
	expect(t, admin.call("POST", "/api/v2/requests/"+rid, `{"decision":"approved"}`, ""), 200)
	a.DB.QueryRow("SELECT count(*) FROM jobs WHERE state='queued'").Scan(&count)
	if count != 1 {
		t.Fatal("approved batch did not enter normal worker queue")
	}
	expect(t, member.call("DELETE", "/api/v2/imports/"+mid, "", ""), 200)
	a.DB.QueryRow("SELECT count(*) FROM migration_acquisition_batches").Scan(&count)
	if count != 0 {
		t.Fatal("deleted migration retained batch receipts")
	}
	a.DB.QueryRow("SELECT count(*) FROM requests").Scan(&count)
	if count != 1 {
		t.Fatal("migration deletion removed independent acquisition requests")
	}
}
