import { AudioPlayer } from "@/components/music/audio-player";
import type { useRecordingPreview } from "@/hooks/use-recording-preview";
import { label, trackTarget } from "@/lib/api";

export function PreviewPlayer({ preview, caption: customCaption }: { preview: ReturnType<typeof useRecordingPreview>; caption?: string }) {
  return <>
    {preview.active && <div aria-hidden="true" className="h-28 shrink-0" />}
    <div className={preview.active ? "fixed inset-x-0 bottom-0 z-[60] border-t border-border bg-background px-4 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] shadow-lg" : "hidden"}>
    <div className="mx-auto max-w-[1100px]">
    <AudioPlayer preview={preview} caption={customCaption ?? "Full track stream"} title={label} artist={track => track.artist.name} cover={track => track.album.cover_medium} recordingKey={trackTarget} />
    {preview.error && <p role="alert" className="m-0 mt-2 text-xs text-destructive">{preview.error}</p>}
    </div></div></>;
}
