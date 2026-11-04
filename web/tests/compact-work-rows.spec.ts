import { test, expect } from "@playwright/test";
import type { ActivityItem, MusicRequest } from "../src/lib/types";

for (const mobile of [false, true]) {
  for (const view of ["requests", "activity"] as const) {
    test(`${view} uses compact full-width rows and an accessible actions menu (${mobile ? "phone" : "desktop"})`, async ({ page }) => {
      await page.setViewportSize(mobile ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
      const failure = "Downloaded duration differs from the selected recording (expected 324s, measured 204s). ".repeat(25);
      const title = "Waves (feat. Lantsberg) (Original Mix)";
      const request: MusicRequest = { id: "request", jobId: "job", owner: "user", title, artist: "Ganju", target: "deezer:track:79906978", decision: "approved", reason: "Allowed without approval", state: "failed", detail: failure, kind: "track", upgrade: false, quality: ["lossless"] };
      const activity: ActivityItem = { id: "job", title, state: "failed", stage: failure, canReview: true, canDismiss: true };
      await page.route("**/api/v2/**", async (route) => {
        const path = new URL(route.request().url()).pathname;
        if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
        if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "admin", quality: ["128"] } } });
        if (path === "/api/v2/requests") return route.fulfill({ json: [request] });
        if (path === "/api/v2/activity") return route.fulfill({ json: [activity] });
        throw new Error("Unexpected API: " + path);
      });
      await page.goto("/" + view);
      const row = page.locator(`[data-slot=${view === "requests" ? "request" : "activity-row"}]`);
      await expect(row).toHaveCount(1);
      const height = await row.evaluate((element) => element.getBoundingClientRect().height);
      expect(height).toBeLessThanOrEqual(mobile ? 68 : 56);
      expect(await row.evaluate((element) => Math.abs(element.getBoundingClientRect().width - (element.parentElement?.getBoundingClientRect().width ?? 0)))).toBeLessThan(2);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      await expect(row.getByRole("button")).toHaveCount(1);
      const trigger = row.getByRole("button", { name: `Actions for ${title}`, exact: true });
      await trigger.focus(); await trigger.press("Enter");
      await expect(page.getByRole("menuitem", { name: "Review recording", exact: true })).toBeVisible();
      await expect(page.getByRole("menuitem", { name: "Review downloaded files", exact: true })).toBeVisible();
      if (view === "activity") {
        await expect(page.getByRole("menuitem", { name: "Retry", exact: true })).toBeVisible();
        await expect(page.getByRole("menuitem", { name: "Dismiss failure", exact: true })).toBeVisible();
      }
      await page.getByRole("menuitem", { name: "Show details", exact: true }).click();
      const details = row.getByRole("region", { name: `Details for ${title}`, exact: true });
      await expect(details).toBeVisible();
      await expect(details.getByText(failure, { exact: true })).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      await trigger.click(); await page.getByRole("menuitem", { name: "Hide details", exact: true }).click();
      await expect(details).toHaveCount(0);
      expect(await row.evaluate((element) => element.getBoundingClientRect().height)).toBe(height);
    });
  }
}

test("approval and decline remain available in request actions without an inline form", async ({ page }) => {
  const request: MusicRequest = { id: "request", owner: "user", title: "Awaiting song", artist: "Artist", target: "deezer:track:42", decision: "awaiting_approval", reason: "", state: "awaiting_approval", kind: "track", upgrade: false, quality: ["128"] };
  let declined: unknown;
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "admin", quality: ["128"] } } });
    if (path === "/api/v2/requests") return route.fulfill({ json: [request] });
    if (path === "/api/v2/requests/request") { declined = route.request().postDataJSON(); request.decision = "declined"; request.state = "declined"; return route.fulfill({ json: { ok: true } }); }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/requests");
  await expect(page.locator("[data-slot=request] form")).toHaveCount(0);
  await page.getByRole("button", { name: "Actions for Awaiting song", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Approve", exact: true })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Withdraw", exact: true })).toBeVisible();
  await page.getByRole("menuitem", { name: "Decline", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Decline request", exact: true });
  await expect(dialog).toBeVisible();
  await dialog.getByPlaceholder("Reason for declining", { exact: true }).fill("Another version was chosen");
  await dialog.getByRole("button", { name: "Decline", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(declined).toEqual({ decision: "declined", reason: "Another version was chosen" });
});
