import type { TransferProgress } from "./api";
export type User = {
  id: string;
  username: string;
  role: string;
  direct: boolean;
  directDelete: boolean;
  quality: string[];
};
export type MusicRequest = {
  jobId?: string;
  transfer?: TransferProgress;
  state: string;
  kind: string;
  detail?: string;
  trackCount?: number;
  items?: {
    title: string;
    artist: string;
    state: string;
    detail: string;
    transfer?: TransferProgress;
  }[];
  id: string;
  owner: string;
  title: string;
  artist: string;
  target: string;
  decision: string;
  reason: string;
  upgrade: boolean;
  quality: string[];
};
