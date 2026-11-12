import { AudioPlayer } from "@/components/music/audio-player";
import type { useRecordingPreview } from "@/hooks/use-recording-preview";
import { label, trackTarget } from "@/lib/api";

export function PreviewPlayer({ preview, caption: customCaption }: { preview: ReturnType<typeof useRecordingPreview>; caption?: string }) {
  return <AudioPlayer preview={preview} caption={customCaption ?? (preview.active?.provider === "lossless" ? "Full track stream" : "30s preview")} title={label} artist={track => track.artist.name} cover={track => track.album.cover_medium} recordingKey={trackTarget} />;
}
