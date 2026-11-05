import { expect, test } from "@playwright/test";

test("creates one labeled key with chosen scopes and expiration, then revokes it", async ({ page }) => {
  type Key = { id: string; label: string; scopes: string[]; expires: number | null; created: string };
  let keys: Key[] = [];
  let created: Omit<Key, "id" | "created"> | undefined;
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { user: { id: "member", username: "member", role: "member", quality: ["320"] }, csrf: "fixture" } });
    if (path === "/api/v2/api-keys") {
      if (method === "POST") {
        created = route.request().postDataJSON() as Omit<Key, "id" | "created">;
        const key = { ...created, id: "key-1", created: "2026-11-05" };
        keys.push(key);
        return route.fulfill({ status: 201, json: { ...key, token: "one-time-secret" } });
      }
      return route.fulfill({ json: keys });
    }
    if (path === "/api/v2/api-keys/key-1" && method === "DELETE") {
      keys = [];
      return route.fulfill({ json: { ok: true } });
    }
    throw new Error(`Unexpected ${method} ${path}`);
  });
  await page.goto("/settings/api");
  await expect(page.getByText("No API keys yet.")).toBeVisible();
  await page.getByRole("button", { name: "Create API key", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Enter a label");
  expect(created).toBeUndefined();
  await page.getByLabel("Label", { exact: true }).fill("My music app");
  await page.getByLabel("Expiration date (optional)").fill("2099-12-31T12:00");
  await page.getByRole("checkbox", { name: "Request song deletion and read its status" }).check();
  await page.getByRole("button", { name: "Create API key", exact: true }).click();
  await expect(page.locator('input[value="one-time-secret"]')).toBeVisible();
  expect(created?.label).toBe("My music app");
  expect(created?.scopes).toEqual(["search:read", "library:read", "requests:read", "requests:create", "deletion:request"]);
  expect(created?.expires).toBeGreaterThan(Date.now() / 1000);
  await page.getByRole("button", { name: "Revoke My music app" }).click();
  await expect(page.getByText("No API keys yet.")).toBeVisible();
  await expect(page.locator('input[value="one-time-secret"]')).toHaveCount(0);
});
