package app

import (
	"database/sql"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
)

func (a *App) users(w http.ResponseWriter) {
	rows, e := a.DB.Query("SELECT id,username,role,direct,quality FROM users ORDER BY username")
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var q string
		if e = rows.Scan(&u.ID, &u.Username, &u.Role, &u.Direct, &q); e != nil {
			a.internal(w, e)
			return
		}
		u.Quality = json.RawMessage(q)
		out = append(out, u)
	}
	if e = rows.Err(); e != nil {
		a.internal(w, e)
		return
	}
	send(w, 200, out)
}
func (a *App) createUser(w http.ResponseWriter, r *http.Request, actor User) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Direct   bool   `json:"direct"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if len(in.Username) < 1 || len(in.Username) > 80 || len(in.Password) < 12 || len(in.Password) > 72 {
		fail(w, 400, "Username required; password must be 12–72 bytes")
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
	var n int
	if e = tx.QueryRow("SELECT count(*) FROM users WHERE username=?", in.Username).Scan(&n); e != nil {
		a.internal(w, e)
		return
	}
	if n > 0 {
		fail(w, 409, "Username already exists")
		return
	}
	uid := id()
	_, e = tx.Exec("INSERT INTO users(id,username,password,role,direct) VALUES(?,?,?,'member',?)", uid, in.Username, hash, in.Direct)
	if e == nil {
		e = audit(tx, uid, actor.ID, "user.created", uid)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 201, map[string]string{"id": uid})
}
func (a *App) quality(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Quality []string `json:"quality"`
	}
	if !decode(w, r, &in) {
		return
	}
	seen := map[string]bool{}
	if len(in.Quality) < 1 || len(in.Quality) > 3 {
		fail(w, 400, "Select one to three qualities")
		return
	}
	for _, q := range in.Quality {
		if seen[q] || (q != "lossless" && q != "320" && q != "128") {
			fail(w, 400, "Qualities must be unique: lossless, 320, 128")
			return
		}
		seen[q] = true
	}
	b, _ := json.Marshal(in.Quality)
	_, e := a.DB.Exec("UPDATE users SET quality=? WHERE id=?", string(b), u.ID)
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 200, in)
}

// updateUser changes the acquisition permission used by both new requests and workers.
func (a *App) updateUser(w http.ResponseWriter, r *http.Request, actor User) {
	var in struct {
		Direct *bool `json:"direct"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Direct == nil {
		fail(w, 400, "Direct permission is required")
		return
	}
	uid := strings.TrimPrefix(r.URL.Path, "/api/v2/users/")
	tx, e := a.DB.Begin()
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback()
	var role string
	e = tx.QueryRow("SELECT role FROM users WHERE id=?", uid).Scan(&role)
	if e == sql.ErrNoRows {
		fail(w, 404, "User not found")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	if role == "admin" {
		fail(w, 400, "Administrators already have full permissions")
		return
	}
	_, e = tx.Exec("UPDATE users SET direct=? WHERE id=?", *in.Direct, uid)
	if e == nil {
		e = audit(tx, uid, actor.ID, "user.permissions_updated", uid)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}
