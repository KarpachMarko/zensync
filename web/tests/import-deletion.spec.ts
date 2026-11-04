import { test, expect } from "@playwright/test";
import type { Detail } from "../src/lib/migration";

for (const state of ["staged", "open"]) {
  test(`delete ${state} migration from its detail page and allow reimport`, async ({ page }) => {
    let deleted = false;
    let fail = true;
    let deletes = 0;
    const detail: Detail = {
      migration: {
        id: "delete-me", name: "Unfinished playlist", state, favorites: false,
        targetUser: "", error: "",
        report: { songs: 0, playlists: 0, unplaylisted: 0, missing: 0, excluded: 0, warnings: [] },
      },
      songs: [], playlists: [],
    };
    await page.route("**/api/v2/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
      if (path.endsWith("/session")) return route.fulfill({ json: {
        csrf: "fixture",
        user: { id: "user", username: "user", role: "member", direct: false, directDelete: false, quality: ["128"] },
      } });
      if (path === "/api/v2/imports") return route.fulfill({ json: deleted ? [] : [detail.migration] });
      if (path === "/api/v2/imports/delete-me") {
        if (route.request().method() === "DELETE") {
          deletes++;
          if (fail) return route.fulfill({ status: 500, json: { error: "Unable to delete; try again" } });
          deleted = true;
          return route.fulfill({ json: { ok: true } });
        }
        return deleted
          ? route.fulfill({ status: 404, json: { error: "Migration not found" } })
          : route.fulfill({ json: detail });
      }
      throw new Error("Unexpected API: " + path);
    });
    await page.goto("/imports/delete-me");
    const trigger = page.getByRole("button", { name: "Delete migration: Unfinished playlist", exact: true });
    await trigger.click();
    const dialog = page.getByRole("alertdialog", { name: "Delete this migration?" });
    await expect(dialog).toContainText("Existing download requests will remain");
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    expect(deletes).toBe(0);
    await trigger.click();
    await dialog.getByRole("button", { name: "Delete migration", exact: true }).click();
    await expect(dialog.getByRole("alert")).toHaveText("Unable to delete; try again");
    await expect(dialog).toBeVisible();
    fail = false;
    await dialog.getByRole("button", { name: "Delete migration", exact: true }).click();
    await expect(page).toHaveURL(/\/imports$/);
    await expect(page.getByText("No imports in progress.")).toBeVisible();
    expect(deletes).toBe(2);
    await page.getByRole("button", { name: "New migration", exact: true }).click();
    await expect(page.getByRole("dialog", { name: "New migration", exact: true })).toBeVisible();
  });
}

test("imports list offers deletion only for unfinished migrations", async ({ page }) => {
  let removed = false;
  const migrations = [
    { id: "preview", name: "Preview import", state: "staged", report: { songs: 1, playlists: 1 } },
    { id: "active", name: "Active import", state: "open", report: { songs: 1, playlists: 1 } },
    { id: "done", name: "Finished import", state: "handed_over", report: { songs: 1, playlists: 1 } },
  ];
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: {
      csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] },
    } });
    if (path === "/api/v2/imports") return route.fulfill({ json: migrations.filter((migration) => !removed || migration.id !== "preview") });
    if (path === "/api/v2/imports/preview" && route.request().method() === "DELETE") {
      removed = true;
      return route.fulfill({ json: { ok: true } });
    }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/imports");
  await expect(page.getByRole("button", { name: "Delete migration: Active import", exact: true })).toBeVisible();
  await page.getByRole("button", { name: /^Completed imports/ }).click();
  await expect(page.getByRole("link", { name: /Finished import/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Delete migration: Finished import", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Delete migration: Preview import", exact: true }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Delete migration", exact: true }).click();
  await expect(page.getByRole("link", { name: /Preview import/ })).toHaveCount(0);
  await expect(page.getByRole("link", { name: /Active import/ })).toBeVisible();
});
