package app

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"zensync/internal/apple"
)

// Confidence describes metadata agreement, never the number of search results.
// Keep this separate from acquisition's exact provider/file validation.
type RecordingMatch struct {
	Confident bool     `json:"confident"`
	Reasons   []string `json:"reasons"`
	score     int
	title     bool
	primary   bool
	version   bool
	credits   bool
}

var (
	featureClause   = regexp.MustCompile(`(?i)\s*[\[(]\s*(?:feat\.?|ft\.?|featuring)\s+([^\])]+)[\])]`)
	featureSuffix   = regexp.MustCompile(`(?i)\s+(?:feat\.?|ft\.?|featuring)\s+(.+)$`)
	artistSeparator = regexp.MustCompile(`\s*[,;&]\s*|(?i:\s+(?:feat\.?|ft\.?|featuring)\s+)`)
	releaseSuffix   = regexp.MustCompile(`(?i)(?:\s+[-–—]\s*(?:single|ep)|\s*[\[(](?:single|ep)[\])])\s*$`)
	versionWords    = regexp.MustCompile(`\b(?:live|remix|remaster(?:ed)?|acoustic|instrumental|karaoke|radio edit|sped up|slowed|nightcore|demo|re recorded|clean|explicit)\b`)
)

// Common catalog transliteration, rather than artist-specific aliases. The
// original strings remain untouched for search, display and acquisition.
var russianLetters = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y",
	'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f",
	'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shh", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

func transliterated(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(value) {
		if replacement, ok := russianLetters[r]; ok {
			out.WriteString(replacement)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func recordingText(value string) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), r == '$', r == '?', r == '*':
			return r
		case r == '\'', r == '’', r == 'ʼ':
			return -1
		default:
			return ' '
		}
	}, transliterated(value))
	return strings.Join(strings.Fields(value), " ")
}

func artistNames(value string) []string {
	return artistSeparator.Split(value, -1)
}

func titleCredits(title string) (string, []string) {
	names := []string{}
	for _, match := range featureClause.FindAllStringSubmatch(title, -1) {
		names = append(names, artistNames(match[1])...)
	}
	title = featureClause.ReplaceAllString(title, "")
	if match := featureSuffix.FindStringSubmatch(title); match != nil {
		names = append(names, artistNames(match[1])...)
		title = title[:len(title)-len(match[0])]
	}
	return strings.TrimSpace(title), names
}

func albumText(album string) string {
	album, _ = titleCredits(releaseSuffix.ReplaceAllString(album, ""))
	return recordingText(album)
}

func nameSet(names []string) map[string]bool {
	set := map[string]bool{}
	for _, name := range names {
		if key := recordingText(name); key != "" {
			set[key] = true
		}
	}
	return set
}

func includesNames(available map[string]bool, expected []string) bool {
	for name := range nameSet(expected) {
		if !available[name] {
			return false
		}
	}
	return true
}

func unverifiedNames(available map[string]bool, expected []string) string {
	missing := []string{}
	for _, name := range expected {
		if key := recordingText(name); key != "" && !available[key] {
			missing = append(missing, name)
		}
	}
	return strings.Join(missing, ", ")
}

// Accept a partially masked word only when its visible letters and exact
// length agree. Never treat punctuation-only names ($$$, ?, ***) as wildcards.
func maskedTitleEqual(a, b string) bool {
	wordsA, wordsB := strings.Fields(a), strings.Fields(b)
	if len(wordsA) != len(wordsB) {
		return false
	}
	for i, word := range wordsA {
		for _, token := range []string{word, wordsB[i]} {
			if strings.ContainsRune(token, '*') && strings.Count(token, "*") > len([]rune(token))-2 {
				return false
			}
		}
	}
	left, right := []rune(a), []rune(b)
	if len(left) != len(right) {
		return false
	}
	masked, visible := false, 0
	for i, r := range left {
		other := right[i]
		if r == '*' || other == '*' {
			if !unicode.IsLetter(r) && r != '*' || !unicode.IsLetter(other) && other != '*' {
				return false
			}
			masked = true
		} else if r != other {
			return false
		} else if unicode.IsLetter(r) {
			visible++
		}
	}
	return masked && visible >= 3
}

func assessRecording(source apple.Song, track Track) RecordingMatch {
	match := RecordingMatch{Reasons: []string{}}
	if source.Missing {
		match.Reasons = append(match.Reasons, "Source metadata is incomplete")
	}
	title, featured := titleCredits(source.Title)
	otherTitle, otherFeatured := titleCredits(trackLabel(track))
	left, right := recordingText(title), recordingText(otherTitle)
	match.title = left != "" && (left == right || maskedTitleEqual(left, right))
	if match.title {
		match.score += 35
	} else {
		match.Reasons = append(match.Reasons, "Title differs")
	}
	versions := func(title, album string) string {
		words := versionWords.FindAllString(recordingText(title+" "+album), -1)
		sort.Strings(words)
		return strings.Join(words, " ")
	}
	match.version = versions(title, source.Album) == versions(otherTitle, track.Album.Title)
	if !match.version {
		match.score -= 25
		match.Reasons = append(match.Reasons, "Recording version differs")
	}
	expected := artistNames(source.Artist)
	primary := nameSet(artistNames(track.Artist.Name))
	for name := range nameSet(expected) {
		match.primary = match.primary || primary[name]
	}
	match.primary = match.primary && includesNames(nameSet(append(expected, featured...)), artistNames(track.Artist.Name))
	if match.primary {
		match.score += 20
	} else {
		match.Reasons = append(match.Reasons, "Artist differs")
	}
	availableNames := append(artistNames(track.Artist.Name), otherFeatured...)
	for _, contributor := range track.Contributors {
		availableNames = append(availableNames, artistNames(contributor.Name)...)
	}
	available := nameSet(availableNames)
	artists := match.primary && includesNames(available, expected)
	if artists {
		match.score += 10
	} else if match.primary {
		match.Reasons = append(match.Reasons, "Artist credit not verified: "+unverifiedNames(available, expected))
	}
	features := includesNames(available, featured) && includesNames(nameSet(append(expected, featured...)), otherFeatured)
	if features {
		match.score += 5
	} else {
		if missing := unverifiedNames(available, featured); missing != "" {
			match.Reasons = append(match.Reasons, "Featured artist credit not verified: "+missing)
		} else {
			match.Reasons = append(match.Reasons, "Featured artist credits differ")
		}
	}
	match.credits = artists && features
	album := albumText(source.Album) != "" && albumText(source.Album) == albumText(track.Album.Title)
	if album {
		match.score += 20
	} else {
		match.Reasons = append(match.Reasons, "Album differs")
	}
	tolerance := math.Min(3, math.Max(1, source.Duration*0.02))
	duration := source.Duration > 0 && track.Duration > 0 && math.Abs(source.Duration-track.Duration) <= tolerance
	if duration {
		match.score += 10
	} else {
		match.Reasons = append(match.Reasons, fmt.Sprintf("Duration differs (%s vs %s)", recordingDuration(source.Duration), recordingDuration(track.Duration)))
	}
	readable := track.Readable == nil || *track.Readable
	if !readable {
		match.score -= 25
		match.Reasons = append(match.Reasons, "Unavailable from Deezer")
	}
	match.Confident = !source.Missing && source.Artist != "" && match.title && match.version && match.credits && album && duration && readable
	return match
}

func recordingDuration(seconds float64) string {
	return fmt.Sprintf("%d:%02d", int(seconds)/60, int(seconds)%60)
}

func rankRecordings(source apple.Song, tracks []Track) {
	matches := make(map[int64]RecordingMatch, len(tracks))
	for _, track := range tracks {
		matches[track.ID] = assessRecording(source, track)
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		left, right := matches[tracks[i].ID], matches[tracks[j].ID]
		if left.Confident != right.Confident {
			return left.Confident
		}
		leftPlausible, rightPlausible := left.title && left.primary && left.version, right.title && right.primary && right.version
		if leftPlausible != rightPlausible {
			return leftPlausible
		}
		return left.score > right.score
	})
}

func confidentRecording(source apple.Song, tracks []Track) *Track {
	var selected *Track
	var explicit *bool
	for i := range tracks {
		if !assessRecording(source, tracks[i]).Confident {
			continue
		}
		if selected != nil {
			if selected.ISRC == "" || tracks[i].ISRC != selected.ISRC {
				return nil
			}
		}
		if tracks[i].ExplicitLyrics != nil {
			if explicit != nil && *explicit != *tracks[i].ExplicitLyrics {
				return nil
			}
			explicit = tracks[i].ExplicitLyrics
		}
		if selected == nil {
			selected = &tracks[i]
		}
	}
	return selected
}

func importSearchQueries(source apple.Song) []string {
	queries := []string{}
	add := func(query string) {
		query = strings.TrimSpace(query)
		for _, prior := range queries {
			if prior == query {
				return
			}
		}
		if query != "" && len(queries) < 5 {
			queries = append(queries, query)
		}
	}
	add(source.Artist + " " + source.Title)
	title, _ := titleCredits(source.Title)
	primary := artistNames(source.Artist)[0]
	add(primary + " " + source.Title)
	add(primary + " " + title)
	if strings.ContainsFunc(title, func(r rune) bool { return unicode.In(r, unicode.Cyrillic) }) && len(primary) >= 2 && len(primary) <= 5 {
		acronym := strings.Map(func(r rune) rune {
			return map[rune]rune{'A': 'а', 'B': 'б', 'C': 'с', 'D': 'д', 'E': 'е', 'F': 'ф', 'G': 'г', 'H': 'х', 'I': 'и', 'J': 'ж', 'K': 'к', 'L': 'л', 'M': 'м', 'N': 'н', 'O': 'о', 'P': 'п', 'R': 'р', 'S': 'с', 'T': 'т', 'U': 'у', 'V': 'в', 'Y': 'й', 'Z': 'з'}[r]
		}, primary)
		if !strings.ContainsRune(acronym, 0) {
			add(acronym + " " + title)
		}
	}
	spaced := strings.NewReplacer("-", " ", "–", " ", "—", " ").Replace(title)
	add(primary + " " + spaced)
	if spaced != title {
		first := strings.Fields(spaced)
		if len(first) > 1 && len([]rune(first[0])) >= 4 {
			add(primary + " " + first[0])
		}
	}
	if transliterated(title) != strings.ToLower(title) || transliterated(primary) != strings.ToLower(primary) {
		add(transliterated(primary + " " + title))
	}
	return queries
}

// Fallbacks are bounded and stop at the first plausible recording. Provider
// lookups for collaborator credits are limited to two plausible candidates.
// A user's edited query is always searched literally.
func (a *App) searchImportRecordings(ctx context.Context, source apple.Song, query string) ([]Track, error) {
	queries := []string{query}
	if strings.TrimSpace(query) == strings.TrimSpace(source.Artist+" "+source.Title) {
		queries = importSearchQueries(source)
	}
	tracks := []Track{}
	seen := map[int64]bool{}
	verified := 0
	for _, query := range queries {
		results, err := a.searchDeezer(ctx, query)
		if err != nil {
			return tracks, err
		}
		for _, track := range results {
			if track.ID <= 0 || track.Title == "" || seen[track.ID] {
				continue
			}
			seen[track.ID] = true
			tracks = append(tracks, track)
		}
		rankRecordings(source, tracks)
		plausible := false
		for i := range tracks {
			match := assessRecording(source, tracks[i])
			if match.title && match.primary && match.version {
				plausible = true
				if !match.credits && len(tracks[i].Contributors) == 0 && verified < 2 {
					verified++
					if full, err := a.track(ctx, fmt.Sprint(tracks[i].ID)); err == nil {
						tracks[i] = full
					}
				}
			}
		}
		if plausible {
			break
		}
	}
	rankRecordings(source, tracks)
	return tracks, nil
}
