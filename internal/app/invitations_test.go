package app

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func createInvite(t *testing.T, admin client, username string) (string, string) {
	t.Helper()
	w := admin.call("POST", "/api/v2/invitations", `{"username":"`+username+`"}`, "")
	expect(t, w, 201)
	var out struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	if len(out.Token) != 64 {
		t.Fatal("invalid token")
	}
	return out.ID, out.Token
}

func TestInvitationsRequireAdminAndAreSingleUse(t *testing.T) {
	a, admin, member := albumFixture(t)
	public := client{a: a}
	expect(t, public.call("POST", "/api/v2/invitations", `{"username":"new"}`, ""), 401)
	expect(t, member.call("POST", "/api/v2/invitations", `{"username":"new"}`, ""), 403)
	expect(t, member.call("GET", "/api/v2/invitations", "", ""), 403)
	iid, token := createInvite(t, admin, "invited")
	var stored string
	if e := a.DB.QueryRow("SELECT token_hash FROM invitations WHERE id=?", iid).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	if stored == token || stored != digest(token) {
		t.Fatal("invitation token was not hashed")
	}
	listing := admin.call("GET", "/api/v2/invitations", "", "")
	expect(t, listing, 200)
	if strings.Contains(listing.Body.String(), token) || strings.Contains(listing.Body.String(), stored) {
		t.Fatal("list exposed a token")
	}
	expect(t, admin.call("POST", "/api/v2/invitations", `{"username":"invited"}`, ""), 409)
	expect(t, public.call("GET", "/api/v2/invitations/preview?token="+token, "", ""), 200)
	expect(t, public.call("POST", "/api/v2/invitations/accept", `{"token":"`+token+`","password":"short"}`, ""), 400)
	expect(t, public.call("POST", "/api/v2/invitations/accept", `{"token":"`+token+`","password":"test-password-long","role":"admin"}`, ""), 400)
	expect(t, public.call("POST", "/api/v2/invitations/accept", `{"token":"`+token+`","password":"test-password-long"}`, ""), 201)
	expect(t, public.call("POST", "/api/v2/invitations/accept", `{"token":"`+token+`","password":"test-password-long"}`, ""), 404)
	expect(t, public.call("GET", "/api/v2/invitations/preview?token="+token, "", ""), 404)
	invited := login(t, a, "invited")
	response := invited.call("GET", "/api/v2/session", "", "")
	expect(t, response, 200)
	var session struct {
		User User `json:"user"`
	}
	json.Unmarshal(response.Body.Bytes(), &session)
	if session.User.Role != "member" || session.User.Direct || session.User.DirectDelete {
		t.Fatal("invitation granted unexpected privileges")
	}
	var events int
	if e := a.DB.QueryRow("SELECT count(*) FROM events WHERE resource=? AND kind='invitation.accepted'", iid).Scan(&events); e != nil || events != 1 {
		t.Fatalf("audit event count %d: %v", events, e)
	}
}

func TestRevokedExpiredAndConcurrentInvitations(t *testing.T) {
	a, admin, member := albumFixture(t)
	public := client{a: a}
	iid, token := createInvite(t, admin, "revoked")
	expect(t, member.call("DELETE", "/api/v2/invitations/"+iid, "", ""), 403)
	expect(t, admin.call("DELETE", "/api/v2/invitations/"+iid, "", ""), 200)
	expect(t, public.call("POST", "/api/v2/invitations/accept", `{"token":"`+token+`","password":"test-password-long"}`, ""), 404)
	iid, token = createInvite(t, admin, "expired")
	a.DB.Exec("UPDATE invitations SET expires=? WHERE id=?", time.Now().Add(-time.Hour).Unix(), iid)
	expect(t, public.call("GET", "/api/v2/invitations/preview?token="+token, "", ""), 404)
	expect(t, public.call("POST", "/api/v2/invitations/accept", `{"token":"`+token+`","password":"test-password-long"}`, ""), 404)
	_, token = createInvite(t, admin, "concurrent")
	codes := make(chan int, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			codes <- public.call("POST", "/api/v2/invitations/accept", `{"token":"`+token+`","password":"test-password-long"}`, "").Code
		}()
	}
	group.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 201 {
			success++
		} else if code != 404 {
			t.Fatalf("unexpected concurrent response %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("expected one acceptance, got %d", success)
	}
}

func TestCombinedPermissionUpdatesAreAtomicAndValidated(t *testing.T) {
	a, admin, member := albumFixture(t)
	var uid string
	a.DB.QueryRow("SELECT id FROM users WHERE username='member'").Scan(&uid)
	path := "/api/v2/users/" + uid + "/permissions"
	expect(t, member.call("PUT", path, `{"direct":true,"directDelete":true}`, ""), 403)
	expect(t, admin.call("PUT", path, `{}`, ""), 400)
	expect(t, admin.call("PUT", path, `{"direct":true,"directDelete":true}`, ""), 200)
	var direct, deleteDirect bool
	a.DB.QueryRow("SELECT direct,EXISTS(SELECT 1 FROM delete_permissions WHERE user_id=users.id) FROM users WHERE id=?", uid).Scan(&direct, &deleteDirect)
	if !direct || !deleteDirect {
		t.Fatal("permissions were not updated together")
	}
	expect(t, admin.call("PUT", path, `{"direct":false,"directDelete":false}`, ""), 200)
	a.DB.QueryRow("SELECT direct,EXISTS(SELECT 1 FROM delete_permissions WHERE user_id=users.id) FROM users WHERE id=?", uid).Scan(&direct, &deleteDirect)
	if direct || deleteDirect {
		t.Fatal("permissions were not removed")
	}
}
