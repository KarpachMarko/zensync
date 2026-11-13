import type { LibrarySong } from "@/lib/api";
import { time } from "@/lib/api";
import { libraryQuality, recordingCover } from "@/lib/playback";
import { RecordingCover } from "./audio-player";
import { Button } from "@/components/ui/button";
import { PauseIcon, PlayIcon } from "lucide-react";

export type LibraryPlaybackControls = {
  active: { id: string | number } | null;
  playing: boolean;
  toggle: (song: LibrarySong) => void;
  stop: () => void;
};
export function LibraryRecordingDetails({ song, playback }: { song: LibrarySong; playback: LibraryPlaybackControls }) {
  const playing = playback.playing && playback.active?.id === song.id;
  return <div className="flex min-w-0 flex-1 items-start gap-3">
    <RecordingCover key={song.id} url={recordingCover(song)} />
    <div className="min-w-0 flex-1 break-words">
      <strong className="text-sm">{song.title}</strong>
      <p className="m-0 mt-1 text-xs text-muted-foreground">{song.artist} / {song.album} / {time(song.duration)}</p>
      {libraryQuality(song) && <p className="m-0 mt-1 text-xs text-muted-foreground">{libraryQuality(song)}</p>}
      <Button type="button" variant="ghost" size="sm" className="mt-1 inline-flex items-center gap-1 border-0 px-1"
        aria-label={`${playing ? "Pause" : "Play"} library recording: ${song.title}`} aria-pressed={playing} onClick={() => playback.toggle(song)}>
        {playing ? <PauseIcon className="size-3" /> : <PlayIcon className="size-3" />}{playing ? "Pause" : "Listen"}
      </Button>
    </div>
  </div>;
}
