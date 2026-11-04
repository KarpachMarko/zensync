import { test, expect } from "@playwright/test";
import type { Migration, Source } from "../src/lib/migration";

function context(title: string, artist: string): { migration: Migration; source: Source } {
  return {
    migration: { id: "migration", name: title, state: "open", favorites: false, targetUser: "", error: "", report: { songs: 1, playlists: 0, unplaylisted: 1, missing: 0, excluded: 0, warnings: [] } },
    source: { id: "source", metadata: { title, artist, album: "Album", duration: 324, favorite: false, missing: false }, candidate: null, serverId: "", matchState: "unresolved", deferred: false, favoriteState: "", requestId: "old-request", decision: "approved", jobState: "failed" },
  };
}

function downloadedAudio() {
  const rate = 8000, dataLength = rate * 204 * 2;
  const wav = Buffer.alloc(44 + dataLength);
  wav.write("RIFF"); wav.writeUInt32LE(36 + dataLength, 4); wav.write("WAVE", 8);
  wav.write("fmt ", 12); wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20);
  wav.writeUInt16LE(1, 22); wav.writeUInt32LE(rate, 24); wav.writeUInt32LE(rate * 2, 28);
  wav.writeUInt16LE(2, 32); wav.writeUInt16LE(16, 34); wav.write("data", 36); wav.writeUInt32LE(dataLength, 40);
  return wav;
}

test("failed download can be listened to and its exact duration mismatch explicitly accepted", async ({ page }) => {
  let approved = false;
  const posts: unknown[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const request = route.request(), path = new URL(request.url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["lossless"] } } });
    if (path.endsWith("/recording-review")) return route.fulfill({ json: context("Waves", "Ganju") });
    if (path.endsWith("/candidates")) return route.fulfill({ json: { library: [], deezer: [] } });
    if (path === "/api/v2/activity") return route.fulfill({ json: [{ id: "job", title: "Waves", state: approved ? "queued" : "failed", stage: "Downloaded duration differs", canReview: !approved }] });
    if (path.endsWith("/review-audio/file")) {
      const body = downloadedAudio();
      const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers().range ?? "");
      if (range) {
        const start = Number(range[1]), end = range[2] ? Math.min(Number(range[2]), body.length - 1) : body.length - 1;
        return route.fulfill({ status: 206, contentType: "audio/wav", headers: { "Accept-Ranges": "bytes", "Content-Range": `bytes ${start}-${end}/${body.length}` }, body: body.subarray(start, end + 1) });
      }
      return route.fulfill({ contentType: "audio/wav", headers: { "Accept-Ranges": "bytes" }, body });
    }
    if (path === "/api/v2/tasks/job/review-audio") {
      if (request.method() === "POST") { posts.push(request.postDataJSON()); approved = true; return route.fulfill({ json: { ok: true } }); }
      return route.fulfill({ json: { title: "Waves", artist: "Ganju", stage: "Expected 324s, measured 204s", canAccept: true, reviewURL: "/imports/migration?recording=source", candidates: [{ quality: "lossless", checksum: "exact-file-hash", expected: 324, measured: 204.069 }] } });
    }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/activity");
  await page.getByRole("button", { name: "Actions for Waves", exact: true }).click();
  await page.getByRole("menuitem", { name: "Review downloaded files", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Waves", exact: true });
  await expect(dialog.getByText(/Catalog 5:24 → downloaded 3:24/)).toBeVisible();
  await dialog.getByRole("button", { name: "Listen to downloaded file" }).click();
  const audio = dialog.getByLabel("Downloaded file player", { exact: true });
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  await audio.evaluate((element: HTMLAudioElement) => { element.currentTime = 100; element.dispatchEvent(new Event("timeupdate")); });
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.currentTime)).toBeGreaterThan(90);
  await expect(audio).not.toHaveAttribute("controls");
  expect(posts).toHaveLength(0);
  await dialog.getByRole("button", { name: "Accept duration difference" }).click();
  expect(posts).toHaveLength(0);
  await dialog.getByRole("button", { name: "Accept this file and retry" }).click();
  await expect(dialog).toHaveCount(0);
  expect(posts).toEqual([{ quality: "lossless", checksum: "exact-file-hash" }]);
  await expect(page.getByText("Duration difference accepted for this file.", { exact: false })).toBeVisible();
});

for (const action of ["library", "deezer", "upload"] as const) {
  test(`failed Activity recording opens the import dialog and resolves through ${action} in place`, async ({ page }) => {
    const data = context("Make It Rain", "Ed Sheeran");
    const mutations: { path: string; body: unknown }[] = [];
    let resolved = false;
    let initialSearch = "";
    await page.route("**/api/v2/**", async (route) => {
      const request = route.request(), path = new URL(request.url()).pathname;
      if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
      if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
      if (path === "/api/v2/activity") return route.fulfill({ json: [{ id: "job", title: "Make It Rain", state: resolved ? "superseded" : "failed", stage: "Download failed", canReview: !resolved }] });
      if (path.endsWith("/recording-review")) return route.fulfill({ json: data });
      if (path.endsWith("/review-audio")) return route.fulfill({ json: { title: "Make It Rain", artist: "Ed Sheeran", stage: "Country restriction", canAccept: true, candidates: [] } });
      if (path.endsWith("/candidates")) {
        initialSearch = request.postDataJSON().query;
        return route.fulfill({ json: {
          library: [{ id: "library-song", title: "Make It Rain", artist: "Ed Sheeran", album: "Album", duration: 404 }],
          deezer: [{ id: 43, title: "Make It Rain", title_version: "", artist: { name: "Ed Sheeran" }, album: { title: "Another release" }, duration: 404, link: "https://www.deezer.com/track/43" }],
        } });
      }
      if (path.endsWith("/resolve")) {
        mutations.push({ path, body: request.postDataJSON() });
        data.source.requestId = ""; data.source.decision = ""; data.source.jobState = "";
        if (action === "library") data.source.serverId = "library-song";
        else data.source.candidate = { id: 43, title: "Make It Rain", title_version: "", artist: { name: "Ed Sheeran" }, album: { title: "Another release" }, duration: 404, link: "https://www.deezer.com/track/43" };
        if (action === "library") resolved = true;
        return route.fulfill({ json: { ok: true, source: data.source } });
      }
      if (path.endsWith("/acquire")) {
        mutations.push({ path, body: null }); resolved = true;
        return route.fulfill({ json: { id: "replacement", source: { ...data.source, requestId: "replacement", decision: "awaiting_approval" } } });
      }
      if (path.endsWith("/upload")) {
        expect(request.postDataBuffer()?.length).toBe(2048);
        mutations.push({ path, body: null }); resolved = true;
        return route.fulfill({ status: 201, json: { id: "uploaded", decision: "awaiting_approval" } });
      }
      throw new Error("Unexpected API: " + path);
    });
    if (action === "upload") await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/activity");
    await page.getByRole("button", { name: "Actions for Make It Rain", exact: true }).click();
    await page.getByRole("menuitem", { name: "Review recording", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Make It Rain", exact: true });
    await expect(dialog.getByRole("region", { name: /^Already in Navidrome/ })).toBeVisible();
    await expect(dialog.getByRole("region", { name: /^Deezer recordings/ })).toBeVisible();
    expect(initialSearch).toBe("Ed Sheeran Make It Rain");
    expect(page.url()).toMatch(/\/activity$/);
    expect(await dialog.locator("header").evaluate((header) => header.getBoundingClientRect().height)).toBeLessThan(100);
    await expect(dialog.getByRole("link", { name: /Review match/ })).toHaveCount(0);
    await expect(dialog.getByRole("button", { name: /Accept duration/ })).toHaveCount(0);
    if (action === "library") await dialog.getByRole("button", { name: "Use existing", exact: true }).click();
    if (action === "deezer") await dialog.getByRole("button", { name: "Use and request", exact: true }).click();
    if (action === "upload") {
      await dialog.getByRole("button", { name: "Upload your version", exact: true }).click();
      await dialog.getByLabel("Your audio file", { exact: true }).setInputFiles({ name: "My version.flac", mimeType: "audio/flac", buffer: Buffer.alloc(2048) });
      await dialog.getByRole("button", { name: "Upload and import", exact: true }).click();
    }
    await expect(dialog).toHaveCount(0);
    expect(page.url()).toMatch(/\/activity$/);
    if (action === "library") expect(mutations).toEqual([{ path: "/api/v2/imports/migration/songs/source/resolve", body: { serverId: "library-song" } }]);
    if (action === "deezer") expect(mutations).toEqual([{ path: "/api/v2/imports/migration/songs/source/resolve", body: { target: "deezer:track:43" } }, { path: "/api/v2/imports/migration/songs/source/acquire", body: null }]);
    if (action === "upload") expect(mutations).toEqual([{ path: "/api/v2/imports/migration/songs/source/upload", body: null }]);
  });
}
