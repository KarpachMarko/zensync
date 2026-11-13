package app

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type mergeCompatibilityResult struct {
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
	result.Compatible = result.SourceFormat != "" && result.SourceFormat == result.TargetFormat
	if !result.Compatible {
		result.Warning = "Cannot replace " + strings.ToUpper(result.SourceFormat) + " with " + strings.ToUpper(result.TargetFormat) + " audio. Recording replacement requires matching file formats; choose another recording. No transcoding is performed."
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
