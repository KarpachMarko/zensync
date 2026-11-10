package app

import (
	"regexp"
	"sort"
	"strings"
)

// These are review hints only. Delimiters do not establish artist identity:
// legal names, bands containing punctuation and collaborations need review.
var creditSeparator = regexp.MustCompile(`(?i),\s+|\s+[x×]\s+|\s+/\s+|\s+(?:feat\.?|ft\.?)\s+|;\s*`)

type creditIssue struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Parts        []string `json:"parts"`
	KnownArtists []string `json:"knownArtists"`
	Tracks       []Song   `json:"tracks"`
}

func songCredits(song Song, album bool) []ArtistCredit {
	if album {
		if len(song.AlbumArtists) > 0 {
			return song.AlbumArtists
		}
		if song.AlbumArtist != "" {
			return []ArtistCredit{{Name: song.AlbumArtist}}
		}
		return nil
	}
	if len(song.Artists) > 0 {
		return song.Artists
	}
	return []ArtistCredit{{ID: song.ArtistID, Name: song.Artist}}
}

func metadataCreditIssues(songs []Song) []creditIssue {
	known := map[string]string{}
	for _, song := range songs {
		for _, album := range []bool{false, true} {
			for _, credit := range songCredits(song, album) {
				if len(creditSeparator.Split(credit.Name, -1)) == 1 && metadataKey(credit.Name) != "" {
					key := metadataKey(credit.Name)
					if previous, ok := known[key]; !ok || credit.Name < previous {
						known[key] = credit.Name
					}
				}
			}
		}
	}
	issues := map[string]*creditIssue{}
	seenTracks := map[string]map[string]bool{}
	for _, song := range songs {
		for _, album := range []bool{false, true} {
			role := "artist"
			if album {
				role = "album artist"
			}
			// Read individual OpenSubsonic credits, never split a display string
			// that is already represented by multiple separate artist identities.
			for _, credit := range songCredits(song, album) {
				parts := creditSeparator.Split(credit.Name, -1)
				if len(parts) < 2 {
					continue
				}
				matches := []string{}
				for i, part := range parts {
					parts[i] = strings.TrimSpace(part)
					if name, ok := known[metadataKey(parts[i])]; ok {
						matches = append(matches, name)
					}
				}
				// A known standalone artist supplies evidence; punctuation alone
				// would flag names like Earth, Wind & Fire.
				if len(matches) == 0 {
					continue
				}
				key := role + ":" + credit.ID + ":" + credit.Name
				issue := issues[key]
				if issue == nil {
					issue = &creditIssue{ID: credit.ID, Name: credit.Name, Role: role, Parts: parts, KnownArtists: matches}
					issues[key] = issue
					seenTracks[key] = map[string]bool{}
				}
				clean := song
				clean.Path = ""
				if !seenTracks[key][clean.ID] {
					issue.Tracks = append(issue.Tracks, clean)
					seenTracks[key][clean.ID] = true
				}
			}
		}
	}
	keys := []string{}
	for key := range issues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []creditIssue{}
	for _, key := range keys {
		issue := issues[key]
		sort.Slice(issue.Tracks, func(i, j int) bool { return issue.Tracks[i].ID < issue.Tracks[j].ID })
		result = append(result, *issue)
	}
	return result
}
