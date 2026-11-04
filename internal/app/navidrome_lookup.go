package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const navIndexLimit = 50000
const navVerificationLimit = 200

type navSongIndex struct {
	songs   []Song
	fetched time.Time
	limited bool
}

// The account's metadata index is shared by acquisition workers. In particular,
// an unsuccessful lookup in a large import must not browse every album again
// for every song. Credentials are part of the key, never logged or stored here.
func (a *App) indexedNavSongs(ctx context.Context, c Credentials) (navSongIndex, error) {
	a.navIndexMu.Lock()
	defer a.navIndexMu.Unlock()
	key := digest(mustJSON(c))
	if index, ok := a.navIndexes[key]; ok && time.Since(index.fetched) < 30*time.Second {
		return index, nil
	}
	index := navSongIndex{}
	for offset := 0; offset < navIndexLimit; offset += 100 {
		if err := ctx.Err(); err != nil {
			return index, err
		}
		var count int
		songs, err := a.navSearch(ctx, c, "", offset, &count)
		if err != nil {
			return index, err
		}
		index.songs = append(index.songs, songs...)
		if count < 100 {
			break
		}
		index.limited = offset+100 == navIndexLimit
	}
	index.fetched = time.Now()
	if a.navIndexes == nil {
		a.navIndexes = map[string]navSongIndex{}
	}
	if _, exists := a.navIndexes[key]; !exists && len(a.navIndexes) >= 2 {
		clear(a.navIndexes)
	}
	a.navIndexes[key] = index
	return index, nil
}

// Keep the source script: transliteration would not match Navidrome's index.
func navTitleWords(title string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, title)), " ")
}

func placedSearchQueries(t Track) []string {
	base, _ := titleCredits(t.Title)
	words := strings.Fields(navTitleWords(base))
	qualified := ""
	if artist := strings.TrimSpace(artistNames(t.Artist.Name)[0]); artist != "" {
		qualified = artist + " " + t.Title
	}
	queries := []string{}
	seen := map[string]bool{}
	for _, query := range []string{t.Title, qualified, strings.Join(words, " "), strings.Join(words[:min(4, len(words))], " "), strings.Join(words[:min(2, len(words))], " ")} {
		if query = strings.TrimSpace(query); query != "" && !seen[strings.ToLower(query)] {
			seen[strings.ToLower(query)] = true
			queries = append(queries, query)
		}
	}
	return queries
}

type placedLookupReport struct {
	checked      int
	librarySongs int
	limited      bool
}

func (r placedLookupReport) reason() string {
	if r.limited {
		return fmt.Sprintf("Navidrome lookup reached its safety limit after checking %d original files", r.checked)
	}
	if r.checked > 0 {
		return fmt.Sprintf("None of %d indexed candidates served the retained file's original bytes; their files are missing or have different content", r.checked)
	}
	if r.librarySongs == 0 {
		return "Navidrome returned no indexed songs to the service account; check its library access and the shared music mount"
	}
	return "Navidrome has indexed songs, but no candidate for this file; check scan progress, the shared music mount and the service account's library access"
}

func placedCandidateScore(s Song, t Track, rel string, size int64) int {
	score := 0
	path := strings.ReplaceAll(s.Path, "\\", "/")
	if path == rel || strings.HasSuffix(path, "/"+rel) {
		score += 128
	}
	if s.Size == size {
		score += 64
	}
	title, candidate := navTitleWords(t.Title), navTitleWords(s.Title)
	if title != "" && candidate != "" && (strings.HasPrefix(title, candidate) || strings.HasPrefix(candidate, title)) {
		score += 8
		if title == candidate {
			score += 8
		}
		primary := navTitleWords(artistNames(t.Artist.Name)[0])
		if primary != "" && strings.Contains(" "+navTitleWords(s.Artist)+" ", " "+primary+" ") {
			score += 64
		}
		if album := navTitleWords(t.Album.Title); album != "" && album == navTitleWords(s.Album) {
			score += 16
		}
	}
	// Older Subsonic responses can omit size. Duration is only a discovery hint;
	// it can never establish that this is the original file.
	if s.Size <= 0 && t.Duration > 0 && s.Duration > 0 && math.Abs(t.Duration-s.Duration) <= 3 {
		score++
	}
	return score
}

func (a *App) findPlacedWithReport(ctx context.Context, c Credentials, rel, metadata string, unique bool) (string, placedLookupReport, error) {
	report := placedLookupReport{}
	var t Track
	if json.Unmarshal([]byte(metadata), &t) != nil || !safeRelativePath(rel) {
		return "", report, errors.New("Invalid placed asset metadata or path")
	}
	local := filepath.Join(a.musicDir, filepath.FromSlash(rel))
	info, err := os.Stat(local)
	if err != nil {
		return "", report, errors.New("Validated local file is missing")
	}
	checksum, err := hashFile(local)
	if err != nil {
		return "", report, err
	}
	checked := map[string]bool{}
	matches := []string{}
	verify := func(songs []Song) error {
		scores := make(map[string]int, len(songs))
		for _, song := range songs {
			scores[song.ID] = placedCandidateScore(song, t, rel, info.Size())
		}
		sort.SliceStable(songs, func(i, j int) bool {
			if scores[songs[i].ID] == scores[songs[j].ID] {
				return songs[i].ID < songs[j].ID
			}
			return scores[songs[i].ID] > scores[songs[j].ID]
		})
		for _, song := range songs {
			if song.ID == "" || checked[song.ID] {
				continue
			}
			if report.checked == navVerificationLimit {
				report.limited = true
				return nil
			}
			checked[song.ID] = true
			report.checked++
			same, err := a.sameRemoteAudioHash(ctx, c, song.ID, info.Size(), checksum)
			if err != nil {
				return err
			}
			if same {
				slog.Info("Navidrome asset verified", "song_id", song.ID, "file_path", rel, "reported_path", song.Path, "metadata_matches", trackMatches(t, song), "indexed_size", song.Size, "local_size", info.Size())
				matches = append(matches, song.ID)
				if !unique || len(matches) > 1 {
					return nil
				}
			}
		}
		return nil
	}
	result := func(err error) (string, placedLookupReport, error) {
		slog.Info("Navidrome original-file lookup", "file_path", rel, "checked", report.checked, "indexed_songs", report.librarySongs, "limited", report.limited, "matches", len(matches))
		if err != nil {
			return "", report, err
		}
		if len(matches) > 1 {
			return "", report, errors.New("Multiple Navidrome songs contain the same audio; relocation cannot safely identify the original file")
		}
		if unique && report.limited {
			return "", report, errors.New("Navidrome lookup reached its safety limit; cannot verify a unique song for relocation")
		}
		if len(matches) == 1 {
			return matches[0], report, nil
		}
		return "", report, nil
	}
	// Small complete search results keep the common case cheap. Broad results
	// must not consume the byte-verification budget page by page: the actual
	// file may be thousands of rows later. Rank them with the full metadata
	// index instead. Reserve at least 190 checks for that global ranking.
	byID := map[string]Song{}
	for _, query := range placedSearchQueries(t) {
		var count int
		songs, err := a.navSearch(ctx, c, query, 0, &count)
		if err != nil {
			return result(err)
		}
		for _, song := range songs {
			byID[song.ID] = song
		}
		if count < 100 && report.checked+len(songs) <= 10 {
			if err := verify(songs); err != nil || len(matches) > 1 || len(matches) == 1 && !unique {
				return result(err)
			}
		}
	}
	// search3 is a text search, not a physical-path lookup. A short title made
	// of symbols, changed tags or a tokenizer difference can defeat every query.
	index, err := a.indexedNavSongs(ctx, c)
	if err != nil {
		return result(err)
	}
	report.librarySongs = len(index.songs)
	report.limited = report.limited || index.limited
	for _, song := range index.songs {
		if placedCandidateScore(song, t, rel, info.Size()) > 0 {
			if _, exists := byID[song.ID]; !exists {
				byID[song.ID] = song
			}
		}
	}
	candidates := make([]Song, 0, len(byID))
	for _, song := range byID {
		candidates = append(candidates, song)
	}
	return result(verify(candidates))
}
