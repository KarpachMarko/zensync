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
