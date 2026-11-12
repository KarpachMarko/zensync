import { useRef, useState } from "react";
import type { LibrarySong } from "@/lib/api";
import { Text } from "@/components/typography";
import { Button } from "@/components/ui/button";

export function DuplicateSongAudio({ song }: { song: LibrarySong }) {
  const [failed, setFailed] = useState(false);
  const audio = useRef<HTMLAudioElement>(null);
  const format = song.suffix?.toUpperCase() || song.contentType || "Unknown format";
  const quality = [format, song.bitRate ? `${song.bitRate} kbps` : null,
    song.bitDepth ? `${song.bitDepth}-bit` : null,
    song.samplingRate ? `${song.samplingRate / 1000} kHz` : null].filter(Boolean).join(" · ");
  const size = song.size && song.size > 0 ? `${(song.size / 1048576).toFixed(2)} MiB (${song.size.toLocaleString()} bytes)` : "Size unknown";
  return <div className="mb-3 space-y-2">
    <Text className="m-0 text-xs">{quality} · {size}</Text>
    <audio ref={audio} controls preload="none" aria-label={`Listen to ${song.title} (${song.id})`}
      src={`/api/v2/metadata/songs/${encodeURIComponent(song.id)}/audio`}
      className="h-9 w-full max-w-md"
      onPlay={event => { for (const other of document.querySelectorAll("audio")) { if (other !== event.currentTarget) other.pause(); } }}
      onError={() => setFailed(true)} />
    {failed && <div className="flex flex-wrap items-center gap-2">
      <Text role="alert" className="m-0 text-xs">Audio could not play. The file may be unavailable or its format unsupported by your browser.</Text>
      <Button size="sm" variant="ghost" onClick={() => { setFailed(false); audio.current?.load(); }}>Retry audio</Button>
    </div>}
  </div>;
}
