import { expect, test } from "@playwright/test";

test("setup uses Zod validation before requests and displays the shared app icon", async ({
  page,
}) => {
  let configured = false,
    signedIn = false;
  const writes: { path: string; body: unknown }[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) {
      if (route.request().method() === "POST") {
        writes.push({ path, body: route.request().postDataJSON() as unknown });
        configured = true;
        return route.fulfill({ json: { ok: true } });
      }
      return route.fulfill({ json: { required: !configured } });
    }
    if (path.endsWith("/session")) {
      if (route.request().method() === "POST") {
        writes.push({ path, body: route.request().postDataJSON() as unknown });
        signedIn = true;
        return route.fulfill({ json: { csrf: "fixture" } });
      }
      return route.fulfill({
        status: signedIn ? 200 : 401,
        json: signedIn
          ? {
              csrf: "fixture",
              user: {
                id: "admin",
                username: "admin",
                role: "admin",
                direct: true,
                directDelete: true,
                quality: ["lossless", "320", "128"],
              },
            }
          : { error: "Sign in required" },
      });
    }
    if (path.endsWith("/imports")) return route.fulfill({ json: [] });
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/");
  await expect(page.locator('img[src="/icon.svg"]')).toBeVisible();
  await expect(page.locator('link[rel="icon"]')).toHaveAttribute(
    "href",
    "/icon.svg",
  );
  const icon = await page.request.get("/icon.svg");
  expect(icon.status()).toBe(200);
  expect(icon.headers()["content-type"]).toContain("image/svg+xml");
  expect(await icon.text()).toContain("<title>ZenSync</title>");
  const submit = page.getByRole("button", {
    name: "Create administrator",
    exact: true,
  });
  await submit.click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Enter a username." }),
  ).toBeVisible();
  expect(writes).toHaveLength(0);
  const username = page.getByLabel("Username", { exact: true });
  const password = page.getByLabel("Password", { exact: true });
  await username.fill("admin");
  await password.fill("short");
  await submit.click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Use at least 12 characters." }),
  ).toBeVisible();
  expect(writes).toHaveLength(0);
  await password.fill("x".repeat(71) + "é");
  await submit.click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Password is too long." }),
  ).toBeVisible();
  expect(writes).toHaveLength(0);
  await password.fill("a-valid-test-password");
  await submit.click();
  await expect(
    page.getByRole("heading", { name: "Imports", exact: true }),
  ).toBeVisible();
  expect(writes.map((write) => write.path)).toEqual([
    "/api/v2/setup",
    "/api/v2/session",
  ]);
  await expect(
    page.locator('[data-sidebar="header"] img[src="/icon.svg"]'),
  ).toBeVisible();
});

test("XML form validates missing and oversized files before uploading", async ({
  page,
}) => {
  let uploads = 0;
  await page.route("**/api/v2/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup"))
      return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session"))
      return route.fulfill({
        json: {
          csrf: "fixture",
          user: {
            id: "member",
            username: "member",
            role: "member",
            direct: false,
            directDelete: false,
            quality: ["128"],
          },
        },
      });
    if (path.endsWith("/imports") && route.request().method() === "GET")
      return route.fulfill({ json: [] });
    if (path.endsWith("/imports") && route.request().method() === "POST") {
      uploads++;
      return route.fulfill({ json: { id: "unexpected" } });
    }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/imports");
  await page
    .getByRole("button", { name: "New migration", exact: true })
    .click();
  const submit = page.getByRole("button", {
    name: "Upload and preview",
    exact: true,
  });
  await submit.click();
  await expect(page.getByRole("alert")).toHaveText("Choose an XML or TXT export.");
  expect(uploads).toBe(0);
  await page
    .getByLabel("Apple Music or iTunes export (XML or TXT)", { exact: true })
    .setInputFiles({
      name: "oversized.xml",
      mimeType: "application/xml",
      buffer: Buffer.alloc(32 * 1024 * 1024 + 1),
    });
  await submit.click();
  await expect(page.getByRole("alert")).toHaveText(
    "Choose an export smaller than 32 MiB.",
  );
  expect(uploads).toBe(0);
});

test("Imports combines creation, active work, and collapsed history", async ({
  page,
}) => {
  await page.route("**/api/v2/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup"))
      return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session"))
      return route.fulfill({
        json: {
          csrf: "fixture",
          user: {
            id: "member",
            username: "member",
            role: "member",
            direct: false,
            directDelete: false,
            quality: ["128"],
          },
        },
      });
    if (path.endsWith("/imports"))
      return route.fulfill({
        json: [
          {
            id: "active",
            name: "Current library",
            state: "open",
            report: { songs: 4, playlists: 1 },
          },
          {
            id: "preview",
            name: "Awaiting review",
            state: "staged",
            report: { songs: 2, playlists: 0 },
          },
          {
            id: "old",
            name: "Previous library",
            state: "handed_over",
            report: { songs: 9, playlists: 2 },
          },
        ],
      });
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/imports");
  const active = page.getByRole("region", { name: "In progress" });
  await expect(
    active.getByRole("link", { name: /Current library/ }),
  ).toBeVisible();
  await expect(
    active.getByRole("link", { name: /Awaiting review/ }),
  ).toBeVisible();
  const history = page.getByRole("button", { name: /Completed imports/ });
  await expect(history).toHaveAttribute("aria-expanded", "false");
  await expect(
    page.getByRole("link", { name: /Previous library/ }),
  ).not.toBeVisible();
  await history.click();
  await expect(
    page.getByRole("link", { name: /Previous library/ }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "New migration", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "New migration",
    exact: true,
  });
  await expect(
    dialog.getByLabel("Apple Music or iTunes export (XML or TXT)"),
  ).toBeVisible();
  await expect(page).toHaveURL(/\/imports\?new=true$/);
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page).toHaveURL(/\/imports$/);
  await page.goto("/imports/new");
  await expect(dialog).toBeVisible();
  await expect(page).toHaveURL(/\/imports\?new=true$/);
});
