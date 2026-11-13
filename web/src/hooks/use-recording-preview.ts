import { requestKey, trackTarget, type Track } from "@/lib/api";
import { useAudioPreview } from "./use-audio-preview";

export function isFullTrackStream(track: Track | null) {
  return track?.provider === "lossless" || track?.provider === "hitmo";
}

export function previewURL(track: Track) {
  if (track.provider === "hitmo") {
    if (!Number.isSafeInteger(track.id) || track.id <= 0) return null;
    return "/api/v2/providers/hitmo/stream?target=" + encodeURIComponent(trackTarget(track));
  }
  if (track.provider === "lossless") {
    if (!track.source || !/^[a-zA-Z0-9_-]{1,40}$/.test(track.source) || !Number.isSafeInteger(track.id) || track.id <= 0) return null;
    return "/api/v2/providers/lossless/stream?target=" + encodeURIComponent(trackTarget(track));
  }
  if (!track.preview) return null;
  try {
    const url = new URL(track.preview);
    const deezerHost = url.hostname.endsWith(".dzcdn.net") || url.hostname.endsWith(".deezer.com");
    return url.protocol === "https:" && deezerHost && !url.username && !url.password ? url.href : null;
  } catch {
    return null;
  }
}

export function useRecordingPreview({ resolveURL = previewURL, maxDuration, errorMessage }: { resolveURL?: (track: Track) => string | null; maxDuration?: number; errorMessage?: string } = {}) {
  return useAudioPreview({
    resolveURL: (track: Track) => {
      const url = resolveURL(track);
      return url && isFullTrackStream(track) && url.startsWith(`/api/v2/providers/${track.provider}/stream?`) ? url + "&sid=" + requestKey() : url;
    },
    keyFor: trackTarget,
    limitFor: track => maxDuration ?? (isFullTrackStream(track) ? Infinity : 30),
    failureMessage: track => errorMessage || (isFullTrackStream(track) ? "Stream unavailable. Try again or open the song at its source." : "Preview unavailable. Open the song in Deezer to listen."),
    errorMessage: errorMessage || "Audio could not be played. Try again or open the song at its source.",
  });
}
