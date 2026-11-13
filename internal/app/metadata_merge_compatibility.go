package app

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type mergeCompatibilityResult struct {
	FormatChange bool   `json:"formatChange"`
	Compatible   bool   `json:"compatible"`
	SourceFormat string `json:"sourceFormat"`
	TargetFormat string `json:"targetFormat"`
	Warning      string `json:"warning,omitempty"`
}

func (a *App) songMergeCompatibility(ctx context.Context, c Credentials, source, target Song) (mergeCompatibilityResult, error) {
	result := mergeCompatibilityResult{}
	_, sourcePath, err := a.metadataCanonicalTags(ctx, c, source)
	if err != nil {
		return result, err
	}
	_, targetPath, err := a.metadataCanonicalTags(ctx, c, target)
	if err != nil {
		return result, err
	}
	result.SourceFormat = strings.TrimPrefix(strings.ToLower(filepath.Ext(sourcePath)), ".")
	result.TargetFormat = strings.TrimPrefix(strings.ToLower(filepath.Ext(targetPath)), ".")
	result.Compatible = uploadFormat("."+result.SourceFormat) != "" && uploadFormat("."+result.TargetFormat) != ""
	result.FormatChange = result.SourceFormat != result.TargetFormat
	if !result.Compatible {
		result.Warning = "Replacement supports MP3, FLAC, M4A, OGG, OPUS, WAV or AIFF audio"
		return result, nil
	}
	if result.FormatChange {
		result.Warning = "The recording will change from " + strings.ToUpper(result.SourceFormat) + " to " + strings.ToUpper(result.TargetFormat) + ". The file extension will change; audio is kept in its native format without transcoding. Navidrome IDs and playlist references may change after scanning."
	}
	return result, nil
}
func (a *App) mergeCompatibility(w http.ResponseWriter, r *http.Request) {
	source, target := r.URL.Query().Get("source"), r.URL.Query().Get("target")
	if source == "" || target == "" || source == target || len(source) > 256 || len(target) > 256 {
		fail(w, 400, "Choose distinct source and destination recordings")
		return
	}
	c, err := a.shared()
	if err != nil {
		fail(w, 409, err.Error())
		return
	}
	from, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {source}})
	if err != nil || from.Song.ID != source {
		fail(w, 404, "Source recording is unavailable")
		return
	}
	to, err := a.nav(r.Context(), c, "getSong", url.Values{"id": {target}})
	if err != nil || to.Song.ID != target {
		fail(w, 404, "Destination recording is unavailable")
		return
	}
	result, err := a.songMergeCompatibility(r.Context(), c, from.Song, to.Song)
	if err != nil {
		fail(w, 409, "Cannot verify recording replacement: "+err.Error())
		return
	}
	send(w, 200, result)
}
