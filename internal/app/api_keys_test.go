package app

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUnifiedAPIKeyScopesExpirationAndOwnership(t *testing.T) {
	f := newDeletionFixture(t)
	for _, body := range []string{`{}`, `{"label":"app","scopes":[]}`, `{"label":"app","scopes":["admin"]}`, `{"label":"app","scopes":["search:read"],"expires":1}`} {
		expect(t, f.member.call("POST", "/api/v2/api-keys", body, ""), 400)
	}
	w := f.member.call("POST", "/api/v2/api-keys", `{"label":"My app","scopes":["requests:read","deletion:request"]}`, "")
	expect(t, w, 201)
	var key APIKey
	if e := json.Unmarshal(w.Body.Bytes(), &key); e != nil {
		t.Fatal(e)
	}
	call := func(method, path, profile string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+key.Token)
		r.Header.Set("X-Profile-Id", profile)
		w := httptest.NewRecorder()
		f.a.Handler().ServeHTTP(w, r)
		return w
	}
	expect(t, call("GET", "/api/v2/requests", ""), 200)
	expect(t, call("POST", "/api/v2/requests", ""), 403)
	expect(t, call("GET", "/api/v2/api-keys", ""), 403)
	expect(t, call("GET", "/api/v2/requests", "other-user"), 403)
	expect(t, f.admin.call("DELETE", "/api/v2/api-keys/"+key.ID, "", ""), 404)
	listed := f.member.call("GET", "/api/v2/api-keys", "", "")
	expect(t, listed, 200)
	if strings.Contains(listed.Body.String(), key.Token) || !strings.Contains(listed.Body.String(), "My app") {
		t.Fatal(listed.Body.String())
	}
	if _, e := f.a.DB.Exec("UPDATE api_keys SET expires=? WHERE id=?", time.Now().Unix()-1, key.ID); e != nil {
		t.Fatal(e)
	}
	expect(t, call("GET", "/api/v2/requests", ""), 401)
	expect(t, f.member.call("DELETE", "/api/v2/api-keys/"+key.ID, "", ""), 200)
	expect(t, call("GET", "/api/v2/requests", ""), 401)
}

func TestLegacyAPIKeyMigration(t *testing.T) {
	f := newDeletionFixture(t)
	owner := f.userID(t, f.member)
	path := f.a.dataDir + "/test.db"
	if _, e := f.a.DB.Exec(`CREATE TABLE deletion_api_keys(id TEXT PRIMARY KEY,owner TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.a.DB.Exec("INSERT INTO deletion_api_keys(id,owner,token_hash) VALUES('legacy',?,?)", owner, digest("original-token")); e != nil {
		t.Fatal(e)
	}
	if e := f.a.DB.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	f.a.DB = reopened.DB
	var label, scopes string
	if e = f.a.DB.QueryRow("SELECT label,scopes FROM api_keys WHERE id='legacy' AND token_hash=?", digest("original-token")).Scan(&label, &scopes); e != nil {
		t.Fatal(e)
	}
	if label != "Deletion key" || scopes != `["deletion:request"]` {
		t.Fatal(label, scopes)
	}
	expect(t, f.member.call("DELETE", "/api/v2/api-keys/legacy", "", ""), 200)
	f.a.DB.Close()
	reopened, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	f.a.DB = reopened.DB
	var count int
	if e = f.a.DB.QueryRow("SELECT count(*) FROM api_keys WHERE id='legacy'").Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
