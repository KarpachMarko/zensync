import type { Track } from "@/lib/api";
import { useCallback, useRef, useState } from "react";

export function previewURL(track: Track) {
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
export function useDeezerPreview() {
  const audioRef = useRef<HTMLAudioElement>(null);
  const [active, setActive] = useState<Track | null>(null);
  const [playing, setPlaying] = useState(false);
  const [error, setError] = useState("");

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
    const url = previewURL(track);
    if (!audio || !url) return;
    if (active?.id === track.id && !audio.paused) {
      audio.pause();
      return;
    }
    setActive(track);
    setError("");
    if (audio.src !== url) {
      audio.pause();
      audio.src = url;
    }
    if (audio.ended || audio.currentTime >= 30) audio.currentTime = 0;
    // Start during the click so browser autoplay restrictions are respected.
    void audio.play().catch((cause: unknown) => {
      if (audio.src !== url || (cause instanceof DOMException && cause.name === "AbortError")) return;
      setPlaying(false);
      setError("Preview could not be played. Try again or open the song in Deezer.");
    });
  }

  function started() {
    setPlaying(true);
    setError("");
  }

  function failed() {
    audioRef.current?.pause();
    setPlaying(false);
    setError("Preview unavailable. Open the song in Deezer to listen.");
  }

  return { attachAudio, active, playing, error, setPlaying, toggle, started, failed };
}
