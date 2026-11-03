package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestEditUserPermissions(t *testing.T) {
	a, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB.Close()
	if err = a.createAdmin("admin", "test-password-long"); err != nil {
		t.Fatal(err)
	}
	admin := login(t, a, "admin")
	w := admin.call("POST", "/api/v2/users", `{"username":"member","password":"test-password-long"}`, "")
	expect(t, w, 201)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	member := login(t, a, "member")
	endpoint := "/api/v2/users/" + created.ID
	expect(t, member.call("PUT", endpoint, `{"direct":true}`, ""), 404)
	expect(t, admin.call("PUT", endpoint, `{}`, ""), 400)
	expect(t, admin.call("PUT", "/api/v2/users/missing", `{"direct":true}`, ""), 404)
	for _, value := range []string{"true", "false"} {
		expect(t, admin.call("PUT", endpoint, `{"direct":`+value+`}`, ""), 200)
		w = member.call("GET", "/api/v2/session", "", "")
		expect(t, w, 200)
		var session struct {
			User User `json:"user"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
			t.Fatal(err)
		}
		if session.User.Direct != (value == "true") {
			t.Fatal("existing session did not reflect permission change")
		}
	}
	var adminID string
	if err := a.DB.QueryRow("SELECT id FROM users WHERE role='admin'").Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	expect(t, admin.call("PUT", "/api/v2/users/"+adminID, `{"direct":false}`, ""), 400)
}
