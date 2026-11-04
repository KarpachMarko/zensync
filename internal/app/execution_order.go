package app

import "strings"

// Rank active work ahead of failures, queued work and history before applying
// API limits. The identifiers passed here are internal SQL column names.
func executionOrderSQL(state string) string {
	return strings.ReplaceAll(`CASE
 WHEN $state='downloading' THEN 0
 WHEN $state IN ('awaiting_scan','scanning') THEN 1
 WHEN $state='importing' THEN 2
 WHEN $state IN ('blocked','failed','deferred') THEN 4
 WHEN $state='awaiting_approval' THEN 4.5
 WHEN $state='queued' THEN 5
 WHEN $state IN ('available','completed','cancelled','declined','withdrawn','superseded') THEN 6
 ELSE 3 END`, "$state", state)
}
