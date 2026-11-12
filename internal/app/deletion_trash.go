package app

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxDeletionTrashBytes int64 = 500_000_000
const maxDeletionTrashAge = 30 * 24 * time.Hour

type deletionTrashEntry struct {
	id, path string
	bytes    int64
	modified time.Time
}

func (a *App) deletionTrashPath(rid string) (string, error) {
	if len(rid) != 64 || strings.Trim(rid, "0123456789abcdef") != "" {
		return "", errors.New("Invalid deletion recovery ID")
	}
	root := filepath.Join(a.dataDir, "trash")
	for _, dir := range []string{a.dataDir, root, filepath.Join(root, rid)} {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("Unsafe recovery directory")
		}
	}
	path := filepath.Join(root, rid, "audio")
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("Unsafe recovery file")
	}
	return path, nil
}

// Caller holds opMu/workMu so retention cannot race deletion or restoration.
func (a *App) trimDeletionTrash(reserveID string, reserve int64) error {
	if reserve > maxDeletionTrashBytes {
		return errors.New("Song exceeds the 500 MB recovery limit; original file retained")
	}
	root := filepath.Join(a.dataDir, "trash")
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Unsafe trash directory")
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	entries := []deletionTrashEntry{}
	total := reserve
	for _, dir := range dirs {
		path, err := a.deletionTrashPath(dir.Name())
		if err != nil {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if dir.Name() == reserveID {
			continue
		} // Existing partial copy will be overwritten, not counted twice.
		entry := deletionTrashEntry{dir.Name(), path, info.Size(), info.ModTime()}
		entries = append(entries, entry)
		total += entry.bytes
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].modified.Before(entries[j].modified) })
	for _, entry := range entries {
		if time.Since(entry.modified) < maxDeletionTrashAge && total <= maxDeletionTrashBytes {
			continue
		}
		if err := os.Remove(entry.path); err != nil {
			return err
		}
		message := "Removed from shared library; recovery copy removed by the 500 MB retention limit"
		if time.Since(entry.modified) >= maxDeletionTrashAge {
			message = "Removed from shared library; recovery copy expired after 30 days"
		}
		if _, err := a.DB.Exec("UPDATE jobs SET stage=? WHERE state='completed' AND request_id IN (SELECT request_id FROM song_deletions WHERE request_id=? AND phase='removed')", message, entry.id); err != nil {
			return err
		}
		_ = os.Remove(filepath.Dir(entry.path))
		total -= entry.bytes
	}
	if total > maxDeletionTrashBytes {
		return errors.New("Could not make room within the 500 MB recovery limit; original file retained")
	}
	return nil
}
func (a *App) reserveDeletionTrash(rid string, bytes int64) error {
	for _, path := range []string{filepath.Join(a.dataDir, "trash", rid), filepath.Join(a.dataDir, "trash", rid, "audio")} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Unsafe recovery path; original retained")
		}
	}
	return a.trimDeletionTrash(rid, bytes)
}
func (a *App) pruneDeletionTrash() error {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	a.workMu.Lock()
	defer a.workMu.Unlock()
	return a.trimDeletionTrash("", 0)
}
func (a *App) availableDeletionCopy(rid string) bool {
	path, err := a.deletionTrashPath(rid)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) < maxDeletionTrashAge
}
