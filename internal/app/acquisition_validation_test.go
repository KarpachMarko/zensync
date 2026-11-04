package app

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquisitionReportsPerQualityAccountFailuresWithoutLeakingRawOutput(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe required")
	}
	a, _, _ := albumFixture(t)
	a.musicDir = t.TempDir()
	a.dfiPath = filepath.Join(t.TempDir(), "fake-d-fi")
	for _, test := range []struct{ code, expected, exit string }{
		{"geo_blocked", "configured account's country", "1"},
		{"license_required", "cannot stream this quality", "1"},
		{"unavailable", "no downloadable recording", "0"},
	} {
		script := "#!/bin/sh\nprintf '%s\\n' 'ZENSYNC_RESULT {\"code\":\"" + test.code + "\"}'\nprintf '%s\\n' 'private-cookie-and-url' >&2\nexit " + test.exit + "\n"
		if err := os.WriteFile(a.dfiPath, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		_, _, err := a.acquire(context.Background(), "diagnostic-job-"+test.code, "deezer:track:42", makeTrack(42, "Song"), `["lossless","320","128"]`, "private-cookie-and-url")
		if err == nil {
			t.Fatal("failed provider produced a successful acquisition")
		}
		for _, quality := range []string{"lossless", "320", "128"} {
			if !strings.Contains(err.Error(), quality+": ") {
				t.Fatalf("quality diagnostic missing: %v", err)
			}
		}
		if !strings.Contains(err.Error(), test.expected) || strings.Contains(err.Error(), "private-cookie-and-url") {
			t.Fatalf("wrong or unsafe diagnostic: %v", err)
		}
	}
}

func TestVerifiedISRCAllowsSmallCatalogDurationDiscrepancy(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg required")
	}
	file := filepath.Join(t.TempDir(), "molly.flac")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=117.144", "-c:a", "flac", "-metadata", "title=Molly Girl", "-metadata", "artist=Lil Tecca", "-metadata", "album=We Love You Tecca", "-metadata", "SOURCEID=741757232", "-metadata", "ISRC=QZ85M1918619", file}
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	a := &App{ffprobePath: "ffprobe"}
	track := makeTrack(741757232, "Molly Girl")
	track.Artist.Name = "Lil Tecca"
	track.Album.Title = "We Love You Tecca"
	track.Duration = 121
	track.ISRC = "QZ85M1918619"
	if err = a.validateAudio(file, track, "lossless"); err != nil {
		t.Fatal(err)
	}
	track.ISRC = ""
	if err = a.validateAudio(file, track, "lossless"); err == nil || !strings.Contains(err.Error(), "expected 121.000s, measured 117.144s") {
		t.Fatalf("unverified discrepancy accepted or diagnostics missing: %v", err)
	}
	track.ISRC = "WRONG"
	if err = a.validateAudio(file, track, "lossless"); err == nil || !strings.Contains(err.Error(), "ISRC differs") {
		t.Fatalf("wrong identity accepted: %v", err)
	}
	track.ISRC = "QZ85M1918619"
	track.Duration = 130
	if err = a.validateAudio(file, track, "lossless"); err == nil || !strings.Contains(err.Error(), "duration differs") {
		t.Fatalf("large discrepancy accepted: %v", err)
	}
}

func TestApprovalMetadataComparisonAcceptsFormattingAndRoundingButRejectsDifferentRecordings(t *testing.T) {
	approved := makeTrack(42, "Song (feat. Guest)")
	approved.Duration = 180.8
	approved.ISRC = "ISRC42"
	approved.Album.Title = "Album - Single"
	current := approved
	current.Duration = 180
	current.Title = "Song"
	current.Album.Title = "Album"
	current.Contributors = []Artist{{Name: approved.Artist.Name}, {Name: "Guest", Role: "Featured"}}
	if !sameApprovedRecording(approved, current) {
		t.Fatal("harmless catalog formatting/rounding rejected")
	}
	for _, test := range []struct {
		name   string
		change func(*Track)
	}{
		{"different ID", func(t *Track) { t.ID++ }},
		{"different ISRC", func(t *Track) { t.ISRC = "OTHER" }},
		{"lost ISRC", func(t *Track) { t.ISRC = "" }},
		{"remaster", func(t *Track) { t.TitleVersion = "Remastered" }},
		{"different duration", func(t *Track) { t.Duration = 210 }},
		{"missing duration", func(t *Track) { t.Duration = 0 }},
		{"different artist", func(t *Track) { t.Artist.Name = "Someone else" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := current
			test.change(&changed)
			if sameApprovedRecording(approved, changed) {
				t.Fatal("changed recording accepted")
			}
		})
	}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if validDuration(value) {
			t.Fatal("invalid duration accepted")
		}
	}
	if verifiedDurationTolerance(2, true) > 1 {
		t.Fatal("long tolerance used for a short clip")
	}
}

func TestDownloaderFailureProtocolUsesOnlyFixedSafeMessages(t *testing.T) {
	for _, code := range []string{"geo_blocked", "license_required", "unavailable", "timeout", "provider_error"} {
		output := &transferOutput{app: &App{}}
		output.event([]byte(`ZENSYNC_RESULT {"code":"`+code+`","ignored":"private-cookie"}`), time.Now())
		if output.failure == "" || strings.Contains(output.failure, "private-cookie") {
			t.Fatalf("unsafe diagnostic for %s: %s", code, output.failure)
		}
	}
	output := &transferOutput{app: &App{}}
	output.event([]byte(`ZENSYNC_RESULT {"code":"private-cookie"}`), time.Now())
	if output.failure != "" {
		t.Fatal("untrusted diagnostic text was retained")
	}
}
