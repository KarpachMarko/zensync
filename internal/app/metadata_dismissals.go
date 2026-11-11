package app

import (
	"errors"
	"net/http"
	"sort"
	"strings"
)

var errCreditRepairDismissed = errors.New("credit repair dismissed before publication")

func metadataCreditDismissalKey(name string) string {
	// Keep punctuation: it can distinguish a literal band name from a person.
	return "credit:" + digest(strings.ToLower(strings.TrimSpace(name)))
}
func metadataGroupDismissalKey(kind string, ids []string) string {
	ids = append([]string{}, ids...)
	sort.Strings(ids)
	return kind + ":" + digest(mustJSON(ids))
}
func metadataDismissalServer(c Credentials) string { return digest(strings.TrimRight(c.URL, "/")) }
func (a *App) metadataDismissals(c Credentials) (map[string]bool, error) {
	rows, err := a.DB.Query("SELECT rule_key FROM metadata_dismissals WHERE server_key=?", metadataDismissalServer(c))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	return result, rows.Err()
}
func reviewedCreditIssues(songs []Song, dismissed map[string]bool) []creditIssue {
	result := []creditIssue{}
	for _, issue := range metadataCreditIssues(songs) {
		if !dismissed[metadataCreditDismissalKey(issue.Name)] {
			result = append(result, issue)
		}
	}
	return result
}
func reviewedDuplicateGroups(songs []Song, dismissed map[string]bool) []duplicateGroup {
	result := []duplicateGroup{}
	for _, group := range metadataDuplicates(songs) {
		ids := []string{}
		for _, member := range group.Members {
			ids = append(ids, member.ID)
		}
		if !dismissed[metadataGroupDismissalKey(group.Kind, ids)] {
			result = append(result, group)
		}
	}
	return result
}
func (a *App) dismissMetadataSuggestion(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Kind    string   `json:"kind"`
		Name    string   `json:"name"`
		IDs     []string `json:"ids"`
		Restore bool     `json:"restore"`
	}
	if !decode(w, r, &in) {
		return
	}
	key := ""
	if in.Kind == "credit" {
		if strings.TrimSpace(in.Name) == "" || len(in.Name) > 500 {
			fail(w, 400, "Choose an artist name")
			return
		}
		key = metadataCreditDismissalKey(in.Name)
	} else {
		if (in.Kind != "song" && in.Kind != "album" && in.Kind != "artist") || len(in.IDs) < 2 || len(in.IDs) > 50000 {
			fail(w, 400, "Choose a duplicate candidate group")
			return
		}
		seen := map[string]bool{}
		for _, id := range in.IDs {
			if id == "" || len(id) > 200 || seen[id] {
				fail(w, 400, "Choose distinct library IDs")
				return
			}
			seen[id] = true
		}
		key = metadataGroupDismissalKey(in.Kind, in.IDs)
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.metadataDecisionMu.Lock()
	defer a.metadataDecisionMu.Unlock()
	tx, err := a.DB.Begin()
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback()
	if in.Restore {
		_, err = tx.Exec("DELETE FROM metadata_dismissals WHERE server_key=? AND rule_key=?", metadataDismissalServer(c), key)
	} else {
		_, err = tx.Exec("INSERT OR IGNORE INTO metadata_dismissals(server_key,rule_key,actor) VALUES(?,?,?)", metadataDismissalServer(c), key, u.ID)
	}
	if err == nil {
		err = audit(tx, u.ID, u.ID, "metadata.dismissal", key)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	send(w, 200, map[string]bool{"ok": true})
}

// A dismissal can be made after a batch was queued. Re-check each song so
// marking a literal artist name as indivisible also protects pending work.
func (a *App) filterDismissedCreditRepairs(c Credentials, change metadataChange) (metadataChange, bool, error) {
	dismissed, err := a.metadataDismissals(c)
	if err != nil {
		return change, false, err
	}
	plans := change.CreditRepairs
	if len(plans) == 0 && change.CreditRepair != nil {
		plans = []creditRepairPlan{*change.CreditRepair}
	}
	if len(plans) == 0 {
		return change, false, errors.New("Credit repair has no artist plan")
	}
	retained := []creditRepairPlan{}
	for _, plan := range plans {
		if !dismissed[metadataCreditDismissalKey(plan.Name)] {
			retained = append(retained, plan)
		}
	}
	change.CreditRepairs = retained
	change.CreditRepair = nil
	return change, len(retained) == 0, nil
}
