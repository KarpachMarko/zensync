import type { LibrarySong } from "@/lib/api";
import { Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { PlayIcon, PauseIcon } from "lucide-react";
import { useAudioPreview } from "@/hooks/use-audio-preview";
import { AudioPlayer } from "@/components/music/audio-player";

type LibraryPreview = ReturnType<typeof useAudioPreview<LibrarySong>>;
export function DuplicateSongAudio({ song, preview }: { song: LibrarySong; preview: LibraryPreview }) {
  const format = song.suffix?.toUpperCase() || song.contentType || "Unknown format";
  const quality = [format, song.bitRate ? `${song.bitRate} kbps` : null,
    song.bitDepth ? `${song.bitDepth}-bit` : null,
    song.samplingRate ? `${song.samplingRate / 1000} kHz` : null].filter(Boolean).join(" · ");
  const size = song.size && song.size > 0 ? `${(song.size / 1048576).toFixed(2)} MiB` : "Size unknown";
  const playing = preview.active?.id === song.id && preview.playing;
  return <div className="mb-3 flex flex-wrap items-center gap-2">
    <Button variant="ghost" size="sm" aria-label={`${playing ? "Pause" : "Play"} ${song.title} (${song.id})`} aria-pressed={playing} onClick={() => preview.toggle(song)}>
      {playing ? <PauseIcon className="size-3" aria-hidden="true" /> : <PlayIcon className="size-3" aria-hidden="true" />}{playing ? "Pause" : "Listen"}
    </Button>
    <Text className="m-0 text-xs">{quality} · {size}</Text>
  </div>;
}

export function DuplicateAudioPlayer({ preview }: { preview: LibraryPreview }) {
  return <>
    <AudioPlayer preview={preview} caption="Original file" title={song => song.title} artist={song => song.artist} recordingKey={song => song.id} />
    {preview.error && <Text role="alert" className="text-xs">{preview.error} Use Listen to retry.</Text>}
  </>;
}
