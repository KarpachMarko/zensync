import { label, trackTarget, type LibrarySong, type Track } from "./api";

export type Recording = Track | LibrarySong;
export function isLibraryRecording(recording: Recording): recording is LibrarySong {
  return typeof recording.id === "string";
}
export function recordingKey(recording: Recording) {
  return isLibraryRecording(recording) ? `library:${recording.id}` : trackTarget(recording);
}
export function recordingTitle(recording: Recording) {
  return isLibraryRecording(recording) ? recording.title : label(recording);
}
export function recordingArtist(recording: Recording) {
  return isLibraryRecording(recording) ? recording.artist : recording.artist.name;
}
export function libraryCoverURL(id?: string) {
  if (id?.startsWith("upload:")) return `/api/v2/metadata/replacement-inputs/${encodeURIComponent(id.slice(7))}/cover`;
  return id ? `/api/v2/library/cover?id=${encodeURIComponent(id)}` : undefined;
}
export function recordingCover(recording: Recording) {
  return isLibraryRecording(recording) ? libraryCoverURL(recording.coverArt ?? recording.albumId) : recording.album.cover_medium;
}
export function libraryQuality(song: LibrarySong) {
  return [song.suffix?.toUpperCase() || song.contentType, song.bitRate ? `${song.bitRate} kbps` : null,
    song.bitDepth ? `${song.bitDepth}-bit` : null, song.samplingRate ? `${song.samplingRate / 1000} kHz` : null].filter(Boolean).join(" · ");
}
