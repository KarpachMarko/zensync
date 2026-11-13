import { test, expect, type Page } from "@playwright/test";
import type { Detail } from "../src/lib/migration";
import type { Track } from "../src/lib/api";

const lossless: Track = { id: 42, provider: "lossless", source: "main", title: "Best match", title_version: "", duration: 90, artist: { name: "Artist" }, album: { title: "Album" }, link: "" };
const deezer: Track = { id: 43, title: "Different recording", title_version: "", duration: 90, artist: { name: "Other artist" }, album: { title: "Other album" }, link: "" };
const detail: Detail = {
  migration: { id: "migration", name: "Import", state: "open", favorites: false, targetUser: "", error: "", report: { songs: 1, playlists: 0, unplaylisted: 1, missing: 0, excluded: 0, warnings: [] } },
  songs: [{ id: "source", metadata: { title: "Best match", artist: "Artist", album: "Album", duration: 90, favorite: false, missing: false }, serverId: "", candidate: null, matchState: "unresolved", deferred: false, favoriteState: "", requestId: "", decision: "", jobState: "" }], playlists: [],
};
function audioFixture() {
  const rate = 8000, length = rate * 90 * 2;
  const wav = Buffer.alloc(44 + length);
  wav.write("RIFF"); wav.writeUInt32LE(wav.length - 8, 4); wav.write("WAVE", 8);
  wav.write("fmt ", 12); wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20); wav.writeUInt16LE(1, 22);
  wav.writeUInt32LE(rate, 24); wav.writeUInt32LE(rate * 2, 28); wav.writeUInt16LE(2, 32); wav.writeUInt16LE(16, 34);
  wav.write("data", 36); wav.writeUInt32LE(length, 40);
  return wav;
}
async function fixture(page: Page, provider: "deezer" | "lossless" | "hitmo") {
  const streamingTrack: Track = { ...lossless, provider };
  const providerName = provider === "hitmo" ? "Hitmo" : provider === "deezer" ? "Deezer" : "Lossless.am";
  const streams: URL[] = [];
  const searches: string[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["lossless"] } } });
    if (path === "/api/v2/imports/migration") return route.fulfill({ json: detail });
    if (path.endsWith("/candidates")) {
      searches.push(path);
      return route.fulfill({ json: { library: [], deezer: [deezer], providers: provider === "deezer" ? [{ id: "deezer", name: "Deezer", tracks: [streamingTrack, deezer] }] : [
        { id: "deezer", name: "Deezer", tracks: [deezer] }, { id: provider, name: providerName, tracks: [streamingTrack] },
      ], recordings: [streamingTrack, deezer] } });
    }
    if (path === "/api/v2/search") return route.fulfill({ json: { tracks: [streamingTrack], albums: [] } });
    if (path === "/api/v2/library") return route.fulfill({ json: [] });
    if (path === `/api/v2/providers/${provider}/stream`) {
      streams.push(url);
      expect(url.searchParams.get("target")).toBe(provider === "hitmo" ? "hitmo:track:42" : provider === "deezer" ? "deezer:track:42" : "lossless:track:main:42");
      expect(url.searchParams.has("_token")).toBe(false);
      expect(url.searchParams.get("sid")).toMatch(/^[a-zA-Z0-9_-]{8,64}$/);
      const body = audioFixture();
      const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers().range ?? "");
      if (range) {
        const start = Number(range[1]), end = range[2] ? Math.min(Number(range[2]), body.length - 1) : body.length - 1;
        return route.fulfill({ status: 206, contentType: "audio/wav", headers: { "Accept-Ranges": "bytes", "Content-Range": `bytes ${start}-${end}/${body.length}` }, body: body.subarray(start, end + 1) });
      }
      return route.fulfill({ contentType: "audio/wav", headers: { "Accept-Ranges": "bytes" }, body });
    }
    throw new Error("Unexpected API: " + path);
  });
  return { streams, searches };
}

for (const provider of ["lossless", "hitmo", "deezer"] as const) {
const providerName = provider === "hitmo" ? "Hitmo" : provider === "deezer" ? "Deezer" : "Lossless.am";
test(`${provider}: review mixes providers, filters locally and streams beyond 30 seconds`, async ({ page }) => {
  const { streams, searches } = await fixture(page, provider);
  await page.goto("/imports/migration?recording=source");
  const dialog = page.getByRole("dialog", { name: "Best match", exact: true });
  const results = dialog.getByRole("region", { name: /^Provider recordings/ });
  const rows = results.locator('[data-slot="list-row"]');
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText("Best match");
  await expect(rows.first().locator('[data-slot="badge"]')).toHaveText(providerName);
  expect(streams).toHaveLength(0);
  if (provider !== "deezer") {
  const filters = results.getByRole("group", { name: "Filter recording sources" });
  await filters.getByRole("button", { name: "Deezer", exact: true }).click();
  await expect(rows).toHaveCount(provider === "deezer" ? 2 : 1);
  if (provider !== "deezer") await expect(rows.first()).toContainText("Different recording");
  await filters.getByRole("button", { name: "All sources", exact: true }).click();
  await expect(rows.first()).toContainText("Best match");
  await filters.getByRole("button", { name: providerName, exact: true }).click();
  expect(searches).toHaveLength(1);
  await expect(page).toHaveURL(new RegExp(`recordingProvider=${provider}`));
  await page.reload();
  await expect(filters.getByRole("button", { name: providerName, exact: true })).toHaveAttribute("aria-pressed", "true");
  }
  await results.getByRole("button", { name: "Play stream of Best match", exact: true }).click();
  const audio = dialog.getByLabel("Full track stream player", { exact: true });
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.duration)).toBe(90);
  const dock = dialog.getByRole("region", { name: "Preview player", exact: true });
  expect(await dock.evaluate(element => Math.abs((element.closest("div.fixed")?.getBoundingClientRect().bottom ?? 0) - innerHeight))).toBeLessThanOrEqual(1);
  await audio.evaluate((element: HTMLAudioElement) => { element.currentTime = 45; element.dispatchEvent(new Event("timeupdate")); });
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.currentTime)).toBeGreaterThan(44);
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  await results.getByRole("button", { name: "Pause stream of Best match", exact: true }).click();
  await results.getByRole("button", { name: "Play stream of Best match", exact: true }).click();
  expect(new Set(streams.map((url) => url.searchParams.get("sid"))).size).toBe(1);
  const handle = await audio.elementHandle();
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(await handle?.evaluate((element: HTMLAudioElement) => element.paused && element.getAttribute("src") === null)).toBe(true);
});

test(`${provider}: Search streams without creating an acquisition`, async ({ page }) => {
  const { streams } = await fixture(page, provider);
  await page.goto(`/search?q=Song&provider=${provider}`);
  await page.getByRole("button", { name: "Play stream of Best match", exact: true }).click();
  const audio = page.getByLabel("Full track stream player", { exact: true });
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  await expect.poll(() => streams.length).toBeGreaterThan(0);
  await expect(page.getByRole("button", { name: "Request", exact: true })).toBeEnabled();
  const player = page.getByRole("region", { name: "Preview player", exact: true });
  const position = await player.evaluate(element => {
    const dock = element.closest("div.fixed");
    if (!(dock instanceof HTMLElement)) throw new Error("Player is not docked");
    return { position: getComputedStyle(dock).position, bottom: dock.getBoundingClientRect().bottom, viewport: innerHeight };
  });
  expect(position.position).toBe("fixed");
  expect(position.bottom).toBe(position.viewport);
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await expect(player).toBeInViewport();
  await page.getByRole("button", { name: "Close player", exact: true }).click();
  await expect(player).toHaveCount(0);
  await expect(audio).not.toHaveAttribute("src");
});

}
