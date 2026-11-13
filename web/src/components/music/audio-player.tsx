import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { time } from "@/lib/api";
import { Disc3Icon, LoaderCircleIcon, PauseIcon, PlayIcon, Volume2Icon, VolumeXIcon } from "lucide-react";
import type { useAudioPreview } from "@/hooks/use-audio-preview";
import { useState } from "react";

function Cover({ url }: { url?: string }) {
  const [failed, setFailed] = useState(false);
  const safe = url && /^https:\/\/(?:[^/]+\.(?:dzcdn\.net|deezer\.com)|avatars\.yandex\.net|lossless\.am|statcore\.hitmcdn\.com)\//.test(url);
  return <div className="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted text-muted-foreground">
    {safe && !failed ? <img src={url} alt="" className="size-full object-cover" onError={() => setFailed(true)} /> : <Disc3Icon className="size-6" aria-hidden="true" />}
  </div>;
}

export function AudioPlayer<T extends { duration: number }>({ preview, caption, title, artist, cover, recordingKey }: {
  preview: ReturnType<typeof useAudioPreview<T>>; caption: string;
  title: (track: T) => string; artist: (track: T) => string;
  cover?: (track: T) => string | undefined; recordingKey: (track: T) => string;
}) {
  const track = preview.active;
  return <>
    <audio ref={preview.attachAudio} preload="none" aria-label={caption + " player"} className="hidden"
      onPlay={preview.started} onPause={() => preview.setPlaying(false)} onEnded={() => preview.setPlaying(false)}
      onError={preview.failed} onTimeUpdate={preview.syncTime} onLoadedMetadata={preview.syncTime}
      onWaiting={() => preview.setLoading(true)} onCanPlay={() => preview.setLoading(false)} />
    {track && <section aria-label="Preview player" className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-2 rounded-lg bg-muted/50 px-3 py-2.5">
      <div className="sm:row-span-2"><Cover key={recordingKey(track)} url={cover?.(track)} /></div>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <div className="min-w-0 truncate text-xs"><strong>{title(track)}</strong><span className="ml-2 text-muted-foreground">{artist(track)}</span></div>
          <span className="shrink-0 text-[11px] text-muted-foreground">{caption}</span>
        </div>
      </div>
        <div className="col-span-2 flex min-w-0 items-center gap-2 sm:col-span-1 sm:col-start-2">
          <Button variant="default" size="icon-sm" className="inline-flex items-center justify-center rounded-full" aria-label={preview.playing ? "Pause preview" : "Play preview"} onClick={() => preview.toggle(track)}>
            {preview.loading ? <LoaderCircleIcon className="size-3.5 animate-spin motion-reduce:animate-none" /> : preview.playing ? <PauseIcon className="size-3.5" /> : <PlayIcon className="size-3.5" />}
          </Button>
          <span className="w-8 shrink-0 text-center text-[11px] tabular-nums text-muted-foreground">{time(preview.position)}</span>
          <Slider aria-label="Preview position" min={0} max={preview.duration} step={0.1} value={[preview.position]} onValueChange={([value]) => preview.seek(value)} className="min-w-10 flex-1" />
          <span className="w-8 shrink-0 text-center text-[11px] tabular-nums text-muted-foreground">{time(preview.duration)}</span>
          <Button variant="ghost" size="icon-sm" className="inline-flex items-center justify-center border-0" aria-label={preview.muted ? "Unmute preview" : "Mute preview"} onClick={preview.toggleMute}>
            {preview.muted ? <VolumeXIcon className="size-4" /> : <Volume2Icon className="size-4" />}
          </Button>
          <Slider aria-label="Preview volume" min={0} max={1} step={0.05} value={[preview.muted ? 0 : preview.volume]} onValueChange={([value]) => preview.changeVolume(value)} className="w-16 max-[600px]:hidden" />
        </div>
    </section>}
  </>;
}
