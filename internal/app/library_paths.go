package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func pathPart(s, fallback string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, " .")
	if s == "" {
		s = fallback
	}
	for len(s) > 140 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	return s
}
func safeRelativePath(s string) bool {
	if s == "" || filepath.IsAbs(s) || strings.Contains(s, `\`) {
		return false
	}
	for _, p := range strings.Split(filepath.ToSlash(s), "/") {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}
	return true
}
func (a *App) ensureMusicDirs(rel string) error {
	if !safeRelativePath(rel) {
		return errors.New("Invalid music directory")
	}
	current := a.musicDir
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		current = filepath.Join(current, part)
		info, e := os.Lstat(current)
		if os.IsNotExist(e) {
			if e = os.Mkdir(current, 0755); e != nil {
				return errors.New("Music library is not writable")
			}
			continue
		}
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Music destination contains an unsafe directory")
		}
	}
	return nil
}
func (a *App) libraryPath(source string, t Track, checksum string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data, e := exec.CommandContext(ctx, a.ffprobePath, "-v", "error", "-show_format", "-of", "json", source).Output()
	if e != nil {
		return "", errors.New("Cannot read layout metadata")
	}
	var p probe
	if json.Unmarshal(data, &p) != nil {
		return "", errors.New("Invalid layout metadata")
	}
	tags := map[string]string{}
	for k, v := range p.Format.Tags {
		tags[strings.ToLower(k)] = v
	}
	artist := tags["album_artist"]
	if artist == "" {
		artist = tags["albumartist"]
	}
	if artist == "" {
		artist = t.Artist.Name
	}
	number := func(s string) int { n, _ := strconv.Atoi(strings.Split(s, "/")[0]); return n }
	track := number(tags["track"])
	if track == 0 {
		track = number(tags["tracknumber"])
	}
	disc := number(tags["disc"])
	if disc == 0 {
		disc = number(tags["discnumber"])
	}
	total := number(tags["disctotal"])
	if parts := strings.Split(tags["disc"], "/"); len(parts) == 2 {
		total = number(parts[1])
	}
	dir := filepath.Join(pathPart(artist, "Unknown artist"), pathPart(t.Album.Title, "Unknown album"))
	if total > 1 || disc > 1 {
		dir = filepath.Join(dir, fmt.Sprintf("Disc %d", max(1, disc)))
	}
	name := pathPart(trackLabel(t), "Untitled")
	if track > 0 {
		name = fmt.Sprintf("%02d - %s", track, name)
	}
	ext := strings.ToLower(filepath.Ext(source))
	if e = a.ensureMusicDirs(dir); e != nil {
		return "", e
	}
	suffixes := []string{"", fmt.Sprintf(" [Deezer %d]", t.ID), fmt.Sprintf(" [Deezer %d-%s]", t.ID, checksum[:8])}
	if t.ID == 0 {
		suffixes = []string{"", " [Upload " + checksum[:8] + "]"}
	}
	for _, suffix := range suffixes {
		rel := filepath.Join(dir, name+suffix+ext)
		info, e := os.Lstat(filepath.Join(a.musicDir, rel))
		if os.IsNotExist(e) {
			return rel, nil
		}
		if e != nil {
			return "", e
		}
		if !info.Mode().IsRegular() {
			continue
		}
		hash, e := hashFile(filepath.Join(a.musicDir, rel))
		if e == nil && hash == checksum {
			return rel, nil
		}
	}
	return "", errors.New("Music filename collision; existing files were preserved")
}
