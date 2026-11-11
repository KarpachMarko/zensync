package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Published recovery copies are a bounded, short-lived undo window. Pending
// copies are never evicted: new writes stop instead of sacrificing recovery.
const metadataBackupRetention = 7 * 24 * time.Hour

type MetadataBackupInfo struct {
	Key         string    `json:"key"`
	JobID       string    `json:"jobId,omitempty"`
	SongID      string    `json:"songId,omitempty"`
	State       string    `json:"state,omitempty"`
	MusicPath   string    `json:"musicPath,omitempty"`
	BeforeHash  string    `json:"beforeHash,omitempty"`
	AfterHash   string    `json:"afterHash,omitempty"`
	Bytes       int64     `json:"bytes"`
	Modified    time.Time `json:"modified"`
	Reclaimable bool      `json:"reclaimable"`
}

func metadataBackupLimit() (int64, error) {
	value := os.Getenv("ZENSYNC_METADATA_BACKUP_LIMIT_MIB")
	if value == "" {
		return 1 << 30, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || n > 1048576 {
		return 0, errors.New("ZENSYNC_METADATA_BACKUP_LIMIT_MIB must be between 1 and 1048576")
	}
	return n << 20, nil
}

// Inventory also includes unrecognized directories, protected by default.
func (a *App) MetadataBackupInventory() ([]MetadataBackupInfo, error) {
	root := filepath.Join(a.dataDir, "metadata-backups")
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return []MetadataBackupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Unsafe metadata recovery directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	jobIDs := map[string]bool{}
	for _, entry := range entries {
		parts := strings.Split(entry.Name(), "-")
		if len(parts) == 2 && len(parts[0]) == 64 && len(parts[1]) == 64 && strings.Trim(parts[0]+parts[1], "0123456789abcdef") == "" {
			jobIDs[parts[0]] = true
		}
	}
	args := []any{}
	for jid := range jobIDs {
		args = append(args, jid)
	}
	predicate := "0"
	if len(args) > 0 {
		predicate = "id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + ")"
	}
	known := map[string]MetadataBackupInfo{}
	rows, err := a.DB.Query("SELECT id,state,done,changes FROM metadata_jobs WHERE "+predicate, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var jid, state, raw string
		var done int
		if err = rows.Scan(&jid, &state, &done, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var changes []metadataChange
		if json.Unmarshal([]byte(raw), &changes) != nil {
			continue
		}
		for pos, change := range changes {
			key := jid + "-" + digest(change.Song.ID)
			known[key] = MetadataBackupInfo{Key: key, JobID: jid, SongID: change.Song.ID, State: state, Reclaimable: done >= 0 && done <= len(changes) && pos < done}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = a.DB.Query("SELECT job_id,song_id,file_path,before_hash,after_hash FROM metadata_files WHERE "+strings.Replace(predicate, "id IN", "job_id IN", 1), args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var jid, sid, path, before, after string
		if err = rows.Scan(&jid, &sid, &path, &before, &after); err != nil {
			rows.Close()
			return nil, err
		}
		key := jid + "-" + digest(sid)
		info := known[key]
		info.Key = key
		info.JobID = jid
		info.SongID = sid
		info.MusicPath = path
		info.BeforeHash = before
		info.AfterHash = after
		known[key] = info
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []MetadataBackupInfo{}
	for _, entry := range entries {
		item, ok := known[entry.Name()]
		if !ok {
			item = MetadataBackupInfo{Key: entry.Name()}
		}
		// Only exact job/song keys consisting of hex IDs can be removed.
		parts := strings.Split(item.Key, "-")
		valid := len(parts) == 2 && len(parts[0]) == 64 && len(parts[1]) == 64 && strings.Trim(parts[0]+parts[1], "0123456789abcdef") == ""
		if !valid || !entry.IsDir() {
			item.Reclaimable = false
		}
		path := filepath.Join(root, entry.Name())
		err = filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("Symlink in metadata recovery storage; cleanup refused")
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(item.Modified) {
				item.Modified = info.ModTime()
			}
			if info.Mode().IsRegular() {
				item.Bytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// Caller serializes with metadata writes (worker locks or offline worker lock).
func (a *App) cleanupMetadataBackups(limit, reserve int64, now time.Time) ([]MetadataBackupInfo, error) {
	entries, err := a.MetadataBackupInventory()
	if err != nil {
		return nil, err
	}
	var total int64
	for _, entry := range entries {
		total += entry.Bytes
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Modified.Before(entries[j].Modified) })
	removed := []MetadataBackupInfo{}
	for _, entry := range entries {
		if !entry.Reclaimable {
			continue
		}
		if total+reserve <= limit && now.Sub(entry.Modified) < metadataBackupRetention {
			continue
		}
		if err := os.RemoveAll(filepath.Join(a.dataDir, "metadata-backups", entry.Key)); err != nil {
			return removed, err
		}
		total -= entry.Bytes
		removed = append(removed, entry)
	}
	if total+reserve > limit {
		return removed, fmt.Errorf("Metadata recovery storage limit reached (%d MiB retained, %d MiB limit). Unfinished or unrecognized backups are protected; resolve old failed jobs or explicitly increase ZENSYNC_METADATA_BACKUP_LIMIT_MIB", total>>20, limit>>20)
	}
	return removed, nil
}
func (a *App) CleanupMetadataBackups() ([]MetadataBackupInfo, error) {
	limit, err := metadataBackupLimit()
	if err != nil {
		return nil, err
	}
	return a.cleanupMetadataBackups(limit, 0, time.Now())
}
