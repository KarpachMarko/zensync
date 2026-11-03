import { Button } from "@/components/ui/button";
import type { Track } from "@/lib/api";
import { label } from "@/lib/api";
import { previewURL } from "@/hooks/use-deezer-preview";
import { ExternalLinkIcon, PauseIcon, PlayIcon } from "lucide-react";

export function DeezerRecordingTools({
  track,
  playing,
  onPreview,
}: {
  track: Track;
  playing: boolean;
  onPreview: (track: Track) => void;
}) {
  const available = !!previewURL(track);
  return (
    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
      <Button
        type="button"
        variant="ghost"
        className="inline-flex items-center gap-1 border-0 px-1 py-0.5 text-xs"
        disabled={!available}
        aria-label={`${playing ? "Pause" : "Play"} 30-second preview of ${label(track)}`}
        aria-pressed={playing}
        title={available ? "Listen to the Deezer preview" : "Deezer has no preview for this recording"}
        onClick={() => onPreview(track)}
      >
        {playing ? <PauseIcon className="size-3" aria-hidden="true" /> : <PlayIcon className="size-3" aria-hidden="true" />}
        {available ? (playing ? "Pause preview" : "Preview 30s") : "No preview"}
      </Button>
      <a
        href={`https://www.deezer.com/track/${track.id}`}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={`Open ${label(track)} in Deezer (opens in new tab)`}
        className="inline-flex items-center gap-1 text-muted-foreground underline-offset-2 hover:text-foreground hover:underline focus-visible:outline-2 focus-visible:outline-solid focus-visible:outline-primary"
      >
        Open in Deezer <ExternalLinkIcon className="size-3" aria-hidden="true" />
      </a>
    </div>
  );
}
