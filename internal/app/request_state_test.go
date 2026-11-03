package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRequestExecutionState(t *testing.T) {
	a, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	fixtureMetadata(t, a)
	if err = a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	expect(t, admin.call("POST", "/api/v2/requests", `{"target":"deezer:track:123"}`, "execution-state"), 201)
	for _, state := range []string{"queued", "downloading", "awaiting_scan", "blocked", "failed", "available"} {
		if _, err := a.DB.Exec("UPDATE jobs SET state=?", state); err != nil {
			t.Fatal(err)
		}
		w := admin.call("GET", "/api/v2/requests", "", "")
		expect(t, w, 200)
		var requests []Request
		if err := json.Unmarshal(w.Body.Bytes(), &requests); err != nil {
			t.Fatal(err)
		}
		want := state
		if state == "available" {
			want = "completed"
		}
		if len(requests) != 1 || requests[0].State != want || requests[0].Decision != "approved" {
			t.Fatalf("state %s: %s", state, w.Body)
		}
	}
}
