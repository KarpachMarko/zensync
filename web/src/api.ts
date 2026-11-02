let csrf = "";
export function setCSRF(value: string) {
  csrf = value;
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  key?: string,
): Promise<T> {
  const res = await fetch("/api/v2" + path, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
      ...(key ? { "Idempotency-Key": key } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "Request failed");
  return data;
}
export async function upload(file: File): Promise<{ id: string }> {
  const res = await fetch("/api/v2/imports", {
    method: "POST",
    headers: {
      "Content-Type": "application/xml",
      "X-CSRF-Token": csrf,
      "X-Import-Name": encodeURIComponent(file.name),
    },
    body: file,
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "Upload failed");
  return data;
}
export type Track = {
  id: number;
  title: string;
  title_version: string;
  duration: number;
  artist: { name: string };
  album: { title: string };
  link: string;
};
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
