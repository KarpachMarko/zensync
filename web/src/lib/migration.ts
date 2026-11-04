import type { Track } from "./api";
export type RecordingMatch = {
  confident: boolean;
  reasons: string[];
  warnings?: string[];
};
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
    return "Already available in Navidrome. Publish available songs to add it to your playlists.";
  if (s.deferred)
    return "Acquisition is paused. Resume to request this song; its playlist entry is retained.";
  if (s.requestId)
    return "A request exists. Check Requests for approval or Activity for download progress.";
  if (s.candidate)
    return "Compare the selected recording, then request missing audio when you are ready.";
  if (s.metadata.missing)
    return "The export has no metadata for this song. Search by title and artist or paste a Deezer song URL.";
  if (s.matchState === "ambiguous")
    return "Compare the album, version and duration, then choose the intended recording.";
  return "No recording has been selected. Edit the search or paste a Deezer song URL.";
}
