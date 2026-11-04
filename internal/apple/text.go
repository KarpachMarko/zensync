package apple

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Parse accepts either a library/playlist XML export or a text playlist export.
func Parse(data []byte) (Snapshot, error) { return ParseFile(data, "") }

// ParseFile uses the filename as the playlist name for text exports, which do
// not contain a playlist name or persistent identifiers.
func ParseFile(data []byte, filename string) (Snapshot, error) {
	if len(data) > MaxBytes {
		return Snapshot{}, errors.New("export exceeds 32 MiB")
	}
	text, err := decodeExport(data)
	if err != nil {
		return Snapshot{}, err
	}
	if strings.HasPrefix(strings.TrimSpace(text), "<") {
		return parseXML([]byte(text))
	}
	return parseText(text, filename)
}

func decodeExport(data []byte) (string, error) {
	var order binary.ByteOrder
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		order = binary.LittleEndian
	}
	if bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		order = binary.BigEndian
	}
	if order != nil {
		data = data[2:]
		if len(data)%2 != 0 {
			return "", errors.New("incomplete UTF-16 export")
		}
		units := make([]uint16, len(data)/2)
		for i := range units {
			units[i] = order.Uint16(data[i*2:])
		}
		for i := 0; i < len(units); i++ {
			u := units[i]
			if u >= 0xd800 && u <= 0xdbff {
				if i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
					return "", errors.New("invalid UTF-16 export")
				}
				i++
			} else if u >= 0xdc00 && u <= 0xdfff {
				return "", errors.New("invalid UTF-16 export")
			}
		}
		return string(utf16.Decode(units)), nil
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) {
		return "", errors.New("export must be UTF-8 or UTF-16 text")
	}
	return string(data), nil
}

func parseText(text, filename string) (Snapshot, error) {
	out := Snapshot{Songs: []Song{}, Playlists: []Playlist{}, Warnings: []string{}}
	// Music's text format uses literal tabs and CR line endings. Quotes in
	// titles are ordinary characters, not CSV quoting.
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(text, "\n")
	headers := strings.Split(lines[0], "\t")
	columns := map[string]int{}
	for i, h := range headers {
		h = strings.TrimSpace(h)
		if h == "" {
			return out, errors.New("empty column in playlist header")
		}
		if _, exists := columns[h]; exists {
			return out, fmt.Errorf("duplicate playlist column %q", h)
		}
		columns[h] = i
		headers[i] = h
	}
	for _, h := range []string{"Name", "Artist", "Album", "Time"} {
		if _, ok := columns[h]; !ok {
			return out, errors.New("expected an Apple Music XML export or tab-separated playlist with Name, Artist, Album and Time columns")
		}
	}
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if strings.TrimSpace(name) == "" || name == "." {
		name = "Imported playlist"
	}
	pl := Playlist{Name: name, SourceID: "text-playlist", Entries: []string{}}
	count := 0
	for i, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		count++
		if count > 20000 {
			return out, errors.New("initial import limit is 20,000 source tracks; export a smaller selection")
		}
		fields := strings.Split(line, "\t")
		for _, h := range []string{"Name", "Artist", "Album", "Time"} {
			if len(fields) <= columns[h] {
				return out, fmt.Errorf("playlist row %d is missing columns", i+2)
			}
		}
		if len(fields) > len(headers) {
			return out, fmt.Errorf("playlist row %d has too many columns", i+2)
		}
		original := map[string]any{}
		for j, value := range fields {
			if len(value) > 1<<20 {
				return out, errors.New("text field exceeds 1 MiB")
			}
			original[headers[j]] = value
		}
		get := func(key string) string { return str(original[key]) }
		if strings.TrimSpace(get("Name")) == "" {
			return out, fmt.Errorf("playlist row %d has no title", i+2)
		}
		duration := 0.0
		if value := get("Time"); value != "" {
			seconds, err := strconv.ParseUint(value, 10, 32)
			if err != nil {
				return out, fmt.Errorf("playlist row %d has invalid Time (expected seconds)", i+2)
			}
			duration = float64(seconds)
		}
		kind := strings.ToLower(get("Kind"))
		if strings.Contains(kind, "audiobook") || strings.Contains(kind, "video") {
			out.Excluded++
			continue
		}
		sid := strconv.Itoa(count)
		out.Songs = append(out.Songs, Song{SourceID: sid, Title: get("Name"), Artist: get("Artist"), Album: get("Album"), Duration: duration, Original: original})
		pl.Entries = append(pl.Entries, sid)
	}
	out.Playlists = append(out.Playlists, pl)
	return out, nil
}
