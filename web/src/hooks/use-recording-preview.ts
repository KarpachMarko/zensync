import { requestKey, trackTarget, type Track } from "@/lib/api";
import { useAudioPreview } from "./use-audio-preview";

export function previewURL(track: Track) {
  if (!Number.isSafeInteger(track.id) || track.id <= 0) return null;
  if (track.provider === "lossless" && (!track.source || !/^[a-zA-Z0-9_-]{1,40}$/.test(track.source))) return null;
  return `/api/v2/providers/${track.provider ?? "deezer"}/stream?target=` + encodeURIComponent(trackTarget(track));
}

export function useRecordingPreview({ resolveURL = previewURL, maxDuration, errorMessage }: { resolveURL?: (track: Track) => string | null; maxDuration?: number; errorMessage?: string } = {}) {
  return useAudioPreview({
    resolveURL: (track: Track) => {
      const url = resolveURL(track);
      return url && url.startsWith(`/api/v2/providers/${track.provider ?? "deezer"}/stream?`) ? url + "&sid=" + requestKey() : url;
    },
    keyFor: trackTarget,
    limitFor: () => maxDuration ?? Infinity,
    failureMessage: () => errorMessage || "Stream unavailable. Check the provider connection in Settings or open the song at its source.",
    errorMessage: errorMessage || "Audio could not be played. Check the provider connection in Settings or open the song at its source.",
  });
}
