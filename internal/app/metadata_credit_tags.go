package app

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

//go:embed metadata_credit_tags.py
var metadataCreditWriter string

type artistMergeIdentity struct {
	SourceName   string `json:"sourceName"`
	TargetName   string `json:"targetName"`
	ReplaceTrack bool   `json:"replaceTrack"`
	ReplaceAlbum bool   `json:"replaceAlbum"`
}

func creditNameForID(s Song, id string) string {
	for _, credits := range [][]ArtistCredit{s.Artists, s.AlbumArtists} {
		for _, credit := range credits {
			if credit.ID == id {
				return credit.Name
			}
		}
	}
	if s.ArtistID == id {
		return s.Artist
	}
	return ""
}
func creditHasID(credits []ArtistCredit, id string) bool {
	for _, credit := range credits {
		if credit.ID == id {
			return true
		}
	}
	return false
}
func creditNames(credits []ArtistCredit) []string {
	names := []string{}
	for _, credit := range credits {
		names = append(names, credit.Name)
	}
	return names
}
func replaceCreditDisplay(credits []ArtistCredit, id, name, fallback string) string {
	if len(credits) == 0 {
		return name
	}
	result := []string{}
	for _, credit := range credits {
		if credit.ID == id {
			result = append(result, name)
		} else {
			result = append(result, credit.Name)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return strings.Join(result, " / ")
}
func metadataPythonPath() string {
	if path := os.Getenv("ZENSYNC_PYTHON"); path != "" {
		return path
	}
	return "python3"
}
func preserveMergeCredits(ctx context.Context, change *metadataChange, original, target, output string) error {
	if change.Target == nil {
		return errors.New("Merge destination is missing")
	}
	identity := artistMergeIdentity{}
	if change.ArtistMerge != nil {
		identity = *change.ArtistMerge
	} else if change.MergeKind == "artist" {
		// Older queued jobs selected the primary artist only. Recover that
		// identity while reading the original native lists to keep collaborators.
		identity = artistMergeIdentity{
			SourceName:   creditNameForID(change.Song, change.Song.ArtistID),
			TargetName:   creditNameForID(*change.Target, change.Target.ArtistID),
			ReplaceTrack: true,
		}
	}
	plan := struct {
		artistMergeIdentity
		Kind                 string            `json:"kind"`
		Original             string            `json:"original"`
		Target               string            `json:"target"`
		Output               string            `json:"output"`
		FallbackArtists      []string          `json:"fallbackArtists"`
		FallbackAlbumArtists []string          `json:"fallbackAlbumArtists"`
		TargetArtists        []string          `json:"targetArtists"`
		TargetAlbumArtists   []string          `json:"targetAlbumArtists"`
		CreditRepair         *creditRepairPlan `json:"creditRepair,omitempty"`
	}{identity, change.MergeKind, original, target, output, creditNames(songCredits(change.Song, false)), creditNames(songCredits(change.Song, true)), creditNames(songCredits(*change.Target, false)), creditNames(songCredits(*change.Target, true)), change.CreditRepair}
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, metadataPythonPath(), "-c", metadataCreditWriter)
	cmd.Stdin = bytes.NewReader(raw)
	protectChild(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		detail := stderr.String()
		if len(detail) > 2000 {
			detail = detail[len(detail)-2000:]
		}
		if strings.TrimSpace(detail) == "" {
			detail = err.Error()
		}
		return fmt.Errorf("Could not preserve individual artist credits: %s", strings.TrimSpace(detail))
	}
	var actual struct {
		Artist      string `json:"artist"`
		AlbumArtist string `json:"albumArtist"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &actual); err != nil {
		return err
	}
	change.Edit.Artist, change.Edit.AlbumArtist = actual.Artist, actual.AlbumArtist
	return nil
}

func removeMetadataArtwork(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, metadataPythonPath(), "-c", metadataCreditWriter)
	cmd.Stdin = strings.NewReader(mustJSON(map[string]string{"kind": "remove-cover", "output": path}))
	protectChild(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Could not remove cover; original retained: %v %s", err, out)
	}
	return nil
}

func preserveArtworkMetadata(ctx context.Context, change metadataChange, original, output string) error {
	changed := map[string]bool{
		"title":       change.Edit.Title != change.Song.Title,
		"album":       change.Edit.Album != change.Song.Album,
		"artist":      change.Edit.Artist != change.Song.Artist,
		"albumartist": change.Edit.AlbumArtist != "" && change.Edit.AlbumArtist != change.Song.AlbumArtist,
		"lyrics":      change.Edit.Lyrics != "" || change.Edit.SyncedLyrics != "",
		"grouping":    change.Edit.Album != change.Song.Album || change.Edit.Artist != change.Song.Artist,
	}
	plan := map[string]any{"kind": "artwork-metadata", "original": original, "output": output, "changed": changed, "artist": change.Edit.Artist, "albumartist": change.Edit.AlbumArtist}
	cmd := exec.CommandContext(ctx, metadataPythonPath(), "-c", metadataCreditWriter)
	cmd.Stdin = strings.NewReader(mustJSON(plan))
	protectChild(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Could not preserve metadata during artwork update: %v %s", err, out)
	}
	return nil
}
