package app

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

type creditRepairPlan struct {
	Role    string   `json:"role"`
	Name    string   `json:"name"`
	Artists []string `json:"artists"`
}

func (a *App) separateArtistCredits(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Source string `json:"source"`
		creditRepairPlan
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Role != "artist" && in.Role != "album artist") || in.Name == "" || len(in.Name) > 500 || len(in.Artists) < 2 || len(in.Artists) > 20 {
		fail(w, 400, "Choose at least two separate artist names")
		return
	}
	for i := range in.Artists {
		in.Artists[i] = strings.TrimSpace(in.Artists[i])
		if in.Artists[i] == "" || len(in.Artists[i]) > 500 {
			fail(w, 400, "Each artist needs a name of at most 500 characters")
			return
		}
		for _, previous := range in.Artists[:i] {
			if strings.EqualFold(previous, in.Artists[i]) {
				fail(w, 400, "List each artist once")
				return
			}
		}
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.navIndexMu.Lock()
	delete(a.navIndexes, digest(mustJSON(c)))
	a.navIndexMu.Unlock()
	index, err := a.indexedNavSongs(r.Context(), c)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if index.limited {
		fail(w, 409, "Separating credits requires a complete library scan")
		return
	}
	var issue *creditIssue
	for _, candidate := range metadataCreditIssues(index.songs) {
		if candidate.ID == in.Source && candidate.Role == in.Role && candidate.Name == in.Name {
			copy := candidate
			issue = &copy
			break
		}
	}
	if issue == nil {
		fail(w, 409, "These credits changed; refresh the review before applying")
		return
	}
	changes := []metadataChange{}
	for _, song := range issue.Tracks {
		// A credit-only correction must not apply saved edits from a failed
		// title/lyrics/artwork job.
		edit := MetadataEdit{Title: song.Title, Artist: song.Artist, Album: song.Album, AlbumArtist: song.AlbumArtist}
		plan := in.creditRepairPlan
		copy := song
		changes = append(changes, metadataChange{Song: song, Edit: edit, MergeKind: "credits", Target: &copy, CreditRepair: &plan})
	}
	if len(changes) == 0 {
		a.internal(w, errors.New("Credit review has no affected songs"))
		return
	}
	jid, err := a.queueMetadata(changes, u)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	send(w, 202, map[string]any{"id": jid, "songs": len(changes)})
}

// Credit-only changes start from an exact copy, avoiding a remux that could
// flatten unrelated multi-valued metadata such as composer credits.
func prepareCreditRepairFile(original, output string) error {
	input, err := os.Open(original)
	if err != nil {
		return err
	}
	defer input.Close()
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, input)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// Queue one job and one write per song, including songs with both roles to fix.
func (a *App) separateKnownArtistCredits(w http.ResponseWriter, r *http.Request, u User) {
	var options struct {
		IncludeUnknown bool `json:"includeUnknown"`
	}
	if r.ContentLength != 0 && !decode(w, r, &options) {
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	a.navIndexMu.Lock()
	delete(a.navIndexes, digest(mustJSON(c)))
	a.navIndexMu.Unlock()
	index, err := a.indexedNavSongs(r.Context(), c)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if index.limited {
		fail(w, 409, "Separating credits requires a complete library scan")
		return
	}
	changes := []metadataChange{}
	positions := map[string]int{}
	count := 0
	for _, issue := range metadataCreditIssues(index.songs) {
		names := []string{}
		seen := map[string]bool{}
		for _, part := range issue.Parts {
			match := ""
			for _, known := range issue.KnownArtists {
				// Exact case-insensitive matches only: punctuation can distinguish identities.
				if strings.EqualFold(strings.TrimSpace(part), known) {
					match = known
					break
				}
			}
			if match == "" && options.IncludeUnknown {
				match = strings.TrimSpace(part)
			}
			if match == "" || len(match) > 500 || seen[strings.ToLower(match)] {
				names = nil
				break
			}
			seen[strings.ToLower(match)] = true
			names = append(names, match)
		}
		if len(names) < 2 || len(names) > 20 {
			continue
		}
		count++
		for _, song := range issue.Tracks {
			pos, ok := positions[song.ID]
			if !ok {
				pos = len(changes)
				positions[song.ID] = pos
				copy := song
				changes = append(changes, metadataChange{Song: song, Edit: MetadataEdit{Title: song.Title, Artist: song.Artist, Album: song.Album, AlbumArtist: song.AlbumArtist}, MergeKind: "credits", Target: &copy})
			}
			changes[pos].CreditRepairs = append(changes[pos].CreditRepairs, creditRepairPlan{Role: issue.Role, Name: issue.Name, Artists: names})
		}
	}
	if len(changes) == 0 {
		if options.IncludeUnknown {
			fail(w, 409, "No combined credits have valid separate artist names; review the remaining names")
		} else {
			fail(w, 409, "No combined credits have all artists matched exactly; review the remaining names")
		}
		return
	}
	jid, err := a.queueMetadata(changes, u)
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	send(w, 202, map[string]any{"id": jid, "songs": len(changes), "issues": count})
}
