import { test, expect, type Page } from "@playwright/test";
import type { LibrarySong } from "../src/lib/api";

const source: LibrarySong = { id: "source", title: "Original song", artist: "Artist", album: "Original album", duration: 90, suffix: "flac", bitRate: 900, coverArt: "original-cover" };
const compatible: LibrarySong = { ...source, id: "kept", title: "Kept recording", album: "Kept album", suffix: "flac", coverArt: "kept-cover" };
const incompatible: LibrarySong = { ...compatible, id: "mp3", title: "MP3 recording", suffix: "mp3", bitRate: 128 };
function audio() {
  const rate = 8000, size = rate * 90 * 2;
  const body = Buffer.alloc(44 + size);
  body.write("RIFF"); body.writeUInt32LE(body.length - 8, 4); body.write("WAVE", 8);
  body.write("fmt ", 12); body.writeUInt32LE(16, 16); body.writeUInt16LE(1, 20); body.writeUInt16LE(1, 22);
  body.writeUInt32LE(rate, 24); body.writeUInt32LE(rate * 2, 28); body.writeUInt16LE(2, 32); body.writeUInt16LE(16, 34);
  body.write("data", 36); body.writeUInt32LE(size, 40); return body;
}
async function fixture(page: Page, admin = true) {
  const merges: { kind: string; source: string; target?: string; providerTarget?: string; uploadId?: string; allowFormatChange?: boolean }[] = [];
  const streams: string[] = [];
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64");
  await page.route("**/api/v2/**", async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: admin ? "admin" : "member", quality: ["128"] } } });
    if (path === "/api/v2/search") return route.fulfill({ json: [{ id: url.searchParams.get("provider") === "lossless" ? 2 : 1, provider: url.searchParams.get("provider"), source: "fixture", title: "Provider song", title_version: "", artist: { name: "Provider artist" }, album: { id: 1, title: "Provider album", cover_medium: "" }, duration: 90 }] });
    if (path === "/api/v2/metadata/replacement-upload") return route.fulfill({ status: 201, json: { id: "upload-fixture" } });
    if (path === "/api/v2/metadata/replacement-inputs/upload-fixture") return route.fulfill({ json: { id: "upload-fixture", format: "mp3", track: { id: 0, title: "Uploaded recording", artist: { name: "Upload artist" }, album: { title: "Upload album" }, duration: 90 } } });
    if (/\/providers\/[^/]+\/stream$/.test(path) || path.endsWith("/upload-fixture/audio")) return route.fulfill({ contentType: "audio/wav", body: audio() });
    if (path === "/api/v2/metadata/jobs") return route.fulfill({ json: [] });
    if (path === "/api/v2/library") return route.fulfill({ json: { songs: url.searchParams.get("q") ? [compatible, incompatible] : [source], nextOffset: -1 } });
    if (path === "/api/v2/library/browse") {
      const kind = url.searchParams.get("view") === "artists" ? "artist" : "album";
      const id = url.searchParams.get("id");
      return route.fulfill({ json: { collections: id ? [] : [{ id: `${kind}-source`, name: "Source collection", songCount: 2 }, { id: `${kind}-destination`, name: "Destination collection", artist: "Artist", songCount: 2, coverArt: "kept-cover", duration: 180, year: 2024 }], songs: id ? [compatible, incompatible] : [], name: id ? "Destination collection" : "", nextOffset: -1 } });
    }
    if (path === "/api/v2/library/cover") return route.fulfill({ contentType: "image/png", body: png });
    if (path === "/api/v2/library/stream") {
      streams.push(url.searchParams.get("id") ?? "");
      const body = audio(), range = /^bytes=(\d+)-(\d*)$/.exec(request.headers().range ?? "");
      if (range) {
        const start = Number(range[1]), end = range[2] ? Math.min(Number(range[2]), body.length - 1) : body.length - 1;
        return route.fulfill({ status: 206, contentType: "audio/wav", headers: { "Accept-Ranges": "bytes", "Content-Range": `bytes ${start}-${end}/${body.length}` }, body: body.subarray(start, end + 1) });
      }
      return route.fulfill({ contentType: "audio/wav", body });
    }
    if (path === "/api/v2/metadata/merge-compatibility") return route.fulfill({ json: url.searchParams.get("target") === "mp3"
      ? { compatible: true, formatChange: true, sourceFormat: "flac", targetFormat: "mp3", warning: "The recording will change from FLAC to MP3. No transcoding is performed." }
      : { compatible: true, sourceFormat: "flac", targetFormat: "flac" } });
    if (path === "/api/v2/metadata/merge") { merges.push(request.postDataJSON() as { kind: string; source: string; target: string }); return route.fulfill({ status: 202, json: { songs: 1, id: "job" } }); }
    throw new Error("Unexpected API: " + path);
  });
  return { merges, streams };
}

test("song replacement reviews metadata, plays candidates and warns about format changes", async ({ page }) => {
  const { merges, streams } = await fixture(page);
  await page.goto("/library/songs");
  await page.getByRole("button", { name: "Play library recording: Original song", exact: true }).click();
  const libraryAudio = await page.getByLabel("Library stream player", { exact: true }).elementHandle();
  await page.getByRole("button", { name: "Merge/Replace: Original song", exact: true }).click();
  expect(await libraryAudio?.evaluate((node: HTMLAudioElement) => node.paused && !node.getAttribute("src"))).toBe(true);
  const dialog = page.getByRole("dialog", { name: "Merge/Replace", exact: true });
  await expect(dialog.getByLabel("Find destination song", { exact: true })).toHaveValue(source.title);
  await expect(dialog.getByRole("region", { name: "Merge source" })).toContainText("Original album");
  await dialog.getByLabel("Find destination song", { exact: true }).fill("recording");
  await dialog.getByRole("button", { name: "Search", exact: true }).click();
  const candidates = dialog.getByRole("region", { name: "Merge candidates", exact: true });
  const kept = candidates.locator("article").filter({ hasText: "Kept recording" });
  await expect(kept).toContainText("Artist / Kept album / 1:30");
  await expect(kept).toContainText("FLAC");
  await expect.poll(() => kept.locator("img").evaluate((node: HTMLImageElement) => node.naturalWidth)).toBeGreaterThan(0);
  await kept.getByRole("button", { name: "Play library recording: Kept recording", exact: true }).click();
  const player = dialog.getByLabel("Library stream player", { exact: true });
  await expect.poll(() => player.evaluate((node: HTMLAudioElement) => node.paused)).toBe(false);
  await player.evaluate((node: HTMLAudioElement) => { node.currentTime = 45; node.dispatchEvent(new Event("timeupdate")); });
  await expect.poll(() => player.evaluate((node: HTMLAudioElement) => node.paused)).toBe(false);
  expect(streams).toContain("kept"); expect(merges).toHaveLength(0);
  await dialog.getByRole("button", { name: "Select song: MP3 recording", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("change from FLAC to MP3");
  await expect(dialog.getByRole("button", { name: "Confirm Merge/Replace", exact: true })).toBeEnabled();
  expect(merges).toHaveLength(0);
  await kept.getByRole("button", { name: "Select song: Kept recording", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect(dialog.getByRole("region", { name: "Selected destination", exact: true })).toContainText("Kept album");
  await expect(candidates.getByText("Selected destination", { exact: true })).toHaveCount(0);
  await expect(kept.getByRole("button", { name: "Select song: Kept recording", exact: true })).toHaveText("Selected");
  const handle = await player.elementHandle();
  await dialog.getByRole("button", { name: "Confirm Merge/Replace", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(merges).toEqual([{ kind: "song", source: "source", target: "kept" }]);
  expect(await handle?.evaluate((node: HTMLAudioElement) => node.paused && !node.getAttribute("src"))).toBe(true);
});

for (const kind of ["album", "artist"] as const) test(`${kind} merge shows destination recordings and supports listening on mobile`, async ({ page }) => {
  const { merges } = await fixture(page);
  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto(`/library/${kind}s`);
  await page.getByRole("button", { name: `Merge ${kind}: Source collection`, exact: true }).click();
  const dialog = page.getByRole("dialog", { name: `Merge ${kind}`, exact: true });
  await dialog.getByLabel(`Find destination ${kind}`, { exact: true }).fill("Destination");
  await dialog.getByRole("button", { name: "Search", exact: true }).click();
  await dialog.getByRole("button", { name: `Select ${kind}: Destination collection`, exact: true }).click();
  const recordings = dialog.getByRole("region", { name: "Destination recordings", exact: true });
  await expect(recordings.getByText("Kept recording", { exact: true })).toBeVisible();
  await recordings.getByRole("button", { name: "Play library recording: Kept recording", exact: true }).click();
  const audio = dialog.getByLabel("Library stream player", { exact: true });
  await expect.poll(() => audio.evaluate((node: HTMLAudioElement) => node.paused)).toBe(false);
  expect(await dialog.evaluate(node => node.scrollWidth - node.clientWidth)).toBeLessThanOrEqual(1);
  const handle = await audio.elementHandle();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(await handle?.evaluate((node: HTMLAudioElement) => node.paused && !node.getAttribute("src"))).toBe(true);
  expect(merges).toHaveLength(0);
});

test("members can listen to existing library songs without metadata permissions or requests", async ({ page }) => {
  const { merges, streams } = await fixture(page, false);
  await page.goto("/library/songs");
  await page.getByRole("button", { name: "Play library recording: Original song", exact: true }).click();
  const audio = page.getByLabel("Library stream player", { exact: true });
  await expect.poll(() => audio.evaluate((node: HTMLAudioElement) => node.paused)).toBe(false);
  expect(streams).toContain("source"); expect(merges).toHaveLength(0);
  const handle = await audio.elementHandle();
  await page.getByRole("button", { name: "Close player", exact: true }).click();
  expect(await handle?.evaluate((node: HTMLAudioElement) => node.paused && !node.getAttribute("src"))).toBe(true);
});


test("library replacement direction can be flipped", async ({ page }) => {
  const { merges } = await fixture(page);
  await page.goto("/library/songs");
  await page.getByRole("button", { name: "Merge/Replace: Original song", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Merge/Replace", exact: true });
  await dialog.getByRole("button", { name: "Select song: Kept recording", exact: true }).click();
  await dialog.getByRole("button", { name: "Flip merge direction", exact: true }).click();
  await expect(dialog.getByRole("region", { name: "Merge source", exact: true })).toContainText("Kept recording");
  await expect(dialog.getByRole("region", { name: "Selected destination", exact: true })).toContainText("Original song");
  await expect(dialog.getByRole("button", { name: "Confirm Merge/Replace", exact: true })).toBeEnabled();
  await dialog.getByRole("button", { name: "Confirm Merge/Replace", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(merges).toEqual([{ kind: "song", source: "kept", target: "source" }]);
});

for (const origin of ["provider", "upload"] as const) test(`${origin} recordings can be reviewed, played and selected for replacement`, async ({ page }) => {
  const { merges } = await fixture(page);
  await page.goto("/library/songs");
  await page.getByRole("button", { name: "Merge/Replace: Original song", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Merge/Replace", exact: true });
  if (origin === "provider") {
    const searches: string[] = [];
    page.on("request", request => { const url = new URL(request.url()); if (url.pathname === "/api/v2/search") searches.push(url.searchParams.get("provider") ?? ""); });
    await dialog.getByRole("tab", { name: "Providers", exact: true }).click();
    await expect(dialog.getByRole("tab", { name: "Providers", exact: true })).toHaveAttribute("aria-selected", "true");
    await dialog.getByRole("button", { name: "Select provider recording: Provider song", exact: true }).first().click();
    await expect.poll(() => new Set(searches).size).toBe(3);
    await dialog.getByRole("region", { name: "Selected destination", exact: true }).getByRole("button", { name: "Play stream of Provider song", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText("native audio format");
  } else {
    await dialog.getByRole("tab", { name: "Upload file", exact: true }).click();
    await dialog.getByLabel("Replacement audio file", { exact: true }).setInputFiles({ name: "recording.mp3", mimeType: "audio/mpeg", buffer: audio() });
    await dialog.getByRole("button", { name: "Upload and select", exact: true }).click();
    const selected = dialog.getByRole("region", { name: "Selected destination", exact: true });
    await expect(selected).toContainText("Upload artist / Upload album / 1:30");
    await selected.getByRole("button", { name: "Play library recording: Uploaded recording", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText("MP3");
  }
  await expect.poll(() => dialog.locator("audio").evaluate((node: HTMLAudioElement) => node.paused)).toBe(false);
  expect(merges).toHaveLength(0);
  await dialog.getByRole("button", { name: "Confirm Merge/Replace", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(merges).toHaveLength(1);
  expect(merges[0]).toMatchObject({ kind: "song", source: "source", allowFormatChange: true, ...(origin === "provider" ? { providerTarget: "deezer:track:1" } : { uploadId: "upload-fixture" }) });
});

test("docked player follows expanded and collapsed sidebar and fills mobile width", async ({ page }) => {
  await fixture(page, false);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/library/songs");
  await page.getByRole("button", { name: "Play library recording: Original song", exact: true }).click();
  const audio = page.getByLabel("Library stream player", { exact: true });
  const geometry = () => audio.evaluate(node => {
    const dock = node.closest(".fixed")!;
    const rect = dock.getBoundingClientRect();
    return { left: rect.left, right: rect.right, bottom: rect.bottom, width: window.innerWidth, height: window.innerHeight };
  });
  await expect.poll(async () => (await geometry()).left).toBe(208);
  await page.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await expect.poll(async () => (await geometry()).left).toBe(60);
  await page.getByRole("button", { name: "Expand sidebar", exact: true }).click();
  await expect.poll(async () => (await geometry()).left).toBe(208);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(async () => (await geometry()).left).toBe(0);
  const { right, bottom, width, height } = await geometry();
  expect(right).toBe(width); expect(bottom).toBe(height);
});
