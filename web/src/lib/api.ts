function responseError(data: unknown, fallback: string): string {
  return typeof data === "object" &&
    data !== null &&
    "error" in data &&
    typeof data.error === "string"
    ? data.error
    : fallback;
}

let csrf = "";
export function setCSRF(value: string) {
  csrf = value;
}
export async function api<T = unknown>(
  path: string,
  method = "GET",
  body?: unknown,
  key?: string,
  signal?: AbortSignal,
): Promise<T> {
  const res = await fetch("/api/v2" + path, {
    method,
    signal,
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
      ...(key ? { "Idempotency-Key": key } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data: unknown = await res.json();
  if (!res.ok) throw new Error(responseError(data, "Request failed"));
  return data as T;
}
export async function upload(file: File): Promise<{ id: string }> {
  const res = await fetch("/api/v2/imports", {
    method: "POST",
    headers: {
      "Content-Type": "application/octet-stream",
      "X-CSRF-Token": csrf,
      "X-Import-Name": encodeURIComponent(file.name),
    },
    body: file,
  });
  const data: unknown = await res.json();
  if (!res.ok) throw new Error(responseError(data, "Upload failed"));
  if (
    typeof data !== "object" ||
    data === null ||
    !("id" in data) ||
    typeof data.id !== "string"
  ) {
    throw new Error("Invalid upload response");
  }
  return { id: data.id };
}
export async function uploadAudioFile(path: string, file: File, key: string): Promise<{ id: string }> {
  const response = await fetch("/api/v2" + path, {
    method: "POST",
    headers: {
      "Content-Type": "application/octet-stream",
      "X-CSRF-Token": csrf,
      "X-File-Name": encodeURIComponent(file.name),
      "Idempotency-Key": key,
    },
    body: file,
  });
  const data: unknown = await response.json();
  if (!response.ok) throw new Error(responseError(data, "Audio upload failed"));
  if (typeof data !== "object" || data === null || !("id" in data) || typeof data.id !== "string")
    throw new Error("Invalid audio upload response");
  return { id: data.id };
}

export type Track = {
  provider?: "deezer" | "lossless";
  source?: string;
  id: number;
  title: string;
  title_version: string;
  duration: number;
  artist: { name: string };
  album: { title: string; cover_medium?: string };
  link: string;
  preview?: string;
};
export function trackTarget(t: Track) {
 return t.provider === "lossless" ? `lossless:track:${t.source}:${t.id}` : `deezer:track:${t.id}`;
}
export type LibrarySong = {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
};
export function label(t: Track) {
  return t.title_version &&
    !t.title.toLowerCase().includes(t.title_version.toLowerCase())
    ? t.title + " " + t.title_version
    : t.title;
}
export function time(n: number) {
  return `${Math.floor(n / 60)}:${Math.round(n % 60)
    .toString()
    .padStart(2, "0")}`;
}
// UUID support is not exposed by every browser on plain HTTP LAN deployments.
export function requestKey() {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

export type TransferProgress = {
  percent?: number;
  downloadedBytes: number;
  totalBytes: number;
  bytesPerSecond?: number;
  updatedAt: string;
};
