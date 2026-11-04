package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Native provider downloads retain their actual encoding; a quality tier never
// relabels a lossy file as lossless or increases its bitrate by transcoding.
func nativeQuality(quality string) (string, int, bool) {
	parts := strings.Split(quality, ":")
	if len(parts) != 2 || (parts[0] != "mp3" && parts[0] != "aac") {
		return "", 0, false
	}
	rate, err := strconv.Atoi(parts[1])
	return parts[0], rate, err == nil && rate > 0 && rate <= 1536
}
func nativeQualityAllowed(profile []string, actual string) bool {
	for _, q := range profile {
		if actual == "lossless" && q == "lossless" {
			return true
		}
	}
	_, rate, ok := nativeQuality(actual)
	if !ok {
		return false
	}
	for _, q := range profile {
		// Enabled lossy tiers are the minimum accepted source bitrate. Keep better
		// originals rather than re-encoding them down to the tier's exact bitrate.
		floor, err := strconv.Atoi(q)
		if err == nil && rate+10 >= floor {
			return true
		}
	}
	return false
}
func (a *App) nativeAudioQuality(path string) (string, string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 || info.Size() > 1<<30 {
		return "", "", errors.New("Downloaded audio is missing, unsafe or exceeds 1 GiB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, a.ffprobePath, "-v", "error", "-show_format", "-show_streams", "-of", "json", path).Output()
	if err != nil {
		return "", "", errors.New("Could not determine the downloaded audio format")
	}
	var p probe
	if json.Unmarshal(data, &p) != nil {
		return "", "", errors.New("Invalid downloaded audio metadata")
	}
	for _, s := range p.Streams {
		if s.Type != "audio" {
			continue
		}
		mp4 := strings.Contains(p.Format.Name, "mp4") || strings.Contains(p.Format.Name, "mov")
		if s.Codec == "flac" {
			if mp4 {
				return "lossless", ".m4a", nil
			}
			return "lossless", ".flac", nil
		}
		if s.Codec != "mp3" && s.Codec != "aac" {
			break
		}
		rate, _ := strconv.Atoi(s.BitRate)
		if rate == 0 {
			rate, _ = strconv.Atoi(p.Format.BitRate)
		}
		kbps := int(math.Round(float64(rate) / 1000))
		if kbps <= 0 || kbps > 1536 {
			break
		}
		ext := ".mp3"
		if s.Codec == "aac" {
			ext = ".aac"
			if mp4 {
				ext = ".m4a"
			}
		}
		return fmt.Sprintf("%s:%d", s.Codec, kbps), ext, nil
	}
	return "", "", errors.New("Lossless.am returned an unsupported audio codec or unknown bitrate")
}
