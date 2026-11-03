import { test, expect } from "@playwright/test";

test("obsolete migration tasks can be cancelled without affecting acquisition jobs", async ({ page }) => {
  const tasks = [
    { id: "migration-failed", title: "Library.xml / match", state: "failed", stage: "Old connection failure", canCancel: true },
    { id: "migration-queued", title: "Playlist / publish", state: "queued", stage: "Waiting", canCancel: true },
    { id: "download-failed", title: "Song acquisition", state: "failed", stage: "Download failed", canCancel: false },
  ];
  let failCancel = true;
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/activity") return route.fulfill({ json: tasks });
    const match = /^\/api\/v2\/tasks\/(migration-failed|migration-queued)\/cancel$/.exec(path);
    if (match) {
      if (failCancel) return route.fulfill({ status: 503, json: { error: "Unable to cancel; try again" } });
      const task = tasks.find((task) => task.id === match[1]);
      if (!task) throw new Error("Unknown task");
      task.state = "cancelled";
      task.stage = "Cancelled by user";
      task.canCancel = false;
      return route.fulfill({ json: { ok: true } });
    }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/activity");
  const failed = page.locator('[data-slot="list-row"]').filter({ hasText: "Library.xml / match" });
  const download = page.locator('[data-slot="list-row"]').filter({ hasText: "Song acquisition" });
  await expect(download.getByRole("button", { name: "Cancel", exact: true })).toHaveCount(0);
  await failed.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Unable to cancel; try again");
  await expect(failed.getByRole("button", { name: "Cancel", exact: true })).toBeEnabled();
  failCancel = false;
  await failed.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(failed).not.toBeVisible();
  await expect(download.getByRole("button", { name: "Retry", exact: true })).toBeEnabled();
  const queued = page.locator('[data-slot="list-row"]').filter({ hasText: "Playlist / publish" });
  await queued.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(queued).not.toBeVisible();
  await page.getByRole("button", { name: /^Completed/ }).click();
  await expect(failed.getByText("cancelled", { exact: true })).toBeVisible();
  await expect(queued.getByText("cancelled", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Cancel", exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("button", { name: "Cancel", exact: true })).toHaveCount(0);
});
