package app

import (
	"context"
	"errors"
	"net/url"
	"path"
	"sort"
)

var errMetadataScanBusy = errors.New("Waiting for the current Navidrome scan")

// OpenSubsonic does not identify a song's library in getSong. Target only
// when there is one accessible library; otherwise use an incremental scan.
func (a *App) scanMetadataChanges(ctx context.Context, c Credentials, jobID string) error {
	status, err := a.nav(ctx, c, "getScanStatus", nil)
	if err != nil {
		return err
	}
	if status.Scan.Scanning {
		return errMetadataScanBusy
	}
	params := url.Values{"fullScan": {"false"}}
	folders, err := a.nav(ctx, c, "getMusicFolders", nil)
	if err == nil && len(folders.MusicFolders.Folders) == 1 && folders.MusicFolders.Folders[0].ID != "" {
		rows, err := a.DB.Query("SELECT song_id FROM metadata_files WHERE job_id=?", jobID)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		targets := map[string]bool{}
		for _, id := range ids {
			song, err := a.nav(ctx, c, "getSong", mapValues("id", id))
			if err != nil || !safeRelativePath(song.Song.Path) {
				targets = nil
				break
			}
			folder := path.Dir(song.Song.Path)
			if folder == "." {
				folder = ""
			}
			targets[folders.MusicFolders.Folders[0].ID.String()+":"+folder] = true
		}
		ordered := make([]string, 0, len(targets))
		for target := range targets {
			ordered = append(ordered, target)
		}
		sort.Strings(ordered)
		for _, target := range ordered {
			params.Add("target", target)
		}
	}
	if _, err = a.nav(ctx, c, "startScan", params); err != nil && len(params["target"]) > 0 {
		// Servers without selective-scan support can still detect changed files.
		_, err = a.nav(ctx, c, "startScan", mapValues("fullScan", "false"))
	}
	return err
}
