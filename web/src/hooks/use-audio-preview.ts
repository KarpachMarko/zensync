import { useCallback, useRef, useState } from "react";

// One player per review dialog keeps comparisons from playing over each other.
export function useAudioPreview<T extends { duration: number }>({ resolveURL, keyFor, limitFor = () => Infinity, failureMessage, errorMessage = "Audio could not be played. Try again." }: {
  resolveURL: (track: T) => string | null;
  keyFor: (track: T) => string;
  limitFor?: (track: T | null) => number;
  errorMessage?: string;
  failureMessage?: (track: T | null) => string;
}) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const [active, setActive] = useState<T | null>(null);
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

  function toggle(track: T) {
    const audio = audioRef.current;
    let url = resolveURL(track);
    if (!audio || !url) return;
    const sameTrack = active && keyFor(active) === keyFor(track);
    if (sameTrack && !audio.paused) {
      audio.pause();
      return;
    }
    const maxDuration = limitFor(track);
    if (sameTrack && audio.src) url = audio.src;
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
    if (audio.error) audio.load();
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
    setError(failureMessage?.(active) ?? errorMessage);
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
