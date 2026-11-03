import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import type { useDeezerPreview } from "@/hooks/use-deezer-preview";
import { label, time } from "@/lib/api";
import { Disc3Icon, LoaderCircleIcon, PauseIcon, PlayIcon, Volume2Icon, VolumeXIcon } from "lucide-react";
import { useState } from "react";

function Cover({ url }: { url?: string }) {
  const [failed, setFailed] = useState(false);
  const safe = url && /^https:\/\/[^/]+\.(dzcdn\.net|deezer\.com)\//.test(url);
  return <div className="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted text-muted-foreground">
    {safe && !failed ? <img src={url} alt="" className="size-full object-cover" onError={() => setFailed(true)} /> : <Disc3Icon className="size-6" aria-hidden="true" />}
  </div>;
}

export function PreviewPlayer({ preview }: { preview: ReturnType<typeof useDeezerPreview> }) {
  const track = preview.active;
  return <>
    <audio ref={preview.attachAudio} preload="none" aria-label="Deezer 30-second preview player" className="hidden"
      onPlay={preview.started} onPause={() => preview.setPlaying(false)} onEnded={() => preview.setPlaying(false)}
      onError={preview.failed} onTimeUpdate={preview.syncTime} onLoadedMetadata={preview.syncTime}
      onWaiting={() => preview.setLoading(true)} onCanPlay={() => preview.setLoading(false)} />
    {track && <section aria-label="Preview player" className="flex items-center gap-3 rounded-lg bg-muted/50 px-3 py-2.5">
      <Cover key={track.id} url={track.album.cover_medium} />
      <div className="min-w-0 flex-1">
        <div className="mb-2 flex items-baseline justify-between gap-3">
          <div className="min-w-0 truncate text-xs"><strong>{label(track)}</strong><span className="ml-2 text-muted-foreground">{track.artist.name}</span></div>
          <span className="shrink-0 text-[11px] text-muted-foreground">30s preview</span>
        </div>
        <div className="flex items-center gap-2">
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
      </div>
    </section>}
  </>;
}
