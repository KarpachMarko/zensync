package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteUnfinishedMigration(t *testing.T) {
	for _, state := range []string{"staged", "open"} {
		t.Run(state, func(t *testing.T) {
			a, err := Open(filepath.Join(t.TempDir(), "app.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer a.DB.Close()
			c := client{a: a}
			expect(t, c.call("POST", "/api/v2/setup", `{"username":"admin","password":"test-password-long"}`, ""), 201)
			admin := login(t, a, "admin")
			expect(t, admin.call("POST", "/api/v2/users", `{"username":"other","password":"test-password-long"}`, ""), 201)
			other := login(t, a, "other")
			data, err := os.ReadFile("../apple/testdata/library.xml")
			if err != nil {
				t.Fatal(err)
			}
			upload := func(c client) string {
				w := c.call("POST", "/api/v2/imports", string(data), "")
				expect(t, w, 201)
				var result map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				return result["id"]
			}
			mid, untouched := upload(admin), upload(other)
			sources, err := a.sources(mid)
			if err != nil {
				t.Fatal(err)
			}
			otherSources, err := a.sources(untouched)
			if err != nil {
				t.Fatal(err)
			}
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := a.DB.Exec(query, args...); err != nil {
					t.Fatal(err)
				}
			}
			var owner string
			if err := a.DB.QueryRow("SELECT owner FROM migrations WHERE id=?", mid).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			exec("UPDATE migrations SET state=? WHERE id=?", state, mid)
			exec("INSERT INTO requests(id,owner,idem,target,title,artist,upgrade,quality,decision) VALUES('request',?,'delete-test','deezer:track:2','Song','Artist',0,'[]','approved')", owner)
			exec("INSERT INTO jobs(id,request_id) VALUES('job','request')")
			exec("INSERT INTO request_sources VALUES(?,'request'),(?,'request')", sources[0].ID, otherSources[0].ID)
			exec("INSERT INTO source_resolutions VALUES(?,'resolution')", sources[0].ID)
			exec("INSERT INTO migration_tasks(id,owner,migration_id,action) VALUES('task',?,?,'match')", owner, mid)
			exec("INSERT INTO migration_task_wakeups VALUES(?,'match')", mid)
			exec("UPDATE migration_playlists SET remote_id='published-playlist' WHERE migration_id=?", mid)
			prefix := "/api/v2/imports/" + mid
			expect(t, other.call("DELETE", prefix, "", ""), 404)
			noCSRF := admin
			noCSRF.csrf = ""
			expect(t, noCSRF.call("DELETE", prefix, "", ""), 403)
			expect(t, admin.call("DELETE", prefix, "", ""), 200)
			expect(t, admin.call("GET", prefix, "", ""), 404)
			expect(t, admin.call("DELETE", prefix, "", ""), 404)
			expect(t, other.call("GET", "/api/v2/imports/"+untouched, "", ""), 200)
			count := func(query string, want int, args ...any) {
				t.Helper()
				var got int
				if err := a.DB.QueryRow(query, args...).Scan(&got); err != nil || got != want {
					t.Fatalf("%s: got %d want %d, %v", query, got, want, err)
				}
			}
			for _, table := range []string{"source_songs", "migration_playlists", "migration_tasks", "migration_task_wakeups"} {
				count("SELECT count(*) FROM "+table+" WHERE migration_id=?", 0, mid)
			}
			count("SELECT count(*) FROM source_resolutions WHERE source_id=?", 0, sources[0].ID)
			count("SELECT count(*) FROM request_sources WHERE source_id=?", 0, sources[0].ID)
			count("SELECT count(*) FROM request_sources WHERE source_id=?", 1, otherSources[0].ID)
			count("SELECT count(*) FROM jobs WHERE id='job' AND state='queued'", 1)
			count("SELECT count(*) FROM requests WHERE id='request' AND decision='approved'", 1)
			count("SELECT count(*) FROM events WHERE resource=? AND kind='migration.deleted'", 1, mid)
			if reimported := upload(admin); reimported == mid {
				t.Fatal("deleted snapshot reused")
			}
		})
	}
}

func TestCannotDeleteFinishedMigration(t *testing.T) {
	a, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	c := client{a: a}
	expect(t, c.call("POST", "/api/v2/setup", `{"username":"admin","password":"test-password-long"}`, ""), 201)
	c = login(t, a, "admin")
	if _, err := a.DB.Exec("INSERT INTO migrations(id,owner,fingerprint,name,source,report,state) SELECT 'finished',id,'fingerprint','Finished','','{}','handed_over' FROM users WHERE username='admin'"); err != nil {
		t.Fatal(err)
	}
	expect(t, c.call("DELETE", "/api/v2/imports/finished", "", ""), 409)
	expect(t, c.call("GET", "/api/v2/imports/finished", "", ""), 200)
}
