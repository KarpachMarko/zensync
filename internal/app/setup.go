package app

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
)

var errConfigured = errors.New("Administrator already configured")

// The empty-database check and insert share one transaction. Setup cannot create
// a second administrator, including concurrent requests or container restarts.
func (a *App) createAdmin(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 80 || len(password) < 12 || len(password) > 72 {
		return errors.New("Username required; password must be 12–72 bytes")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := a.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var n int
	if e = tx.QueryRow("SELECT count(*) FROM users").Scan(&n); e != nil {
		return e
	}
	if n != 0 {
		return errConfigured
	}
	uid := id()
	if _, e = tx.Exec("INSERT INTO users(id,username,password,role) VALUES(?,?,?,'admin')", uid, username, hash); e != nil {
		return e
	}
	if e = audit(tx, uid, uid, "setup.completed", uid); e != nil {
		return e
	}
	return tx.Commit()
}
func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	var n int
	if e := a.DB.QueryRow("SELECT count(*) FROM users").Scan(&n); e != nil {
		a.internal(w, e)
		return
	}
	if r.Method == "GET" {
		send(w, 200, map[string]bool{"required": n == 0})
		return
	}
	if r.Method != "POST" {
		fail(w, 405, "Method not allowed")
		return
	}
	if n > 0 {
		fail(w, 409, errConfigured.Error())
		return
	}
	if !a.allowLogin(w) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e := a.createAdmin(in.Username, in.Password); e != nil {
		status := 400
		if errors.Is(e, errConfigured) {
			status = 409
		}
		fail(w, status, e.Error())
		return
	}
	send(w, 201, map[string]bool{"ok": true})
}
