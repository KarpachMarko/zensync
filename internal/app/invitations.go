package app

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
	"time"
)

type Invitation struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Expires  int64  `json:"expires"`
	Created  string `json:"created"`
}

func (a *App) invitations(w http.ResponseWriter, r *http.Request, actor User) {
	if actor.Role != "admin" {
		fail(w, 403, "Administrator required")
		return
	}
	if r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/v2/invitations/") {
		iid := strings.TrimPrefix(r.URL.Path, "/api/v2/invitations/")
		tx, e := a.DB.Begin()
		if e != nil {
			a.internal(w, e)
			return
		}
		defer tx.Rollback()
		result, e := tx.Exec("DELETE FROM invitations WHERE id=?", iid)
		if e != nil {
			a.internal(w, e)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			fail(w, 404, "Invitation unavailable")
			return
		}
		if e = audit(tx, actor.ID, actor.ID, "invitation.revoked", iid); e == nil {
			e = tx.Commit()
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.URL.Path != "/api/v2/invitations" {
		fail(w, 404, "Invitation unavailable")
		return
	}
	if r.Method == "GET" {
		rows, e := a.DB.Query("SELECT id,username,expires,created FROM invitations WHERE expires>? ORDER BY created,id", time.Now().Unix())
		if e != nil {
			a.internal(w, e)
			return
		}
		defer rows.Close()
		out := []Invitation{}
		for rows.Next() {
			var invite Invitation
			if e = rows.Scan(&invite.ID, &invite.Username, &invite.Expires, &invite.Created); e != nil {
				a.internal(w, e)
				return
			}
			out = append(out, invite)
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
		Username string `json:"username"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || len(in.Username) > 80 {
		fail(w, 400, "Username must be 1–80 bytes")
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("DELETE FROM invitations WHERE expires<=?", time.Now().Unix()); e != nil {
		a.internal(w, e)
		return
	}
	var count int
	if e = tx.QueryRow("SELECT (SELECT count(*) FROM users WHERE username=?)+(SELECT count(*) FROM invitations WHERE username=?)", in.Username, in.Username).Scan(&count); e != nil {
		a.internal(w, e)
		return
	}
	if count > 0 {
		fail(w, 409, "Username already exists or has a pending invitation")
		return
	}
	token, iid := id(), id()
	expires := time.Now().Add(7 * 24 * time.Hour).Unix()
	_, e = tx.Exec("INSERT INTO invitations(id,token_hash,username,inviter,expires) VALUES(?,?,?,?,?)", iid, digest(token), in.Username, actor.ID, expires)
	if e == nil {
		e = audit(tx, actor.ID, actor.ID, "invitation.created", iid)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 201, map[string]any{"id": iid, "token": token, "username": in.Username, "expires": expires})
}

func (a *App) previewInvitation(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if len(token) != 64 {
		fail(w, 404, "Invitation unavailable")
		return
	}
	var username string
	var expires int64
	e := a.DB.QueryRow("SELECT username,expires FROM invitations WHERE token_hash=? AND expires>?", digest(token), time.Now().Unix()).Scan(&username, &expires)
	if e == sql.ErrNoRows {
		fail(w, 404, "Invitation unavailable")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 200, map[string]any{"username": username, "expires": expires})
}

func (a *App) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	if !a.allowLogin(w) {
		return
	}
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Token) != 64 || len(in.Password) < 12 || len(in.Password) > 72 {
		fail(w, 400, "A valid invitation and a 12–72 byte password are required")
		return
	}
	// Avoid expensive hashing for unknown tokens, then check again inside the consumption transaction.
	var exists int
	if e := a.DB.QueryRow("SELECT count(*) FROM invitations WHERE token_hash=? AND expires>?", digest(in.Token), time.Now().Unix()).Scan(&exists); e != nil {
		a.internal(w, e)
		return
	}
	if exists == 0 {
		fail(w, 404, "Invitation unavailable")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		a.internal(w, e)
		return
	}
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var iid, username, inviter string
	e = tx.QueryRow("SELECT id,username,inviter FROM invitations WHERE token_hash=? AND expires>?", digest(in.Token), time.Now().Unix()).Scan(&iid, &username, &inviter)
	if e == sql.ErrNoRows {
		fail(w, 404, "Invitation unavailable")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	var count int
	if e = tx.QueryRow("SELECT count(*) FROM users WHERE username=?", username).Scan(&count); e != nil {
		a.internal(w, e)
		return
	}
	if count > 0 {
		fail(w, 409, "Username already exists")
		return
	}
	uid := id()
	_, e = tx.Exec("INSERT INTO users(id,username,password,role) VALUES(?,?,?,'member')", uid, username, hash)
	if e == nil {
		_, e = tx.Exec("DELETE FROM invitations WHERE id=?", iid)
	}
	if e == nil {
		e = audit(tx, uid, inviter, "invitation.accepted", iid)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 201, map[string]string{"username": username})
}
