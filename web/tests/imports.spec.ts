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
  const candidateQueries: string[] = [];
  let failSearch = true;
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
    if (path.endsWith("/candidates")) {
      const body: { query: string } = route.request().postDataJSON();
      candidateQueries.push(body.query);
      if (body.query === "local only") return route.fulfill({ json: {
        library: [{ id: "local", title: "Local recording", artist: "Artist", album: "Other album", duration: 210 }],
        libraryMatches: { local: { confident: false, reasons: ["Title differs", "Album differs", "Duration differs (3:00 vs 3:30)"] } },
        deezer: [], deezerError: "Deezer unavailable", libraryError: "",
      } });
      if (body.query === "local exact") return route.fulfill({ json: {
        library: [{ id: "local", title: "Unresolved song", artist: "Artist", album: "Album", duration: 180 }],
        libraryMatches: { local: { confident: true, reasons: [] } }, deezer: [],
      } });
      if (body.query === "remote only") return route.fulfill({ json: {
        library: [], deezer: [recording], libraryError: "Navidrome unavailable", deezerError: "",
      } });
      if (body.query === "alternative" && failSearch)
        return route.fulfill({ status: 502, json: { error: "Search unavailable" } });
      return route.fulfill({ json: { library: [], deezer: [recording], deezerMatches: { 42: { confident: false, reasons: ["Artist differs", "Duration differs (3:00 vs 3:30)"], warnings: ["The public Deezer API reports limited availability. Playback and acquisition may differ by account or region."] } } } });
    }
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
  await expect(available).toHaveCount(0);
  await page.getByRole("checkbox", { name: "Available (1)", exact: true }).check();
  await expect(available).toBeVisible();
  await expect(available.getByRole("button", { name: "Skip for now" })).toHaveCount(0);
  await expect(available.getByRole("button", { name: "View song" })).toBeVisible();
  await expect(ready.getByRole("button", { name: "Review recording" })).toBeVisible();
  await expect(ambiguous.getByRole("button", { name: "Review match" })).toBeVisible();
  await page.getByRole("button", { name: "Scan again" }).click();
  expect(rescans).toBe(1);

  await unresolved.getByRole("button", { name: "Skip for now" }).click();
  await expect(unresolved.getByText("Skipped for now", { exact: true })).toBeVisible();
  await unresolved.getByRole("button", { name: "Resume" }).click();
  await expect(unresolved.getByText("Unresolved", { exact: true })).toBeVisible();
  await page.getByPlaceholder("Title, artist or status").fill("Ambiguous song");
  const reviewButton = ambiguous.getByRole("button", { name: "Review match" });
  await reviewButton.click();
  const dialog = page.getByRole("dialog", { name: "Ambiguous song", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Use this recording" })).toBeVisible();
  await expect(dialog.getByText("Artist differs · Duration differs (3:00 vs 3:30)", { exact: true })).toBeVisible();
  await expect(dialog.getByText(/Playback and acquisition may differ by account or region/)).toBeVisible();
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "Use this recording" })).toBeEnabled();
  expect(candidateQueries).toEqual(["Artist Ambiguous song"]);
  expect(new URL(page.url()).pathname).toBe("/imports/migration");
  expect(new URL(page.url()).searchParams.get("recording")).toBe("Ambiguous song");
  const bounds = await dialog.boundingBox();
  expect(bounds?.width).toBeGreaterThan(1000);
  const header = await dialog.locator("header").boundingBox();
  expect(header?.height).toBeLessThan(100);
  const footer = dialog.locator("footer");
  await expect(footer.getByRole("button", { name: "Skip for now" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(reviewButton).toBeFocused();
  await expect(page.getByPlaceholder("Title, artist or status")).toHaveValue("Ambiguous song");
  await page.getByPlaceholder("Title, artist or status").fill("");

  await unresolved.getByRole("button", { name: "Find recording" }).click();
  const unresolvedDialog = page.getByRole("dialog", { name: "Unresolved song", exact: true });
  await expect(unresolvedDialog.getByRole("button", { name: "Use this recording" })).toBeVisible();
  expect(candidateQueries).toContain("Artist Unresolved song");
  await unresolvedDialog.getByLabel("Search or provider song URL").fill("local only");
  await unresolvedDialog.getByRole("button", { name: "Search", exact: true }).click();
  const localResults = unresolvedDialog.getByRole("region", { name: /^Already in Navidrome/ });
  const deezerResults = unresolvedDialog.getByRole("region", { name: /^Provider recordings/ });
  await expect(localResults.getByText("Local recording", { exact: true })).toBeVisible();
  await expect(localResults.getByText("Title differs · Album differs · Duration differs (3:00 vs 3:30)", { exact: true })).toBeVisible();
  await expect(localResults.getByRole("button", { name: "Use existing" })).toBeEnabled();
  await expect(deezerResults.getByRole("alert")).toContainText("Deezer search failed");
  await expect(deezerResults.getByText(/No results/)).toHaveCount(0);
  await unresolvedDialog.getByLabel("Search or provider song URL").fill("local exact");
  await unresolvedDialog.getByRole("button", { name: "Search", exact: true }).click();
  await expect(localResults.getByText("Title, artist credits, album and duration match.", { exact: true })).toBeVisible();
  await expect(localResults.getByText(/Title differs/)).toHaveCount(0);
  await unresolvedDialog.getByLabel("Search or provider song URL").fill("remote only");
  await unresolvedDialog.getByRole("button", { name: "Search", exact: true }).click();
  await expect(localResults.getByRole("alert")).toContainText("Library search failed");
  await expect(localResults.getByText("No local candidates.")).toHaveCount(0);
  await expect(deezerResults.getByRole("button", { name: "Use this recording" })).toBeEnabled();
  await unresolvedDialog.getByLabel("Search or provider song URL").fill("alternative");
  await unresolvedDialog.getByRole("button", { name: "Search", exact: true }).click();
  await expect(unresolvedDialog.getByRole("alert")).toContainText("Search unavailable");
  failSearch = false;
  await unresolvedDialog.getByRole("button", { name: "Retry search" }).click();
  await expect(unresolvedDialog.getByRole("alert")).toHaveCount(0);
  await unresolvedDialog.getByRole("button", { name: "Use this recording" }).click();
  await expect(unresolvedDialog).toHaveCount(0);
  await unresolved.getByRole("button", { name: "Review recording", exact: true }).click();
  await expect(unresolvedDialog.getByRole("heading", { name: "Selected recording" })).toBeVisible();
  await expect(unresolvedDialog.getByRole("button", { name: "Request missing audio" })).toBeEnabled();
  await expect.poll(() => new URL(page.url()).searchParams.get("recording")).toBe("Unresolved song");
  await page.reload();
  await expect(unresolvedDialog).toBeVisible();
  await unresolvedDialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(unresolvedDialog).toHaveCount(0);

  await page.goto("/imports/migration");
  await page.getByRole("checkbox", { name: "Available (1)", exact: true }).check();
  await available.getByRole("button", { name: "View song" }).click();
  await expect(page.getByText(/Already available in Navidrome/)).toBeVisible();
  await expect(page.getByRole("dialog").getByRole("button", { name: "Skip for now" })).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(async () => (await page.getByRole("dialog").boundingBox())?.width).toBeLessThanOrEqual(390);
  await expect.poll(async () => (await page.getByRole("dialog").boundingBox())?.height).toBeLessThanOrEqual(844);

  // Old song URLs still open the recording dialog over the migration.
  await page.goto("/imports/migration/songs/Ambiguous%20song");
  await expect(page.getByRole("dialog", { name: "Ambiguous song", exact: true })).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Close", exact: true }).click();
  await expect(page).toHaveURL(/\/imports\/migration$/);
});
