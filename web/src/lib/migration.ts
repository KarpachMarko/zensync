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
  migration: Migration;
  songs: Source[];
  playlists: Playlist[];
};
export function status(s: Source) {
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
              : "Unresolved";
}
