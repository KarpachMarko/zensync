import { test, expect } from "@playwright/test";

test("successful resubmission moves an old failure from Blocked into history", async ({ page }) => {
  const tasks = [
    { id: "old", title: "Old Make It Rain attempt", state: "failed", stage: "Provider metadata changed since approval" },
    { id: "new", title: "New Make It Rain attempt", state: "queued", stage: "Queued for acquisition" },
  ];
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/activity") return route.fulfill({ json: tasks });
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/activity");
  const blocked = page.locator('[data-slot="status-section"]').filter({ has: page.getByRole("button", { name: /^Blocked/ }) });
  await expect(blocked.getByText("Old Make It Rain attempt", { exact: true })).toBeVisible();
  tasks[0].state = "superseded";
  tasks[0].stage = "Superseded by a newer successful request. Previous failure: Provider metadata changed since approval";
  tasks[1].state = "available";
  await page.reload();
  await expect(blocked.getByText("Old Make It Rain attempt", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Retry", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: /^Completed/ }).click();
  await expect(page.getByText("Old Make It Rain attempt", { exact: true })).toBeVisible();
  await expect(page.getByText("superseded", { exact: true })).toBeVisible();
  await expect(page.getByText(/Previous failure:/)).toBeVisible();
});

test("legacy orphan failure can be dismissed without retrying or changing music", async ({ page }) => {
  let dismissed = false;
  const mutations: string[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/activity") return route.fulfill({ json: [{ id: "old", title: "Make It Rain", state: dismissed ? "cancelled" : "failed", stage: dismissed ? "Dismissed failure. Previous failure: Country restriction" : "Country restriction", canDismiss: !dismissed }] });
    if (path === "/api/v2/tasks/old/dismiss" && request.method() === "POST") {
      mutations.push(path);
      dismissed = true;
      return route.fulfill({ json: { ok: true } });
    }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/activity");
  await page.getByRole("button", { name: "Dismiss failure", exact: true }).click();
  await expect(page.getByRole("button", { name: "Retry", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Dismiss failure", exact: true })).toHaveCount(0);
  await expect(page.getByText("Failure dismissed. Music files are kept.", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: /^Completed/ }).click();
  await expect(page.getByText("Make It Rain", { exact: true })).toBeVisible();
  await expect(page.getByText("Dismissed failure. Previous failure: Country restriction", { exact: true })).toBeVisible();
  expect(mutations).toEqual(["/api/v2/tasks/old/dismiss"]);
});
