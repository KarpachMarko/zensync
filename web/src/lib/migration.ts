import type { Track } from "./api";
export type Migration = {
  id: string;
  name: string;
  state: string;
  favorites: boolean;
  targetUser: string;
  error: string;
  report: {
    songs: number;
    playlists: number;
    unplaylisted: number;
    missing: number;
    excluded: number;
    warnings: string[];
  };
};
export type Source = {
  id: string;
  metadata: {
    title: string;
    artist: string;
    album: string;
    duration: number;
    favorite: boolean;
    missing: boolean;
  };
  serverId: string;
  candidate: Track | null;
  matchState: string;
  deferred: boolean;
  favoriteState: string;
  requestId: string;
  decision: string;
  jobState: string;
};
export type Playlist = {
  id: string;
  name: string;
  selected: boolean;
  state: string;
  remoteId: string;
  baseline: string;
  desired: string;
  error: string;
  entries: string[];
  metadata: {
    folder: boolean;
    system: boolean;
    smart: boolean;
    parentId: string;
  };
};
export type Detail = {
  matchTask?: { state: string; message: string };
  migration: Migration;
  songs: Source[];
  playlists: Playlist[];
};
export function status(s: Source, scanning = false) {
  return s.serverId
    ? "Available"
    : s.deferred
      ? "Skipped for now"
      : s.jobState
        ? s.jobState.replaceAll("_", " ")
        : s.decision
          ? s.decision.replaceAll("_", " ")
          : s.candidate
            ? "Ready to request"
            : s.matchState === "ambiguous"
              ? "Review match"
              : scanning
                ? "Waiting for scan"
                : "Unresolved";
}

export function recordingAction(s: Source) {
  if (s.serverId) return "View song";
  if (s.candidate) return "Review recording";
  return s.matchState === "ambiguous" ? "Review match" : "Find recording";
}

export function songGuidance(s: Source) {
  if (s.serverId)
    return "Already available in Navidrome. Publish available songs to include it in your selected playlists.";
  if (s.deferred)
    return "New acquisition is paused for this song. Resume when you want to request it. Its playlist entry is retained; a running download is not cancelled.";
  if (s.requestId)
    return "A request exists for this song. Check Requests for approval and Activity for download progress.";
  if (s.candidate)
    return "A recording was matched. Review it, then request missing audio here or select songs and use Request selected.";
  if (s.metadata.missing)
    return "The export references a song without its metadata. Search by title and artist or paste a Deezer song URL to choose the recording manually.";
  if (s.matchState === "ambiguous")
    return "Search results need your review. Open Review match, compare the album, version and duration, then choose the intended recording.";
  return "No recording has been selected. Use Find recording to search by title and artist or paste a Deezer song URL. You can skip it and return later.";
}
