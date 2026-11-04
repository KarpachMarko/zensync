import { requestKey, trackTarget, type Track } from "@/lib/api";
import { useCallback, useRef, useState } from "react";

export function previewURL(track: Track) {
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

// One player per review dialog keeps comparisons from playing over each other.
export function useRecordingPreview({ resolveURL = previewURL, maxDuration, errorMessage }: { resolveURL?: (track: Track) => string | null; maxDuration?: number; errorMessage?: string } = {}) {
  const limitFor = (track: Track | null) => maxDuration ?? (track?.provider === "lossless" ? Infinity : 30);
  const audioRef = useRef<HTMLAudioElement>(null);
  const [active, setActive] = useState<Track | null>(null);
  const [playing, setPlaying] = useState(false);
  const [error, setError] = useState("");
  const [position, setPosition] = useState(0);
  const [duration, setDuration] = useState(30);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [loading, setLoading] = useState(false);

  // A callback ref also handles audio mounted through the dialog's portal.
  const attachAudio = useCallback((audio: HTMLAudioElement | null) => {
    const previous = audioRef.current;
    if (previous && previous !== audio) {
      previous.pause();
      previous.removeAttribute("src");
      previous.load();
    }
    audioRef.current = audio;
  }, []);

  function toggle(track: Track) {
    const audio = audioRef.current;
    let url = resolveURL(track);
    if (!audio || !url) return;
    const sameTrack = active && trackTarget(active) === trackTarget(track);
    if (sameTrack && !audio.paused) {
      audio.pause();
      return;
    }
    const maxDuration = limitFor(track);
    if (track.provider === "lossless" && url.startsWith("/api/v2/providers/lossless/stream?")) {
      url = sameTrack && audio.src ? audio.src : url + "&sid=" + requestKey();
    }
    url = new URL(url, window.location.origin).href;
    setActive(track);
    setError("");
    if (audio.src !== url) {
      audio.pause();
      audio.src = url;
      setPosition(0);
      setDuration(Number.isFinite(maxDuration) ? maxDuration : track.duration);
      setLoading(true);
    }
    if (audio.ended || audio.currentTime >= maxDuration) audio.currentTime = 0;
    // Start during the click so browser autoplay restrictions are respected.
    void audio.play().catch((cause: unknown) => {
      if (audio.src !== url || (cause instanceof DOMException && cause.name === "AbortError")) return;
      setPlaying(false);
      setLoading(false);
      setError(errorMessage || "Audio could not be played. Try again or open the song at its source.");
    });
  }

  function started() {
    setPlaying(true);
    setError("");
    setLoading(false);
  }

  function failed() {
    audioRef.current?.pause();
    setPlaying(false);
    setLoading(false);
    setError(errorMessage || (active?.provider === "lossless" ? "Stream unavailable. Check the provider connection in Settings or open the song at its source." : "Preview unavailable. Open the song in Deezer to listen."));
  }

  function syncTime() {
    const audio = audioRef.current;
    if (!audio) return;
    const maxDuration = limitFor(active);
    const limit = Number.isFinite(audio.duration) && audio.duration > 0 ? Math.min(maxDuration, audio.duration) : duration;
    setDuration(limit);
    setPosition(Math.min(limit, audio.currentTime));
    if (audio.currentTime >= limit) audio.pause();
  }
  function seek(seconds: number) {
    const audio = audioRef.current;
    if (!audio || !Number.isFinite(audio.duration)) return;
    audio.currentTime = Math.max(0, Math.min(duration, seconds));
    syncTime();
  }
  function changeVolume(value: number) {
    const audio = audioRef.current;
    if (!audio) return;
    audio.volume = value;
    audio.muted = value === 0;
    setVolume(value);
    setMuted(audio.muted);
  }
  function toggleMute() {
    const audio = audioRef.current;
    if (!audio) return;
    audio.muted = !audio.muted;
    setMuted(audio.muted);
  }
  return { attachAudio, active, playing, error, setPlaying, toggle, started, failed,
    position, duration, volume, muted, loading, setLoading, syncTime, seek, changeVolume, toggleMute };
}
