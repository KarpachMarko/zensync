package app

import (
	"database/sql"
	"net/http"
	"strings"
)

// These keys deliberately authorize only deletion requests and their status.
// Decisions and permission administration continue to require browser sessions.
func (a *App) deletionKeyRoutes(w http.ResponseWriter, r *http.Request, u User) {
	if r.URL.Path != "/api/v2/deletion-api-keys" && r.Method == "DELETE" {
		key := strings.TrimPrefix(r.URL.Path, "/api/v2/deletion-api-keys/")
		result, e := a.DB.Exec("DELETE FROM deletion_api_keys WHERE id=? AND owner=?", key, u.ID)
		if e != nil {
			a.internal(w, e)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			fail(w, 404, "API key not found")
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query("SELECT id,created FROM deletion_api_keys WHERE owner=? ORDER BY created,id", u.ID)
		if e != nil {
			a.internal(w, e)
			return
		}
		defer rows.Close()
		out := []map[string]string{}
		for rows.Next() {
			var key, created string
			if e = rows.Scan(&key, &created); e != nil {
				a.internal(w, e)
				return
			}
			out = append(out, map[string]string{"id": key, "created": created})
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
	token, key := id(), id()
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var count int
	if e = tx.QueryRow("SELECT count(*) FROM deletion_api_keys WHERE owner=?", u.ID).Scan(&count); e != nil {
		a.internal(w, e)
		return
	}
	if count >= 10 {
		fail(w, 409, "Revoke an existing deletion API key before creating another")
		return
	}
	_, e = tx.Exec("INSERT INTO deletion_api_keys(id,owner,token_hash) VALUES(?,?,?)", key, u.ID, digest(token))
	if e == nil {
		e = audit(tx, u.ID, u.ID, "deletion.api_key.created", key)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 201, map[string]string{"id": key, "token": token, "scope": "deletion:request"})
}

func (a *App) deletionBearer(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var u User
	e := a.DB.QueryRow(`SELECT u.id,u.username,u.role,EXISTS(SELECT 1 FROM delete_permissions p WHERE p.user_id=u.id) FROM deletion_api_keys k JOIN users u ON u.id=k.owner WHERE k.token_hash=?`, digest(token)).Scan(&u.ID, &u.Username, &u.Role, &u.DirectDelete)
	if e == sql.ErrNoRows {
		fail(w, 401, "Invalid deletion API key")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	if r.URL.Path == "/api/v2/deletion-requests" && r.Method == "POST" {
		a.createDeletionRequest(w, r, u)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v2/deletion-requests/") && r.Method == "GET" {
		rid := strings.TrimPrefix(r.URL.Path, "/api/v2/deletion-requests/")
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
		return
	}
	fail(w, 403, "This API key only permits submitting deletion requests and reading their own status")
}
