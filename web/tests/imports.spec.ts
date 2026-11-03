import { test, expect } from "@playwright/test";
import type { Detail, Source } from "../src/lib/migration";

const song = (id: string, extra: Partial<Source> = {}): Source => ({
  id,
  metadata: {
    title: id,
    artist: "Artist",
    album: "Album",
    duration: 180,
    favorite: false,
    missing: false,
  },
  serverId: "",
  candidate: null,
  matchState: "unresolved",
  deferred: false,
  favoriteState: "",
  requestId: "",
  decision: "",
  jobState: "",
  ...extra,
});

const recording = {
  id: 42,
  title: "Ready song",
  title_version: "",
  duration: 180,
  artist: { name: "Artist" },
  album: { title: "Album" },
};

test("commit starts matching and each result has a clear next action", async ({ page }) => {
  const detail: Detail = {
    migration: {
      id: "migration",
      name: "Test migration",
      state: "staged",
      favorites: false,
      targetUser: "",
      error: "",
      report: { songs: 4, playlists: 0, unplaylisted: 4, missing: 0, excluded: 0, warnings: [] },
    },
    songs: [
      song("Available song", { serverId: "nav-song" }),
      song("Ready song", { candidate: recording, matchState: "confident" }),
      song("Ambiguous song", { matchState: "ambiguous" }),
      song("Unresolved song"),
    ],
    playlists: [],
  };
  let rescans = 0;
  await page.route("**/api/v2/**", async (route) => {
    const path = decodeURIComponent(new URL(route.request().url()).pathname);
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: {
      csrf: "fixture",
      user: { id: "user", username: "user", role: "member", direct: false, directDelete: false, quality: ["128"] },
    } });
    if (path === "/api/v2/imports/migration") return route.fulfill({ json: detail });
    if (path.endsWith("/commit")) {
      detail.migration.state = "open";
      detail.matchTask = { state: "queued", message: "" };
      return route.fulfill({ json: { ok: true } });
    }
    if (path.endsWith("/match")) {
      rescans++;
      return route.fulfill({ status: 202, json: { id: "scan" } });
    }
    if (path.endsWith("/defer") || path.endsWith("/resume")) {
      detail.songs[3].deferred = path.endsWith("/defer");
      return route.fulfill({ json: { ok: true } });
    }
    if (path.endsWith("/candidates")) return route.fulfill({ json: { library: [], deezer: [recording] } });
    if (path.endsWith("/resolve")) {
      detail.songs[3].candidate = recording;
      detail.songs[3].matchState = "manual";
      return route.fulfill({ json: { ok: true } });
    }
    throw new Error("Unexpected API: " + path);
  });

  await page.goto("/imports/migration");
  await expect(page.getByText(/automatically starts a scan/)).toBeVisible();
  await page.getByRole("button", { name: "Commit migration" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Match scan queued" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Scanning…" })).toBeDisabled();
  await expect(page.getByText("Waiting for scan", { exact: true })).toBeVisible();
  expect(rescans).toBe(0);

  detail.matchTask = { state: "completed", message: "Finished" };
  await page.reload();
  const available = page.locator('[data-slot="import-song"]').filter({ hasText: "Available song" });
  const ready = page.locator('[data-slot="import-song"]').filter({ hasText: "Ready song" });
  const ambiguous = page.locator('[data-slot="import-song"]').filter({ hasText: "Ambiguous song" });
  const unresolved = page.locator('[data-slot="import-song"]').filter({ hasText: "Unresolved song" });
  await expect(available.getByRole("button", { name: "Skip for now" })).toHaveCount(0);
  await expect(available.getByRole("link", { name: "View song" })).toBeVisible();
  await expect(ready.getByRole("link", { name: "Review recording" })).toBeVisible();
  await expect(ambiguous.getByRole("link", { name: "Review match" })).toBeVisible();
  await page.getByRole("button", { name: "Scan again" }).click();
  expect(rescans).toBe(1);

  await unresolved.getByRole("button", { name: "Skip for now" }).click();
  await expect(unresolved.getByText("Skipped for now", { exact: true })).toBeVisible();
  await unresolved.getByRole("button", { name: "Resume" }).click();
  await expect(unresolved.getByText("Unresolved", { exact: true })).toBeVisible();
  await ambiguous.getByRole("link", { name: "Review match" }).click();
  await expect(page.getByText(/Search results need your review/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Find candidates" })).toBeVisible();

  await page.goto("/imports/migration");
  await unresolved.getByRole("link", { name: "Find recording" }).click();
  await expect(page.getByText(/No recording has been selected/)).toBeVisible();
  await page.getByRole("button", { name: "Find candidates" }).click();
  await page.getByRole("button", { name: "Use this recording" }).click();
  await expect(page.getByRole("heading", { name: "Selected recording" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Request missing audio" })).toBeEnabled();

  await page.goto("/imports/migration");
  await available.getByRole("link", { name: "View song" }).click();
  await expect(page.getByText(/Already available in Navidrome/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Skip for now" })).toHaveCount(0);
});
