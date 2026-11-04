import { expect, test, type Page } from "@playwright/test";

async function settingsFixture(page: Page) {
  let quality = ["lossless", "320", "128"];
  let configured = false;
  let serviceUsername = "";
  let personalUsername = "";
  let deezerConfigured = false;
  const qualityWrites: string[][] = [];
  const connections: unknown[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/setup"))
      return route.fulfill({ json: { required: false } });
    if (url.pathname.endsWith("/session"))
      return route.fulfill({
        json: {
          user: {
            id: "admin",
            username: "admin",
            role: "admin",
            direct: true,
            directDelete: true,
            quality,
          },
          csrf: "fixture",
        },
      });
    if (url.pathname.endsWith("/connections")) {
      if (route.request().method() === "PUT") {
        const body: unknown = route.request().postDataJSON();
        connections.push(body);
        if (typeof body === "object" && body !== null && "kind" in body) {
          if (body.kind === "shared") {
            configured = true;
            if ("username" in body && typeof body.username === "string")
              serviceUsername = body.username;
          }
          if (
            body.kind === "personal" &&
            "username" in body &&
            typeof body.username === "string"
          )
            personalUsername = body.username;
          if (body.kind === "deezer") deezerConfigured = true;
        }
        return route.fulfill({ json: { ok: true } });
      }
      return route.fulfill({
        json: {
          url: configured ? "http://music.local:4533" : "",
          serviceUsername,
          configured,
          personalUsername,
          deezerConfigured,
          downloaderReady: true,
          musicMounted: true,
          downloaderVersion: "2.3.5",
        },
      });
    }
    if (url.pathname.endsWith("/quality")) {
      const body: unknown = route.request().postDataJSON();
      if (
        typeof body !== "object" ||
        body === null ||
        !("quality" in body) ||
        !Array.isArray(body.quality) ||
        !body.quality.every(
          (value: unknown): value is string => typeof value === "string",
        )
      )
        throw new Error("Invalid quality payload");
      quality = [...body.quality];
      qualityWrites.push(quality);
      return route.fulfill({ json: { quality } });
    }
    if (url.pathname.endsWith("/settings/downloads"))
      return route.fulfill({ json: { parallelDownloads: 3 } });
    throw new Error(
      `Unexpected request: ${route.request().method()} ${url.pathname}`,
    );
  });
  return { qualityWrites, connections };
}

test("autosave serializes rapid download edits and rolls back a failed write", async ({
  page,
}) => {
  await settingsFixture(page);
  let stored = 3;
  let fail = false;
  let releaseFirst: () => void = () => {};
  let markStarted: () => void = () => {};
  const firstStarted = new Promise<void>((resolve) => {
    markStarted = resolve;
  });
  const release = new Promise<void>((resolve) => {
    releaseFirst = resolve;
  });
  const writes: number[] = [];
  await page.route("**/api/v2/settings/downloads", async (route) => {
    if (route.request().method() === "GET")
      return route.fulfill({ json: { parallelDownloads: stored } });
    const body: unknown = route.request().postDataJSON();
    if (
      typeof body !== "object" ||
      body === null ||
      !("parallelDownloads" in body) ||
      typeof body.parallelDownloads !== "number"
    )
      throw new Error("Invalid limit payload");
    const value = body.parallelDownloads;
    writes.push(value);
    if (writes.length === 1) {
      markStarted();
      await release;
    }
    if (fail)
      return route.fulfill({
        status: 503,
        json: { error: "Download settings unavailable" },
      });
    stored = value;
    return route.fulfill({ json: { parallelDownloads: stored } });
  });
  await page.goto("/settings/downloads");
  const input = page.getByLabel("Parallel downloads", { exact: true });
  await expect(input).toHaveValue("3");
  await input.fill("12");
  await input.blur();
  await firstStarted;
  await input.fill("19");
  await input.blur();
  expect(writes).toEqual([12]);
  releaseFirst();
  await expect.poll(() => writes).toEqual([12, 19]);
  await expect(
    page
      .locator("[data-sonner-toast]")
      .filter({ hasText: "Download limit saved." }),
  ).toBeVisible();
  await expect(input).toHaveValue("19");
  fail = true;
  await input.fill("20");
  await input.blur();
  await expect(page.getByRole("alert")).toHaveText(
    "Download settings unavailable",
  );
  await expect(input).toHaveValue("19");
  await expect(
    page
      .locator("[data-sonner-toast]")
      .filter({ hasText: "Download settings unavailable" }),
  ).toBeVisible();
  await input.fill("");
  await input.blur();
  await expect(page.getByRole("alert")).toHaveText(
    "Enter a positive whole number.",
  );
  await page.getByRole("tab", { name: "Quality", exact: true }).click();
  expect(writes).toEqual([12, 19, 20]);
  await page.reload();
  await page.getByRole("tab", { name: "Downloads", exact: true }).click();
  await expect(input).toHaveValue("19");
  await expect(page.getByRole("button", { name: /Save download/ })).toHaveCount(
    0,
  );
});

test("connection credentials autosave only as a complete group and clear secrets", async ({
  page,
}) => {
  const { connections } = await settingsFixture(page);
  await page.goto("/settings/connections");
  await page
    .getByLabel("Navidrome URL", { exact: true })
    .fill("http://music.local:4533");
  await page.getByLabel("Service username", { exact: true }).fill("service");
  await page.getByLabel("Service username", { exact: true }).blur();
  expect(connections).toHaveLength(0);
  const password = page.getByLabel("Service password", { exact: true });
  await password.fill("secret-service-password");
  await password.blur();
  await expect(
    page
      .locator("[data-sonner-toast]")
      .filter({ hasText: "Navidrome server verified and saved." }),
  ).toBeVisible();
  expect(connections).toEqual([
    {
      kind: "shared",
      url: "http://music.local:4533",
      username: "service",
      password: "secret-service-password",
    },
  ]);
  await expect(password).toHaveValue("");
  await expect(
    page.getByLabel("Personal password", { exact: true }),
  ).toBeEnabled();
  const arl = page.getByLabel("Deezer ARL cookie", { exact: true });
  await arl.fill("a".repeat(31));
  await arl.blur();
  expect(connections).toHaveLength(1);
  await arl.fill("a".repeat(32));
  await arl.blur();
  await expect(
    page
      .locator("[data-sonner-toast]")
      .filter({ hasText: "Deezer credentials saved." }),
  ).toBeVisible();
  expect(connections.at(-1)).toEqual({ kind: "deezer", arl: "a".repeat(32) });
  await expect(arl).toHaveValue("");
  await expect(page.getByRole("button", { name: /Save|Test and/ })).toHaveCount(
    0,
  );
});

test("quality switches and reorder buttons autosave one FLAC option and persist order", async ({
  page,
}, testInfo) => {
  const { qualityWrites } = await settingsFixture(page);
  await page.goto("/settings/quality");
  const list = page.getByRole("list", { name: "Quality preferences" });
  await expect(
    list.getByRole("switch", { name: "Enable FLAC", exact: true }),
  ).toHaveCount(1);
  await page
    .getByRole("switch", { name: "Enable FLAC", exact: true })
    .uncheck();
  await page
    .getByRole("button", { name: "Move MP3 128 kbps up", exact: true })
    .click();
  await expect.poll(() => qualityWrites.at(-1)).toEqual(["128", "320"]);
  await page.reload();
  await expect(list.locator("li").first()).toHaveAttribute(
    "data-quality",
    "128",
  );
  await page
    .getByRole("switch", { name: "Enable MP3 320 kbps", exact: true })
    .uncheck();
  await expect.poll(() => qualityWrites.at(-1)).toEqual(["128"]);
  await expect(
    page.getByRole("switch", { name: "Enable MP3 128 kbps", exact: true }),
  ).toBeDisabled();
  await page.getByRole("switch", { name: "Enable FLAC", exact: true }).check();
  await page.getByRole("button", { name: "Move FLAC up", exact: true }).click();
  await expect.poll(() => qualityWrites.at(-1)).toEqual(["lossless", "128"]);
  await expect(
    page.getByRole("button", { name: /Save preference/ }),
  ).toHaveCount(0);
  const toastBounds = await page
    .locator("[data-sonner-toast]")
    .filter({ hasText: "Quality preference saved." })
    .boundingBox();
  expect(toastBounds).not.toBeNull();
  expect(toastBounds!.width).toBeLessThanOrEqual(400);
  await page.screenshot({
    animations: "disabled",
    path: testInfo.outputPath("quality-desktop.png"),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await expect(
    page.getByRole("switch", { name: "Enable FLAC", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    animations: "disabled",
    path: testInfo.outputPath("quality-mobile.png"),
    fullPage: true,
  });
});

test("lossless sign-in validates email and password, autosaves and clears the password", async ({
  page,
}) => {
  const { connections } = await settingsFixture(page);
  await page.goto("/settings/connections");
  const email = page.getByLabel("Lossless.am email", { exact: true });
  const password = page.getByLabel("Lossless.am password", { exact: true });
  await email.fill("invalid");
  await password.fill("long-password");
  await password.blur();
  expect(connections).toHaveLength(0);
  await email.fill("listener@example.com");
  await email.blur();
  await expect(
    page
      .locator("[data-sonner-toast]")
      .filter({ hasText: "Lossless.am connected." }),
  ).toBeVisible();
  expect(connections).toEqual([
    {
      kind: "lossless-login",
      email: "listener@example.com",
      password: "long-password",
    },
  ]);
  await expect(password).toHaveValue("");
  await expect(email).toHaveValue("listener@example.com");
});
