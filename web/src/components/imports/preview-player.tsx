import { DockedAudioPlayer } from "@/components/music/docked-audio-player";
import type { useRecordingPreview } from "@/hooks/use-recording-preview";
import { isLibraryRecording, recordingArtist, recordingCover, recordingKey, recordingTitle, type Recording } from "@/lib/playback";

export function PreviewPlayer<T extends Recording>({ preview, caption }: { preview: ReturnType<typeof useRecordingPreview<T>>; caption?: string }) {
  return <DockedAudioPlayer preview={preview} caption={caption ?? (preview.active && isLibraryRecording(preview.active) ? "Library stream" : "Full track stream")}
    title={recordingTitle} artist={recordingArtist} cover={recordingCover} recordingKey={recordingKey} />;
}
