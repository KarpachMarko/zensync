package app

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Startup only, under the service lock. Remove crash leftovers only when the
// hidden name identifies a recorded metadata job/song and a generated suffix.
// Music originals and older unidentifiable files are never removed here.
func (a *App) cleanupMetadataTemps() error {
	keys := map[string]bool{}
	rows, err := a.DB.Query("SELECT id,changes FROM metadata_jobs")
	if err != nil {
		return err
	}
	for rows.Next() {
		var jid, raw string
		if err := rows.Scan(&jid, &raw); err != nil {
			rows.Close()
			return err
		}
		if len(jid) != 64 || strings.Trim(jid, "0123456789abcdef") != "" {
			continue
		}
		var changes []metadataChange
		if json.Unmarshal([]byte(raw), &changes) != nil {
			continue
		}
		for _, change := range changes {
			keys[jid+"-"+digest(change.Song.ID)] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return filepath.WalkDir(a.musicDir, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		name := entry.Name()
		var suffix string
		if strings.HasPrefix(name, ".zensync-tags-") {
			suffix = strings.TrimPrefix(name, ".zensync-tags-")
			switch filepath.Ext(name) {
			case ".mp3", ".flac", ".m4a", ".ogg", ".opus", ".wav", ".aif", ".aiff":
			default:
				return nil
			}
			suffix = strings.TrimSuffix(suffix, filepath.Ext(name))
		} else if strings.HasPrefix(name, ".zensync-cover-") {
			suffix = strings.TrimPrefix(name, ".zensync-cover-")
		} else {
			return nil
		}
		parts := strings.SplitN(suffix, "-", 3)
		if len(parts) != 3 || !keys[parts[0]+"-"+parts[1]] || parts[2] == "" || strings.Trim(parts[2], "0123456789") != "" {
			return nil
		}
		return os.Remove(path)
	})
}
