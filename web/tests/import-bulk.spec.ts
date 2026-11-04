import { test, expect } from "@playwright/test";
import type { Detail, Source } from "../src/lib/migration";

test("hundreds of selected songs use one bulk POST per attempt and keep a retry key", async ({ page }) => {
  const recording = { id: 42, title: "Recording", title_version: "", duration: 180, artist: { name: "Artist" }, album: { title: "Album" } };
  const songs: Source[] = Array.from({ length: 500 }, (_, index) => ({
    id: `song-${index}`, metadata: { title: `Song ${index}`, artist: "Artist", album: "Album", duration: 180, favorite: false, missing: false },
    serverId: "", candidate: recording, matchState: "confident", deferred: false, favoriteState: "", requestId: "", decision: "", jobState: "",
  }));
  const detail: Detail = { migration: { id: "migration", name: "Large import", state: "open", favorites: false, targetUser: "", error: "", report: { songs: 500, playlists: 0, unplaylisted: 500, missing: 0, excluded: 0, warnings: [] } }, songs, playlists: [] };
  const keys: string[] = [];
  let fail = true;
  let reads = 0;
  let release = () => {};
  const refresh = new Promise<void>((resolve) => { release = resolve; });
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/imports/migration") {
      reads++;
      if (reads > 1) await refresh;
      return route.fulfill({ json: detail });
    }
    if (path === "/api/v2/activity") return route.fulfill({ json: [] });
    if (path === "/api/v2/imports/migration/acquire") {
      const body: { sourceIds: string[] } = route.request().postDataJSON();
      expect(body.sourceIds.length).toBe(500);
      expect(new Set(body.sourceIds).size).toBe(500);
      const key = route.request().headers()["idempotency-key"];
      expect(key.length).toBeGreaterThanOrEqual(8);
      keys.push(key);
      if (fail) return route.fulfill({ status: 503, json: { error: "Could not save requests" } });
      for (const song of songs) { song.requestId = `request-${song.id}`; song.decision = "awaiting_approval"; }
      return route.fulfill({ status: 201, json: { bulk: { created: 500, existing: 0, skipped: 0 } } });
    }
    throw new Error("Unexpected API (individual song requests are forbidden here): " + path);
  });
  try {
    await page.goto("/imports/migration");
    await page.getByRole("checkbox", { name: "Select ready songs", exact: true }).check();
    await page.getByRole("button", { name: "Request selected (500)", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("Could not save requests");
    fail = false;
    await page.getByRole("button", { name: "Request selected (500)", exact: true }).click();
    await expect(page.getByText(/500 song requests saved\./)).toBeVisible();
    expect(keys).toHaveLength(2);
    expect(keys[0]).toBe(keys[1]);
    // Confirmation must not wait for the full catalog refresh, and no further
    // browser-driven requests should be needed after leaving this page.
    await page.goto("/activity");
    release();
    await page.goto("/imports/migration");
    await expect(page.getByRole("checkbox", { name: "awaiting approval (500)", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Request selected (0)", exact: true })).toBeDisabled();
    expect(keys).toHaveLength(2);
  } finally { release(); }
});
