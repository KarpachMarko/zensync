import { requestKey, trackTarget, type Track } from "@/lib/api";
import { isLibraryRecording, recordingKey, type Recording } from "@/lib/playback";
import { useAudioPreview } from "./use-audio-preview";

export function previewURL(track: Recording) {
  if (isLibraryRecording(track) && track.id.startsWith("upload:")) return `/api/v2/metadata/replacement-inputs/${encodeURIComponent(track.id.slice(7))}/audio`;
  if (isLibraryRecording(track)) return track.id ? "/api/v2/library/stream?id=" + encodeURIComponent(track.id) : null;
  if (!Number.isSafeInteger(track.id) || track.id <= 0) return null;
  if (track.provider === "lossless" && (!track.source || !/^[a-zA-Z0-9_-]{1,40}$/.test(track.source))) return null;
  return `/api/v2/providers/${track.provider ?? "deezer"}/stream?target=` + encodeURIComponent(trackTarget(track));
}

export function useRecordingPreview<T extends Recording = Track>({ resolveURL = previewURL, maxDuration, errorMessage }: { resolveURL?: (track: T) => string | null; maxDuration?: number; errorMessage?: string } = {}) {
  return useAudioPreview<T>({
    resolveURL: (track: T) => {
      const url = resolveURL(track);
      return url && !isLibraryRecording(track) && url.startsWith(`/api/v2/providers/${track.provider ?? "deezer"}/stream?`) ? url + "&sid=" + requestKey() : url;
    },
    keyFor: recordingKey,
    limitFor: () => maxDuration ?? Infinity,
    failureMessage: () => errorMessage || "Stream unavailable. Check the provider connection in Settings or open the song at its source.",
    errorMessage: errorMessage || "Audio could not be played. Check the provider connection in Settings or open the song at its source.",
  });
}
