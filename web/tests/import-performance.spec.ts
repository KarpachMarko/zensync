import { test, expect } from "@playwright/test";
import type { Detail, Source } from "../src/lib/migration";
import type { Track } from "../src/lib/api";

test("3,000-song imports resolve and close without waiting for full-list refreshes", async ({ page }) => {
  await page.clock.install();
  const recording: Track = { id: 42, title: "Recording", title_version: "", duration: 180, artist: { name: "Artist" }, album: { title: "Album" }, link: "https://www.deezer.com/track/42" };
  const songs: Source[] = Array.from({ length: 3000 }, (_, index) => ({
    id: "song-" + index, metadata: { title: "Song " + index.toString().padStart(4,"0"), artist: "Artist", album: "Album", duration: 180, favorite: false, missing: false },
    serverId: "", candidate: index === 1 ? recording : null, matchState: "unresolved", deferred: false, favoriteState: "", requestId: "", decision: "", jobState: "",
  }));
  const detail: Detail = { migration: { id: "migration", name: "Large import", state: "open", favorites: false, targetUser: "", error: "", report: { songs: 3000, playlists: 0, unplaylisted: 3000, missing: 0, excluded: 0, warnings: [] } }, songs, playlists: [] };
  let reads = 0;
  let releaseRefresh = () => {};
  const stalledRefresh = new Promise<void>((resolve) => { releaseRefresh = resolve; });
  let releaseAction = () => {};
  const stalledAction = new Promise<void>((resolve) => { releaseAction = resolve; });
  let markActionStarted = () => {};
  const actionStarted = new Promise<void>((resolve) => { markActionStarted = resolve; });
  let failResolution = true;
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/imports/migration") {
      reads++;
      const snapshot = JSON.stringify(detail);
      if (reads > 1) await stalledRefresh;
      return route.fulfill({ contentType: "application/json", body: snapshot });
    }
    if (path.endsWith("/candidates")) return route.fulfill({ json: { deezer: [recording], library: [{ id: "local", title: "Local recording", artist: "Artist", album: "Album", duration: 180 }] } });
    const match = /\/songs\/song-(\d+)\/(defer|resume|resolve|acquire)$/.exec(path);
    if (match) {
      const song = songs[Number(match[1])];
      if (song.id === "song-2" && match[2] === "defer") {
        markActionStarted();
        await stalledAction;
      }
      if (match[2] === "defer") song.deferred = true;
      if (match[2] === "resume") song.deferred = false;
      if (match[2] === "resolve") {
        if (failResolution) return route.fulfill({ status: 409, json: { error: "Recording could not be selected" } });
        const body: { serverId?: string } = route.request().postDataJSON();
        song.serverId = body.serverId ?? "";
        song.candidate = body.serverId ? null : recording;
        song.matchState = "manual";
      }
      if (match[2] === "acquire") { song.requestId = "request"; song.decision = "awaiting_approval"; }
      return route.fulfill({ json: { ok: true, source: song } });
    }
    throw new Error("Unexpected API: " + path);
  });
  try {
    await page.goto("/imports/migration");
    const first = page.locator('[data-slot="import-song"]').filter({ hasText: "Song 0000" });
    await first.getByRole("button", { name: "Find recording", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Song 0000", exact: true });
    await dialog.getByRole("button", { name: "Skip for now", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(first.getByText("Skipped for now", { exact: true })).toBeVisible();
    await expect(page.locator("[data-sonner-toast]").filter({ hasText: "Song updated." }).first()).toBeVisible();
    expect(reads).toBe(1);
    await first.getByRole("button", { name: "Find recording", exact: true }).click();
    await dialog.getByRole("button", { name: "Resume", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(first.getByText("Unresolved", { exact: true })).toBeVisible();
    await first.getByRole("button", { name: "Find recording", exact: true }).click();
    await dialog.getByRole("button", { name: "Use this recording", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText("Recording could not be selected");
    failResolution = false;
    await dialog.getByRole("button", { name: "Use this recording", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(first.getByText("Ready to request", { exact: true })).toBeVisible();
    await first.getByRole("button", { name: "Review recording", exact: true }).click();
    await dialog.getByRole("button", { name: "Use existing", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(first).toHaveCount(0); // Available remains hidden by default.
    const second = page.locator('[data-slot="import-song"]').filter({ hasText: "Song 0001" });
    await second.getByRole("button", { name: "Review recording", exact: true }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Request missing audio", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(second.getByText("awaiting approval", { exact: true })).toBeVisible();
    // Start a poll during an action. Its older response must be cancelled
    // before applying the saved row, so it cannot undo the user's decision.
    const third = page.locator('[data-slot="import-song"]').filter({ hasText: "Song 0002" });
    await third.getByRole("button", { name: "Find recording", exact: true }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Skip for now", exact: true }).click();
    await actionStarted;
    await page.clock.fastForward(4100);
    await expect.poll(() => reads).toBeGreaterThan(1);
    const cancelled = page.waitForEvent("requestfailed", (request) => new URL(request.url()).pathname === "/api/v2/imports/migration");
    releaseAction();
    await cancelled;
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(third.getByText("Skipped for now", { exact: true })).toBeVisible();
  } finally {
    releaseAction();
    releaseRefresh();
  }
});
