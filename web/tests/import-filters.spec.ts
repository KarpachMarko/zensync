import { test, expect } from "@playwright/test";
import type { Detail, Source } from "../src/lib/migration";

const source = (id: string, extra: Partial<Source> = {}): Source => ({
  id, metadata: { title: id, artist: "Artist", album: "Album", duration: 180, favorite: false, missing: false },
  serverId: "", candidate: null, matchState: "unresolved", deferred: false, favoriteState: "", requestId: "", decision: "", jobState: "", ...extra,
});

test("migration status filters hide available by default, persist and preserve playlist positions", async ({ page }) => {
  const detail: Detail = {
    migration: { id: "migration", name: "Migration", state: "open", favorites: false, targetUser: "", error: "", report: { songs: 5, playlists: 1, unplaylisted: 0, missing: 0, excluded: 0, warnings: [] } },
    songs: [
      source("Available song", { serverId: "nav" }),
      source("Ready song", { candidate: { id: 42, title: "Recording", title_version: "", artist: { name: "Artist" }, album: { title: "Album" }, duration: 180, link: "https://www.deezer.com/track/42" } }),
      source("Ambiguous song", { matchState: "ambiguous" }),
      source("Unresolved song"),
      source("Skipped song", { deferred: true }),
    ],
    playlists: [{ id: "playlist", name: "Playlist", selected: true, state: "active", remoteId: "", baseline: "", desired: "", error: "", entries: ["Available song", "Ready song", "Unresolved song", "Ready song", "Available song"], metadata: { folder: false, system: false, smart: false, parentId: "" } }],
  };
  const requested: string[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const path = decodeURIComponent(new URL(route.request().url()).pathname);
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/imports/migration") return route.fulfill({ json: detail });
    if (path.endsWith("/acquire")) {
      requested.push(path);
      return route.fulfill({ json: { id: "request" } });
    }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/imports/migration");
  const rows = page.locator('[data-slot="import-song"]');
  const filters = page.getByRole("group", { name: "Show statuses", exact: true });
  await expect(filters.getByRole("checkbox", { name: "Available (1)", exact: true })).not.toBeChecked();
  await expect(rows).toHaveCount(4);
  await expect(page.getByText("Showing 4 of 5 songs. Available songs are hidden.", { exact: true })).toBeVisible();
  await filters.getByRole("checkbox", { name: "Unresolved (1)", exact: true }).uncheck();
  await filters.getByRole("checkbox", { name: "Review match (1)", exact: true }).uncheck();
  await filters.getByRole("checkbox", { name: "Skipped for now (1)", exact: true }).uncheck();
  await expect(rows).toHaveCount(1);
  await expect(rows).toContainText("Ready song");
  await page.getByRole("checkbox", { name: "Select Ready song", exact: true }).check();
  await expect(page.getByRole("button", { name: "Request selected (1)", exact: true })).toBeEnabled();
  await filters.getByRole("checkbox", { name: "Available (1)", exact: true }).check();
  await expect(page.getByRole("button", { name: "Request selected (0)", exact: true })).toBeDisabled();
  await expect(rows).toHaveCount(2);
  await expect.poll(() => new URL(page.url()).searchParams.get("hideStatus")?.split(",").sort()).toEqual(["Review match", "Skipped for now", "Unresolved"]);
  await page.reload();
  await expect(rows).toHaveCount(2);
  await expect(filters.getByRole("checkbox", { name: "Unresolved (1)", exact: true })).not.toBeChecked();
  await filters.getByRole("checkbox", { name: "Available (1)", exact: true }).uncheck();
  await filters.getByRole("checkbox", { name: "Ready to request (1)", exact: true }).uncheck();
  await expect(rows).toHaveCount(0);
  await expect(page.getByText("No songs match these filters. Change the statuses or clear your search.")).toBeVisible();
  await filters.getByRole("button", { name: "Show all", exact: true }).click();
  await expect(rows).toHaveCount(5);
  await expect.poll(() => new URL(page.url()).searchParams.get("hideStatus")).toBe("");
  await page.reload();
  await expect(rows).toHaveCount(5);
  await filters.getByRole("button", { name: "Hide available", exact: true }).click();
  await expect(rows).toHaveCount(4);
  await page.getByPlaceholder("Title, artist or status").fill("Ambiguous song");
  await expect(rows).toHaveCount(1);
  await expect(rows).toContainText("Ambiguous song");

  await page.goto("/imports/migration/playlists/playlist");
  await expect(rows).toHaveCount(3);
  await expect(page.locator('[data-slot="playlist-position"]')).toHaveText(["2", "3", "4"]);
  await filters.getByRole("checkbox", { name: "Unresolved (1)", exact: true }).uncheck();
  await expect(page.locator('[data-slot="playlist-position"]')).toHaveText(["2", "4"]);
  await expect(filters.getByRole("checkbox", { name: "Ready to request (2)", exact: true })).toBeChecked();
  await page.getByRole("checkbox", { name: "Select ready songs", exact: true }).check();
  await page.getByRole("button", { name: "Request selected (1)", exact: true }).click();
  expect(requested).toEqual(["/api/v2/imports/migration/songs/Ready song/acquire"]);
});
