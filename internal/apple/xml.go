// Package apple reads fixed Apple Music/iTunes XML snapshots. It never follows
// a Location or retrieves a DTD. Limits apply before metadata is persisted.
package apple

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const MaxBytes = 32 << 20

type Song struct {
	SourceID     string         `json:"sourceId"`
	PersistentID string         `json:"persistentId"`
	Title        string         `json:"title"`
	Artist       string         `json:"artist"`
	Album        string         `json:"album"`
	Duration     float64        `json:"duration"`
	Favorite     bool           `json:"favorite"`
	Missing      bool           `json:"missing"`
	Original     map[string]any `json:"original"`
}
type Playlist struct {
	Name     string   `json:"name"`
	SourceID string   `json:"sourceId"`
	ParentID string   `json:"parentId"`
	Folder   bool     `json:"folder"`
	System   bool     `json:"system"`
	Smart    bool     `json:"smart"`
	Entries  []string `json:"entries"`
}
type Snapshot struct {
	LibraryID    string     `json:"libraryId"`
	Songs        []Song     `json:"songs"`
	Playlists    []Playlist `json:"playlists"`
	Excluded     int        `json:"excluded"`
	Missing      int        `json:"missing"`
	Unplaylisted int        `json:"unplaylisted"`
	Warnings     []string   `json:"warnings"`
}
type parser struct {
	d     *xml.Decoder
	nodes int
}

func (p *parser) next() (xml.Token, error) {
	for {
		t, e := p.d.Token()
		if e != nil {
			return nil, e
		}
		switch v := t.(type) {
		case xml.Directive:
			if strings.Contains(string(v), "[") || strings.Contains(strings.ToUpper(string(v)), "ENTITY") {
				return nil, errors.New("XML entity declarations are not supported")
			}
		case xml.CharData:
			if strings.TrimSpace(string(v)) != "" {
				return nil, errors.New("unexpected text outside plist value")
			}
		case xml.Comment, xml.ProcInst:
		default:
			return t, nil
		}
	}
}
func (p *parser) value(start xml.StartElement, depth int) (any, error) {
	p.nodes++
	if depth > 32 || p.nodes > 1000000 {
		return nil, errors.New("XML structure exceeds import limits")
	}
	switch start.Name.Local {
	case "dict":
		out := map[string]any{}
		for {
			t, e := p.next()
			if e != nil {
				return nil, e
			}
			if end, ok := t.(xml.EndElement); ok {
				if end.Name.Local != "dict" {
					return nil, errors.New("invalid dictionary")
				}
				return out, nil
			}
			key, ok := t.(xml.StartElement)
			if !ok || key.Name.Local != "key" {
				return nil, errors.New("expected dictionary key")
			}
			var k string
			if k, e = p.scalar(key); e != nil {
				return nil, e
			}
			if len(k) > 1024 {
				return nil, errors.New("XML key too long")
			}
			if _, exists := out[k]; exists {
				return nil, fmt.Errorf("duplicate XML key %q", k)
			}
			t, e = p.next()
			if e != nil {
				return nil, e
			}
			v, ok := t.(xml.StartElement)
			if !ok {
				return nil, errors.New("missing dictionary value")
			}
			out[k], e = p.value(v, depth+1)
			if e != nil {
				return nil, e
			}
		}
	case "array":
		out := []any{}
		for {
			t, e := p.next()
			if e != nil {
				return nil, e
			}
			if end, ok := t.(xml.EndElement); ok {
				if end.Name.Local != "array" {
					return nil, errors.New("invalid array")
				}
				return out, nil
			}
			v, ok := t.(xml.StartElement)
			if !ok {
				return nil, errors.New("invalid array value")
			}
			item, e := p.value(v, depth+1)
			if e != nil {
				return nil, e
			}
			out = append(out, item)
		}
	case "string", "integer", "real", "date", "data", "true", "false":
		s, e := p.scalar(start)
		if e != nil {
			return nil, e
		}
		switch start.Name.Local {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "integer":
			return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		case "real":
			return strconv.ParseFloat(strings.TrimSpace(s), 64)
		}
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported plist element %s", start.Name.Local)
	}
}
func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func yes(v any) bool { b, _ := v.(bool); return b }
func number(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return 0
}
func Parse(data []byte) (Snapshot, error) {
	out := Snapshot{Songs: []Song{}, Playlists: []Playlist{}, Warnings: []string{}}
	if len(data) > MaxBytes {
		return out, errors.New("XML file exceeds 32 MiB")
	}
	p := parser{d: xml.NewDecoder(bytes.NewReader(data))}
	t, e := p.next()
	if e != nil {
		return out, e
	}
	root, ok := t.(xml.StartElement)
	if !ok || root.Name.Local != "plist" {
		return out, errors.New("expected an Apple XML plist export")
	}
	t, e = p.next()
	if e != nil {
		return out, e
	}
	dict, ok := t.(xml.StartElement)
	if !ok || dict.Name.Local != "dict" {
		return out, errors.New("expected library dictionary")
	}
	v, e := p.value(dict, 1)
	if e != nil {
		return out, e
	}
	t, e = p.next()
	if e != nil {
		return out, e
	}
	end, ok := t.(xml.EndElement)
	if !ok || end.Name.Local != "plist" {
		return out, errors.New("invalid plist end")
	}
	if _, e = p.next(); e != io.EOF {
		return out, errors.New("unexpected content after plist")
	}
	m := v.(map[string]any)
	out.LibraryID = str(m["Library Persistent ID"])
	tracks, ok := m["Tracks"].(map[string]any)
	if !ok {
		return out, errors.New("export has no Tracks dictionary")
	}
	if len(tracks) > 20000 {
		return out, errors.New("initial import limit is 20,000 source tracks; export a smaller selection")
	}
	ids := []string{}
	for id := range tracks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	present := map[string]bool{}
	excluded := map[string]bool{}
	for _, sid := range ids {
		track, ok := tracks[sid].(map[string]any)
		if !ok {
			return out, errors.New("invalid track record")
		}
		kind := strings.ToLower(str(track["Kind"]))
		if yes(track["Podcast"]) || yes(track["Has Video"]) || yes(track["Movie"]) || yes(track["TV Show"]) || strings.Contains(kind, "audiobook") || strings.Contains(kind, "video") {
			out.Excluded++
			excluded[sid] = true
			continue
		}
		s := Song{SourceID: sid, PersistentID: str(track["Persistent ID"]), Title: str(track["Name"]), Artist: str(track["Artist"]), Album: str(track["Album"]), Duration: number(track["Total Time"]) / 1000, Favorite: yes(track["Loved"]) || yes(track["Favorite"]), Original: track}
		if s.Title == "" {
			out.Warnings = append(out.Warnings, "Track "+sid+" has no title")
		}
		out.Songs = append(out.Songs, s)
		present[sid] = true
	}
	lists, _ := m["Playlists"].([]any)
	used := map[string]bool{}
	entries := 0
	for _, value := range lists {
		m, ok := value.(map[string]any)
		if !ok {
			return out, errors.New("invalid playlist record")
		}
		pl := Playlist{Name: str(m["Name"]), SourceID: str(m["Playlist Persistent ID"]), ParentID: str(m["Parent Persistent ID"]), Folder: yes(m["Folder"]), System: yes(m["Master"]) || m["Distinguished Kind"] != nil, Smart: m["Smart Info"] != nil || m["Smart Criteria"] != nil, Entries: []string{}}
		items, _ := m["Playlist Items"].([]any)
		for _, item := range items {
			ref, ok := item.(map[string]any)
			if !ok {
				return out, errors.New("invalid playlist reference")
			}
			sid := str(ref["Track ID"])
			if sid == "" {
				sid = fmt.Sprintf("missing-%d", entries)
			}
			entries++
			if entries > 100000 {
				return out, errors.New("initial import limit is 100,000 playlist entries")
			}
			if excluded[sid] {
				continue
			}
			if !present[sid] {
				present[sid] = true
				out.Missing++
				out.Songs = append(out.Songs, Song{SourceID: sid, Title: "Missing source track " + sid, Missing: true, Original: map[string]any{}})
			}
			pl.Entries = append(pl.Entries, sid)
			if !pl.System && !pl.Folder {
				used[sid] = true
			}
		}
		out.Playlists = append(out.Playlists, pl)
	}
	for _, s := range out.Songs {
		if !used[s.SourceID] && !s.Missing {
			out.Unplaylisted++
		}
	}
	return out, nil
}
func JSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func (p *parser) scalar(start xml.StartElement) (string, error) {
	var out strings.Builder
	for {
		t, e := p.d.Token()
		if e != nil {
			return "", e
		}
		switch v := t.(type) {
		case xml.CharData:
			if out.Len()+len(v) > 1<<20 {
				return "", errors.New("XML field exceeds 1 MiB")
			}
			out.Write(v)
		case xml.EndElement:
			if v.Name != start.Name {
				return "", errors.New("invalid scalar end")
			}
			return out.String(), nil
		case xml.Comment:
		default:
			return "", errors.New("nested elements or declarations are not allowed inside plist scalars")
		}
	}
}
