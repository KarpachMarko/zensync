import { AudioPlayer } from "@/components/music/audio-player";
import { isFullTrackStream, type useRecordingPreview } from "@/hooks/use-recording-preview";
import { label, trackTarget } from "@/lib/api";

export function PreviewPlayer({ preview, caption: customCaption }: { preview: ReturnType<typeof useRecordingPreview>; caption?: string }) {
  return <AudioPlayer preview={preview} caption={customCaption ?? (isFullTrackStream(preview.active) ? "Full track stream" : "30s preview")} title={label} artist={track => track.artist.name} cover={track => track.album.cover_medium} recordingKey={trackTarget} />;
}
