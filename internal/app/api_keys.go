package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

var apiKeyScopes = map[string]bool{
	"search:read": true, "library:read": true, "requests:read": true,
	"requests:create": true, "deletion:request": true,
}

type APIKey struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Scopes  []string `json:"scopes"`
	Expires *int64   `json:"expires"`
	Created string   `json:"created"`
	Token   string   `json:"token,omitempty"`
}

// Key management requires a browser session. The former deletion routes are
// aliases over this same store, preserving existing integrations.
func (a *App) apiKeyRoutes(w http.ResponseWriter, r *http.Request, u User) {
	base := "/api/v2/api-keys"
	legacy := strings.HasPrefix(r.URL.Path, "/api/v2/deletion-api-keys")
	if legacy {
		base = "/api/v2/deletion-api-keys"
	}
	if r.URL.Path != base {
		key := strings.TrimPrefix(r.URL.Path, base+"/")
		if key == "" || strings.Contains(key, "/") {
			fail(w, 404, "API key not found")
			return
		}
		if r.Method != "DELETE" {
			fail(w, 405, "Method not allowed")
			return
		}
		result, e := a.DB.Exec("DELETE FROM api_keys WHERE id=? AND owner=?", key, u.ID)
		if e != nil {
			a.internal(w, e)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			fail(w, 404, "API key not found")
			return
		}
		requestLogger(r.Context()).Info("API key revoked", "revoked_key_id", key)
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query("SELECT id,label,scopes,expires,created FROM api_keys WHERE owner=? ORDER BY created,id", u.ID)
		if e != nil {
			a.internal(w, e)
			return
		}
		defer rows.Close()
		out := []APIKey{}
		for rows.Next() {
			var key APIKey
			var scopes string
			if e = rows.Scan(&key.ID, &key.Label, &scopes, &key.Expires, &key.Created); e == nil {
				e = json.Unmarshal([]byte(scopes), &key.Scopes)
			}
			if e != nil {
				a.internal(w, e)
				return
			}
			out = append(out, key)
		}
		if e = rows.Err(); e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, out)
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "Method not allowed")
		return
	}
	var in struct {
		Label   string   `json:"label"`
		Scopes  []string `json:"scopes"`
		Expires *int64   `json:"expires"`
	}
	if !decode(w, r, &in) {
		return
	}
	if legacy && in.Label == "" && in.Scopes == nil {
		in.Label = "Deletion key"
		in.Scopes = []string{"deletion:request"}
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || utf8.RuneCountInString(in.Label) > 100 {
		fail(w, 400, "Enter a label of 1 to 100 characters")
		return
	}
	if len(in.Scopes) == 0 {
		fail(w, 400, "Select at least one scope")
		return
	}
	scopes := []string{}
	seen := map[string]bool{}
	for _, scope := range in.Scopes {
		if !apiKeyScopes[scope] {
			fail(w, 400, "Unknown API key scope")
			return
		}
		if !seen[scope] {
			scopes = append(scopes, scope)
			seen[scope] = true
		}
	}
	if in.Expires != nil && *in.Expires <= time.Now().Unix() {
		fail(w, 400, "Expiration must be in the future")
		return
	}
	encoded, _ := json.Marshal(scopes)
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var count int
	if e = tx.QueryRow("SELECT count(*) FROM api_keys WHERE owner=?", u.ID).Scan(&count); e != nil {
		a.internal(w, e)
		return
	}
	if count >= 10 {
		fail(w, 409, "Revoke an existing API key before creating another")
		return
	}
	key := APIKey{ID: id(), Token: id(), Label: in.Label, Scopes: scopes, Expires: in.Expires}
	_, e = tx.Exec("INSERT INTO api_keys(id,owner,token_hash,label,scopes,expires) VALUES(?,?,?,?,?,?)", key.ID, u.ID, digest(key.Token), key.Label, string(encoded), key.Expires)
	if e == nil {
		e = audit(tx, u.ID, u.ID, "api_key.created", key.ID)
	}
	if e == nil {
		e = tx.QueryRow("SELECT created FROM api_keys WHERE id=?", key.ID).Scan(&key.Created)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	requestLogger(r.Context()).Info("API key created", "created_key_id", key.ID, "scopes", key.Scopes, "expires", key.Expires)
	send(w, 201, key)
}

func (a *App) apiKeyBearer(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var u User
	var scopes, quality, keyID string
	e := a.DB.QueryRow(`SELECT u.id,u.username,u.role,u.direct,u.quality,
 EXISTS(SELECT 1 FROM delete_permissions p WHERE p.user_id=u.id),k.scopes,k.id
 FROM api_keys k JOIN users u ON u.id=k.owner
 WHERE k.token_hash=? AND (k.expires IS NULL OR k.expires>?)`, digest(token), time.Now().Unix()).Scan(&u.ID, &u.Username, &u.Role, &u.Direct, &quality, &u.DirectDelete, &scopes, &keyID)
	if e == sql.ErrNoRows {
		fail(w, 401, "Invalid or expired API key")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	setRequestActor(r, u.ID, keyID)
	u.Quality = json.RawMessage(quality)
	// A key always acts as its owner, including when a compatibility client sends a profile.
	if profile := r.Header.Get("X-Profile-Id"); profile != "" && profile != u.ID && !(strings.HasPrefix(r.URL.Path, "/api/v1/") && profile == "1") {
		fail(w, 403, "Profile does not belong to this API key")
		return
	}
	var allowed []string
	if e = json.Unmarshal([]byte(scopes), &allowed); e != nil {
		a.internal(w, e)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		a.soulSyncRoutes(w, r, u, allowed)
		return
	}
	path := r.URL.Path
	required := ""
	switch {
	case path == "/api/v2/search" && r.Method == "GET":
		required = "search:read"
	case path == "/api/v2/library" && r.Method == "GET":
		required = "library:read"
	case path == "/api/v2/requests" && r.Method == "GET":
		required = "requests:read"
	case path == "/api/v2/requests" && r.Method == "POST":
		required = "requests:create"
	case path == "/api/v2/deletion-requests" && r.Method == "POST":
		required = "deletion:request"
	case strings.HasPrefix(path, "/api/v2/deletion-requests/") && r.Method == "GET":
		required = "deletion:request"
	}
	granted := false
	for _, scope := range allowed {
		if scope == required {
			granted = true
		}
	}
	if required == "" || !granted {
		fail(w, 403, "API key does not permit this action")
		return
	}
	switch required {
	case "search:read":
		a.search(w, r)
	case "library:read":
		a.library(w, r)
	case "requests:create":
		a.createRequest(w, r, u)
	case "requests:read":
		// Even an administrator's integration key reads only its owner's requests.
		u.Role = "member"
		a.listRequests(w, u)
	case "deletion:request":
		if r.Method == "POST" {
			a.createDeletionRequest(w, r, u)
			return
		}
		rid := strings.TrimPrefix(path, "/api/v2/deletion-requests/")
		var decision, reason, state, detail string
		e = a.DB.QueryRow(`SELECT r.decision,r.reason,COALESCE(j.state,''),COALESCE(j.stage,'') FROM requests r JOIN song_deletions d ON d.request_id=r.id LEFT JOIN jobs j ON j.request_id=r.id WHERE r.id=? AND r.owner=?`, rid, u.ID).Scan(&decision, &reason, &state, &detail)
		if e == sql.ErrNoRows {
			fail(w, 404, "Deletion request not found")
			return
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]string{"id": rid, "decision": decision, "reason": reason, "state": state, "detail": detail})
	}
}
