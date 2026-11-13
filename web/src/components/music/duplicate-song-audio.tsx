import { useLayoutEffect, useRef, useState } from "react";
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
  const anchor=useRef<HTMLDivElement>(null);
  const dock=useRef<HTMLDivElement>(null);
  const [bounds,setBounds]=useState({left:0,width:0,height:0});
  useLayoutEffect(() => {
    const measure=() => {
      if (!anchor.current || !dock.current) return;
      const rect=anchor.current.getBoundingClientRect();
      const height=dock.current.getBoundingClientRect().height;
      setBounds(previous => previous.left===rect.left && previous.width===rect.width && previous.height===height ? previous : {left:rect.left,width:rect.width,height});
    };
    const observer=new ResizeObserver(measure);
    if (anchor.current) observer.observe(anchor.current);
    if (dock.current) observer.observe(dock.current);
    window.addEventListener("resize",measure);
    measure();
    return () => {observer.disconnect();window.removeEventListener("resize",measure);};
  },[]);
  return <div ref={anchor} style={{height:preview.active ? bounds.height+24 : 0}}>
    <div ref={dock} role={preview.active ? "region" : undefined} aria-label={preview.active ? "Duplicate comparison player" : undefined}
      className={preview.active ? "fixed bottom-4 z-30 rounded-lg border border-solid border-border bg-background p-3 shadow-lg" : undefined}
      style={preview.active ? {left:bounds.left,width:bounds.width} : undefined}>
      <AudioPlayer preview={preview} caption="Original file" title={song => song.title} artist={song => song.artist} recordingKey={song => song.id} />
      {preview.error && <Text role="alert" className="mb-0 text-xs">{preview.error} Use Listen to retry.</Text>}
    </div>
  </div>;
}
