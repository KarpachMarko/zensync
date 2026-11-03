import { test, expect } from "@playwright/test";
import type { Detail, Source } from "../src/lib/migration";
import type { Track } from "../src/lib/api";

function previewAudio() {
  const rate = 8000;
  const dataLength = rate * 31 * 2;
  const wav = Buffer.alloc(44 + dataLength);
  wav.write("RIFF"); wav.writeUInt32LE(36 + dataLength, 4); wav.write("WAVE", 8);
  wav.write("fmt ", 12); wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20);
  wav.writeUInt16LE(1, 22); wav.writeUInt32LE(rate, 24); wav.writeUInt32LE(rate * 2, 28);
  wav.writeUInt16LE(2, 32); wav.writeUInt16LE(16, 34);
  wav.write("data", 36); wav.writeUInt32LE(dataLength, 40);
  return wav;
}

test("review previews play one at a time, stop on close, and link to Deezer", async ({ page }) => {
  const first: Track = { id: 42, title: "First recording", title_version: "", duration: 180, artist: { name: "Artist" }, album: { title: "Album" }, link: "https://www.deezer.com/track/42", preview: "https://cdnt-preview.dzcdn.net/one.wav" };
  const second: Track = { ...first, id: 43, title: "Second recording", preview: "https://cdnt-preview.dzcdn.net/two.wav" };
  const missing: Track = { ...first, id: 44, title: "No clip recording", preview: "" };
  const source: Source = {
    id: "source", metadata: { title: "Source song", artist: "Artist", album: "Album", duration: 180, favorite: false, missing: false },
    serverId: "", candidate: { ...first, preview: undefined }, matchState: "confident", deferred: false, favoriteState: "", requestId: "", decision: "", jobState: "",
  };
  const detail: Detail = {
    migration: { id: "migration", name: "Migration", state: "open", favorites: false, targetUser: "", error: "", report: { songs: 1, playlists: 0, unplaylisted: 1, missing: 0, excluded: 0, warnings: [] } },
    songs: [source], playlists: [],
  };
  let mediaRequests = 0;
  const mutations: string[] = [];
  await page.route("https://cdnt-preview.dzcdn.net/**", async (route) => {
    mediaRequests++;
    const body = previewAudio();
    const range = /^bytes=(\d+)-(\d*)$/.exec(route.request().headers().range ?? "");
    if (range) {
      const start = Number(range[1]);
      const end = range[2] ? Math.min(Number(range[2]), body.length - 1) : body.length - 1;
      await route.fulfill({ status: 206, contentType: "audio/wav", headers: {
        "Accept-Ranges": "bytes",
        "Content-Range": `bytes ${start}-${end}/${body.length}`,
      }, body: body.subarray(start, end + 1) });
    } else {
      await route.fulfill({ contentType: "audio/wav", headers: { "Accept-Ranges": "bytes" }, body });
    }
  });
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/imports/migration") return route.fulfill({ json: detail });
    if (path.endsWith("/candidates")) return route.fulfill({ json: { library: [], deezer: [first, second, missing] } });
    mutations.push(path);
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/imports/migration?recording=source");
  const dialog = page.getByRole("dialog", { name: "Source song", exact: true });
  const results = dialog.getByRole("region", { name: /^Deezer recordings/ });
  const firstRow = results.locator('[data-slot="list-row"]').filter({ hasText: "First recording" });
  const secondRow = results.locator('[data-slot="list-row"]').filter({ hasText: "Second recording" });
  await expect(firstRow.getByRole("button", { name: "Play 30-second preview of First recording", exact: true })).toBeEnabled();
  expect(mediaRequests).toBe(0);
  await expect(results.getByRole("button", { name: "Play 30-second preview of No clip recording", exact: true })).toBeDisabled();
  const link = firstRow.getByRole("link", { name: /Open First recording in Deezer/ });
  await expect(link).toHaveAttribute("href", "https://www.deezer.com/track/42");
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  const selected = dialog.getByRole("heading", { name: "Selected recording" }).locator("..");
  await expect(selected.getByRole("button", { name: "Play 30-second preview of First recording", exact: true })).toBeEnabled();
  await firstRow.getByRole("button", { name: "Play 30-second preview of First recording", exact: true }).click();
  const audio = dialog.locator("audio");
  await expect(audio).toHaveCount(1);
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  await expect(audio).not.toHaveAttribute("controls");
  const player = dialog.getByRole("region", { name: "Preview player", exact: true });
  await expect(player).toBeVisible();
  await player.getByRole("button", { name: "Mute preview", exact: true }).click();
  expect(await audio.evaluate((element: HTMLAudioElement) => element.muted)).toBe(true);
  await player.getByRole("button", { name: "Unmute preview", exact: true }).click();
  const volume = player.getByRole("slider", { name: "Preview volume", exact: true });
  await volume.focus();
  await volume.press("Home");
  expect(await audio.evaluate((element: HTMLAudioElement) => element.volume)).toBe(0);
  await volume.press("End");
  expect(await audio.evaluate((element: HTMLAudioElement) => element.volume)).toBe(1);
  const seek = player.getByRole("slider", { name: "Preview position", exact: true });
  await seek.focus();
  await seek.press("Home");
  await seek.press("ArrowRight");
  expect(await audio.evaluate((element: HTMLAudioElement) => element.currentTime)).toBeGreaterThan(0);
  await expect(selected.getByRole("button", { name: "Pause 30-second preview of First recording", exact: true })).toBeVisible();
  await firstRow.getByRole("button", { name: "Pause 30-second preview of First recording", exact: true }).click();
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(true);
  await secondRow.getByRole("button", { name: "Play 30-second preview of Second recording", exact: true }).click();
  await expect(audio).toHaveAttribute("src", second.preview ?? "");
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.duration)).toBe(31);
  await audio.evaluate((element: HTMLAudioElement) => { element.currentTime = 30; });
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => ({ paused: element.paused, currentTime: element.currentTime, seeking: element.seeking, readyState: element.readyState }))).toMatchObject({ paused: true });
  await audio.dispatchEvent("error");
  await expect(dialog.getByRole("alert")).toContainText("Preview unavailable");
  await secondRow.getByRole("button", { name: "Play 30-second preview of Second recording", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  const savedAudio = await audio.elementHandle();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(await savedAudio?.evaluate((element: HTMLAudioElement) => element.paused)).toBe(true);
  expect(await savedAudio?.getAttribute("src")).toBeNull();
  expect(mutations).toEqual([]);
});
