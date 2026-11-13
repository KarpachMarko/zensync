import type { useAudioPreview } from "@/hooks/use-audio-preview";
import { AudioPlayer } from "./audio-player";

type PlayerProps<T extends { duration: number }> = {
  preview: ReturnType<typeof useAudioPreview<T>>;
  caption: string;
  title: (track: T) => string;
  artist: (track: T) => string;
  cover?: (track: T) => string | undefined;
  recordingKey: (track: T) => string;
  regionLabel?: string;
};
export function DockedAudioPlayer<T extends { duration: number }>({ regionLabel, ...props }: PlayerProps<T>) {
  const { preview } = props;
  return <>
    {preview.active && <div aria-hidden="true" className="h-28 shrink-0" />}
    <div role={preview.active && regionLabel ? "region" : undefined} aria-label={preview.active ? regionLabel : undefined}
      className={preview.active ? "fixed inset-x-0 bottom-0 z-[60] border-t border-border bg-background px-4 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] shadow-lg" : "hidden"}>
      <div className="mx-auto max-w-[1100px]">
        <AudioPlayer {...props} />
        {preview.error && <p role="alert" className="m-0 mt-2 text-xs text-destructive">{preview.error}</p>}
      </div>
    </div>
  </>;
}
