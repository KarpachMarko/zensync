import { z } from "zod";
import type { LibrarySong } from "./api";
export const metadataSchema = z.object({
  title: z.string().trim().min(1).max(500),
  artist: z.string().trim().min(1).max(500),
  album: z.string().max(500),
  albumArtist: z.string().max(500).optional(),
  removeCover: z.boolean().optional(),
  coverUploadId: z.string().max(64).optional(),
  coverUrl: z.union([z.literal(""), z.url().startsWith("https://").max(2000)]),
  lyrics: z.string().max(200000),
  syncedLyrics: z.string().max(200000),
  lyricsSource: z.string().max(500),
});
export type MetadataEdit = z.infer<typeof metadataSchema>;
export type LyricsCandidate = {
  id: number;
  trackName: string;
  artistName: string;
  albumName: string;
  duration: number;
  instrumental: boolean;
  plainLyrics: string;
  syncedLyrics: string;
};
export type CoverCandidate = {
  id: string;
  title: string;
  artist: string;
  url: string;
  source: string;
};
export type MetadataJob = {
  title?: string;
  id: string;
  state: "queued" | "running" | "completed" | "failed";
  message: string;
  done: number;
  total: number;
};
export type DuplicateGroup = {
  kind: "song" | "album" | "artist";
  reason: string;
  members: { id: string; label: string; song?: LibrarySong; count?: number; albumArtists?: { id: string; name: string }[]; artists?: { id: string; name: string; albumArtist: string }[]; tracks?: LibrarySong[] }[];
};
export type CreditIssue = {
  id: string;
  name: string;
  role: "artist" | "album artist";
  parts: string[];
  knownArtists: string[];
  tracks: LibrarySong[];
};
export type DuplicateReport = {
  creditIssues?: CreditIssue[];
  scanned: number;
  limited: boolean;
  groups: DuplicateGroup[];
};
export type LibraryCollection = {
  id: string;
  name: string;
  artist?: string;
  songCount: number;
};
export type LibraryBrowse = {
  collections: LibraryCollection[];
  songs: LibrarySong[];
  name: string;
  nextOffset: number;
};
