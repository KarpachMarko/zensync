import { test, expect, type Page } from "@playwright/test";

async function musicFixture(
  page: Page,
  directDelete: boolean,
  directAlbum = false,
) {
  const searches: string[] = [];
  const deletions: { songId: string; key: string }[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v2/setup")
      return route.fulfill({ json: { required: false } });
    if (url.pathname === "/api/v2/session")
      return route.fulfill({
        json: {
          user: {
            id: "member",
            username: "listener",
            role: "member",
            direct: false,
            directDelete,
            quality: ["128"],
          },
          csrf: "fixture-csrf",
        },
      });
    if (url.pathname === "/api/v2/library") {
      searches.push(route.request().url());
      return route.fulfill({
        json: [
          {
            id: "owned-song",
            title: "Owned recording",
            artist: "Artist",
            album: "Owned album",
            duration: 120,
          },
        ],
      });
    }
    if (url.pathname === "/api/v2/search") {
      searches.push(route.request().url());
      return route.fulfill({
        json: {
          tracks: [
            {
              id: 99,
              title: "Candidate recording",
              title_version: "",
              duration: 120,
              artist: { name: "Artist" },
              album: { title: "Candidate album" },
            },
          ],
          albums: directAlbum
            ? []
            : [
                {
                  id: 42,
                  title: "Candidate album",
                  artist: { name: "Artist" },
                  trackCount: 2,
                },
              ],
          ...(directAlbum
            ? {
                album: {
                  id: 42,
                  title: "Direct album",
                  artist: { name: "Artist" },
                  trackCount: 2,
                },
              }
            : {}),
        },
      });
    }
    if (url.pathname === "/api/v2/deletion-requests") {
      deletions.push({
        ...route.request().postDataJSON(),
        key: route.request().headers()["idempotency-key"],
      });
      return route.fulfill({ status: 201, json: { id: "deletion-request" } });
    }
    throw new Error(
      `Unexpected API call: ${route.request().method()} ${url.pathname}`,
    );
  });
  return { searches, deletions };
}

for (const directDelete of [true, false]) {
  test(`shared library rows support ${directDelete ? "direct deletion" : "deletion requests"} after searching`, async ({
    page,
  }) => {
    const { searches, deletions } = await musicFixture(page, directDelete);
    const action = `${directDelete ? "Delete song" : "Request deletion"}: Owned recording`;
    await page.goto("/library/songs");
    await expect(
      page.getByRole("button", { name: action, exact: true }),
    ).toBeVisible();
    const globalSearch = page.getByRole("complementary").getByLabel("Search music", { exact: true });
    await globalSearch.fill("Owned recording");
    await globalSearch.press("Enter");
    await expect(page).toHaveURL(/search\?q=Owned(?:%20|\+)recording/);
    const existing = page.getByRole("region", {
      name: "Existing library",
      exact: true,
    });
    const candidates = page.getByRole("region", {
      name: "Candidates for import",
      exact: true,
    });
    const button = existing.getByRole("button", { name: action, exact: true });
    await expect(button).toBeVisible();
    await expect(
      candidates.getByText("Candidate recording", { exact: true }),
    ).toBeVisible();
    await expect(
      candidates.getByText("Candidate album", { exact: true }),
    ).toBeVisible();
    const before = searches.length;
    const toggles = page.getByRole("group", { name: "Result type" });
    await expect(
      toggles.getByRole("button", { name: "Both", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
    await toggles.getByRole("button", { name: "Album", exact: true }).click();
    await expect(existing).not.toBeVisible();
    await expect(
      candidates.getByText("Candidate recording", { exact: true }),
    ).not.toBeVisible();
    await expect(
      candidates.getByText("Candidate album", { exact: true }),
    ).toBeVisible();
    await expect(
      candidates.getByRole("button", { name: /^Candidates for import/ }),
    ).toHaveText("Candidates for import1");
    await toggles.getByRole("button", { name: "Song", exact: true }).click();
    await expect(button).toBeVisible();
    await expect(
      candidates.getByText("Candidate recording", { exact: true }),
    ).toBeVisible();
    await expect(
      candidates.getByText("Candidate album", { exact: true }),
    ).not.toBeVisible();
    await toggles.getByRole("button", { name: "Both", exact: true }).click();
    await expect(
      candidates.getByText("Candidate album", { exact: true }),
    ).toBeVisible();
    await expect(toggles.locator('[aria-pressed="true"]')).toHaveCount(1);
    expect(searches).toHaveLength(before);

    await button.click();
    const confirmation = page.getByRole("alertdialog");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toContainText("Owned recording");
    await expect(confirmation).toContainText("for everyone");
    if (!directDelete)
      await expect(confirmation).toContainText("An administrator must approve");
    const cancel = confirmation.getByRole("button", {
      name: "Cancel",
      exact: true,
    });
    const confirm = confirmation.getByRole("button", {
      name: directDelete ? "Delete song" : "Request deletion",
      exact: true,
    });
    await expect(cancel).toBeFocused();
    expect(deletions).toHaveLength(0);
    await cancel.click();
    await expect(confirmation).not.toBeVisible();
    await expect(button).toBeFocused();
    expect(deletions).toHaveLength(0);
    await button.click();
    await page.keyboard.press("Escape");
    await expect(confirmation).not.toBeVisible();
    await expect(button).toBeFocused();
    expect(deletions).toHaveLength(0);
    await button.click();
    await expect(cancel).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(confirm).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(cancel).toBeFocused();
    await confirm.click();
    await expect(confirmation).not.toBeVisible();
    await expect(existing.getByRole("status")).toContainText(
      directDelete
        ? "Deletion approved and queued"
        : "Deletion request submitted for administrator approval",
    );
    await expect(button).toBeDisabled();
    expect(deletions).toEqual([
      { songId: "owned-song", key: expect.stringMatching(/^[0-9a-f]{48}$/) },
    ]);
    await toggles.getByRole("button", { name: "Album", exact: true }).click();
    await toggles.getByRole("button", { name: "Song", exact: true }).click();
    await expect(button).toBeDisabled();
    expect(searches).toHaveLength(before);
  });
}

test("direct album results respect the type toggle without refetching", async ({
  page,
}) => {
  const { searches } = await musicFixture(page, true, true);
  await page.goto("/search?q=album-link");
  const hero = page.getByRole("heading", { name: "Direct album", exact: true });
  await expect(hero).toBeVisible();
  await expect(
    page.getByText("Candidate recording", { exact: true }),
  ).toBeVisible();
  const before = searches.length;
  const toggles = page.getByRole("group", { name: "Result type" });
  await toggles.getByRole("button", { name: "Song", exact: true }).click();
  await expect(hero).not.toBeVisible();
  await expect(
    page.getByRole("button", { name: "Request album", exact: true }),
  ).not.toBeVisible();
  await expect(
    page.getByText("Candidate recording", { exact: true }),
  ).toBeVisible();
  await toggles.getByRole("button", { name: "Album", exact: true }).click();
  await expect(hero).toBeVisible();
  await expect(
    page.getByText("Candidate recording", { exact: true }),
  ).not.toBeVisible();
  const candidates = page.getByRole("region", {
    name: "Candidates for import",
    exact: true,
  });
  await expect(
    candidates.getByRole("button", { name: /^Candidates for import/ }),
  ).toHaveText("Candidates for import1");
  expect(searches).toHaveLength(before);
});

test("deletion dialog keeps failed requests open for retry and prevents duplicate submissions", async ({
  page,
}) => {
  await musicFixture(page, true);
  const keys: string[] = [];
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v2/deletion-requests", async (route) => {
    keys.push(route.request().headers()["idempotency-key"]);
    if (keys.length === 1) {
      await gate;
      await route.fulfill({
        status: 409,
        json: { error: "Song changed; review and try again" },
      });
    } else
      await route.fulfill({ status: 201, json: { id: "retried-deletion" } });
  });
  await page.goto("/library/songs");
  await page
    .getByRole("button", { name: "Delete song: Owned recording" })
    .click();
  const dialog = page.getByRole("alertdialog", { name: "Delete this song?" });
  await dialog
    .getByRole("button", { name: "Delete song", exact: true })
    .click();
  await expect(
    dialog.getByRole("button", { name: "Submitting…", exact: true }),
  ).toBeDisabled();
  await expect(dialog.getByRole("button", { name: "Cancel" })).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  release();
  await expect(dialog.getByRole("alert")).toHaveText(
    "Song changed; review and try again",
  );
  await expect(
    dialog.getByRole("button", { name: "Delete song", exact: true }),
  ).toBeEnabled();
  await dialog
    .getByRole("button", { name: "Delete song", exact: true })
    .click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("status")).toContainText(
    "Deletion approved and queued",
  );
  expect(keys).toHaveLength(2);
  expect(keys[0]).toBe(keys[1]);
});

test("search query and result type survive history navigation and reload", async ({
  page,
}) => {
  await musicFixture(page, true);
  await page.goto("/search?q=first");
  const input = page.getByLabel("Song, artist, album or Deezer URL");
  await expect(input).toHaveValue("first");
  await input.fill("second & third");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page).toHaveURL(
    (url) => url.searchParams.get("q") === "second & third",
  );
  await page.goBack();
  await expect(input).toHaveValue("first");
  await page.goForward();
  await expect(input).toHaveValue("second & third");
  await page
    .getByRole("group", { name: "Result type" })
    .getByRole("button", { name: "Album", exact: true })
    .click();
  await expect(page).toHaveURL(
    (url) =>
      url.searchParams.get("q") === "second & third" &&
      url.searchParams.get("type") === "Album",
  );
  await page.reload();
  await expect(input).toHaveValue("second & third");
  await expect(
    page.getByRole("button", { name: "Album", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("region", { name: "Existing library", exact: true }),
  ).not.toBeVisible();
});

test("library pagination appends server pages through Query", async ({
  page,
}) => {
  await musicFixture(page, true);
  const offsets: number[] = [];
  await page.route("**/api/v2/library?**", async (route) => {
    const offset = Number(
      new URL(route.request().url()).searchParams.get("offset"),
    );
    offsets.push(offset);
    await route.fulfill({
      json: {
        songs: [
          {
            id: `song-${offset}`,
            title: `Library page ${offset}`,
            artist: "Artist",
            album: "Album",
            duration: 120,
          },
          ...(offset === 0 ? Array.from({ length: 30 }, (_, index) => ({
            id: `filler-${index}`, title: `Library filler ${index}`, artist: "Artist", album: "Album", duration: 120,
          })) : []),
        ],
        nextOffset: offset === 0 ? 100 : -1,
      },
    });
  });
  await page.goto("/library/songs");
  await expect(page.getByText("Library page 0", { exact: true })).toBeVisible();
  await page.getByText("Library filler 29", { exact: true }).scrollIntoViewIfNeeded();

  await expect(
    page.getByText("Library page 100", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Library page 0", { exact: true })).toBeVisible();
  await page.getByText("Library filler 29", { exact: true }).scrollIntoViewIfNeeded();
  expect(offsets).toEqual([0, 100]);
});

test("collapsed sidebar stays usable on a narrow screen", async ({
  page,
}, testInfo) => {
  await musicFixture(page, true);
  await page.goto("/search?q=Owned");
  const navigation = page.getByRole("navigation", { name: "Main navigation" });
  await page.getByRole("button", { name: "Collapse sidebar" }).click();
  await expect(
    navigation.getByRole("link", { name: "Library", exact: true }),
  ).toBeVisible();
  await expect(
    navigation.getByText("Library", { exact: true }),
  ).not.toBeVisible();
  await expect(
    page
      .getByRole("region", { name: "Existing library", exact: true })
      .locator("[data-slot=status-section]"),
  ).toHaveCSS("border-left-width", "0px");
  await page.screenshot({
    animations: "disabled",
    path: testInfo.outputPath("desktop-collapsed.png"),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  const mobileNavigation = page.getByRole("dialog", {
    name: "Navigation",
    exact: true,
  });
  await expect(
    mobileNavigation.getByRole("link", { name: "Library", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    animations: "disabled",
    path: testInfo.outputPath("mobile-navigation.png"),
  });
  await mobileNavigation
    .getByRole("button", { name: "Close navigation", exact: true })
    .click();
  await expect(mobileNavigation).not.toBeVisible();
  await expect(
    page.getByRole("button", { name: "Open navigation", exact: true }),
  ).toBeFocused();
  await expect(
    page.getByRole("button", { name: "Expand sidebar" }),
  ).not.toBeVisible();
  await expect(
    page.getByLabel("Song, artist, album or Deezer URL"),
  ).toBeVisible();
  await expect(
    page.getByText("Owned recording", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page
    .getByRole("button", { name: "Delete song: Owned recording", exact: true })
    .click();
  const dialog = page.getByRole("alertdialog", { name: "Delete this song?" });
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  const bounds = await dialog.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390);
  await expect(dialog).toHaveCSS("opacity", "1");
  await page.screenshot({
    animations: "disabled",
    path: testInfo.outputPath("mobile-dialog.png"),
    fullPage: true,
  });
});

test("settings tabs support keyboard navigation, URL state, and action toasts", async ({
  page,
}) => {
  await musicFixture(page, true);
  await page.route("**/api/v2/connections", (route) =>
    route.fulfill({
      json: {
        url: "",
        configured: false,
        personalUsername: "",
        deezerConfigured: false,
        downloaderReady: true,
        musicMounted: true,
        downloaderVersion: "2.3.5",
      },
    }),
  );
  let failSave = false;
  const qualities: unknown[] = [];
  await page.route("**/api/v2/quality", (route) => {
    const body: unknown = route.request().postDataJSON();
    qualities.push(body);
    return route.fulfill({
      status: failSave ? 503 : 200,
      json: failSave ? { error: "Cannot save quality right now" } : {},
    });
  });
  await page.goto("/settings/connections");
  const connections = page.getByRole("tab", {
    name: "Connections",
    exact: true,
  });
  const quality = page.getByRole("tab", { name: "Quality", exact: true });
  await expect(connections).toHaveAttribute("aria-selected", "true");
  await connections.focus();
  await page.keyboard.press("ArrowRight");
  await expect(quality).toBeFocused();
  await expect(page).toHaveURL(/settings\/connections$/);
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/settings\/quality$/);
  await expect(quality).toHaveAttribute("aria-selected", "true");
  await expect(quality).toBeFocused();
  await expect(page.getByRole("tabpanel")).toContainText(
    "Your quality preference",
  );
  await page
    .getByRole("switch", { name: "Enable MP3 320 kbps", exact: true })
    .check();
  await page
    .getByRole("switch", { name: "Enable MP3 128 kbps", exact: true })
    .uncheck();
  const toasts = page.locator("[data-sonner-toast]");
  await expect.poll(() => qualities.at(-1)).toEqual({ quality: ["320"] });
  await expect(
    toasts.filter({ hasText: "Quality preference saved." }),
  ).toBeVisible();
  failSave = true;
  const before = qualities.length;
  await page.getByRole("switch", { name: "Enable FLAC", exact: true }).check();
  await expect(
    toasts.filter({ hasText: "Cannot save quality right now" }),
  ).toBeVisible();
  await expect(page.getByRole("alert")).toHaveText(
    "Cannot save quality right now",
  );
  await expect(
    page.getByRole("switch", { name: "Enable FLAC", exact: true }),
  ).not.toBeChecked();
  expect(qualities).toHaveLength(before + 1);
  await page.reload();
  await expect(quality).toHaveAttribute("aria-selected", "true");
  await expect(toasts).toHaveCount(0);
});

test("collapsible sections work with the keyboard", async ({ page }) => {
  await musicFixture(page, true);
  await page.goto("/search?q=Owned");
  const existing = page.getByRole("region", {
    name: "Existing library",
    exact: true,
  });
  const trigger = existing.getByRole("button", { name: /^Existing library/ });
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await trigger.focus();
  await page.keyboard.press("Enter");
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(
    existing.getByText("Owned recording", { exact: true }),
  ).not.toBeVisible();
  await page.keyboard.press("Space");
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(
    existing.getByText("Owned recording", { exact: true }),
  ).toBeVisible();
  await expect(trigger).toBeFocused();
});

test("sidebar tooltips exist only while collapsed", async ({ page }) => {
  await musicFixture(page, true);
  await page.goto("/search?q=Owned");
  const library = page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Library", exact: true });
  await library.hover();
  await expect(library).not.toHaveAttribute("data-slot", "tooltip-trigger");
  await expect(page.getByRole("tooltip")).toHaveCount(0);
  await page
    .getByRole("button", { name: "Collapse sidebar", exact: true })
    .click();
  await library.hover();
  await expect(
    page.getByRole("tooltip", { name: "Library", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Expand sidebar", exact: true })
    .click();
  await library.hover();
  await expect(page.getByRole("tooltip")).toHaveCount(0);
});

test("search form rejects blank input and submits a validated trimmed query", async ({
  page,
}) => {
  const { searches } = await musicFixture(page, true);
  await page.goto("/search");
  const input = page.getByLabel("Song, artist, album or Deezer URL", {
    exact: true,
  });
  await input.fill("   ");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText("Enter a search query.");
  expect(searches).toHaveLength(0);
  await input.fill("  Owned recording  ");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page).toHaveURL(
    (url) => url.searchParams.get("q") === "Owned recording",
  );
  await expect(input).toHaveValue("Owned recording");
  await expect(
    page.getByText("Owned recording", { exact: true }),
  ).toBeVisible();
  expect(searches).toHaveLength(4);
});

test("provider selection persists and requests namespaced lossless recordings", async ({
  page,
}) => {
  await musicFixture(page, false);
  await page.route("**/api/v2/search?**", (route) => {
    const provider = new URL(route.request().url()).searchParams.get(
      "provider",
    );
    return route.fulfill({
      json: {
        tracks: [
          {
            id: 99,
            title: "Candidate recording",
            duration: 120,
            artist: { name: "Artist" },
            album: { title: "Album" },
            ...(provider === "lossless"
              ? { provider: "lossless", source: "main" }
              : {}),
          },
        ],
        albums: [],
      },
    });
  });
  const targets: string[] = [];
  await page.route("**/api/v2/requests", (route) => {
    targets.push((route.request().postDataJSON() as { target: string }).target);
    return route.fulfill({ status: 201, json: { id: "request" } });
  });
  await page.goto("/search?q=Song&provider=deezer");
  const candidates = page.getByRole("region", {
    name: "Candidates for import",
    exact: true,
  });
  await candidates
    .getByRole("button", { name: "Request", exact: true })
    .click();
  await expect(
    candidates.getByRole("button", { name: "Requested", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("group", { name: "Media provider" })
    .getByRole("button", { name: "lossless.am", exact: true })
    .click();
  await expect(page).toHaveURL(/provider=lossless/);
  await candidates
    .getByRole("button", { name: "Request", exact: true })
    .click();
  expect(targets).toEqual(["deezer:track:99", "lossless:track:main:99"]);
  await page.reload();
  await expect(
    page.getByRole("button", { name: "lossless.am", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("Hitmo search persists its provider and requests a namespaced song", async ({ page }) => {
  await musicFixture(page, false);
  const targets: string[] = [];
  await page.route("**/api/v2/search?**", async route => {
    const url = new URL(route.request().url());
    expect(url.searchParams.get("provider")).toBe("hitmo");
    return route.fulfill({ json: { tracks: [{ provider: "hitmo", id: 42, title: "Hitmo recording", title_version: "", duration: 120, artist: { name: "Artist" }, album: { title: "" } }], albums: [], warnings: [] } });
  });
  await page.route("**/api/v2/requests", async route => {
    targets.push((route.request().postDataJSON() as { target: string }).target);
    return route.fulfill({ status: 201, json: { id: "hitmo-request" } });
  });
  await page.goto("/search?provider=hitmo&q=Artist");
  await expect(page.getByRole("button", { name: "Hitmo", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("Hitmo recording", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: /Open Hitmo recording in Hitmo/ })).toHaveAttribute("href", "https://eu.hitmoz.com/song/42");
  await expect(page.getByRole("button", { name: /Play stream of Hitmo recording/ })).toBeEnabled();
  await page.getByRole("button", { name: "Request", exact: true }).click();
  await expect.poll(() => targets).toEqual(["hitmo:track:42"]);
  await page.reload();
  await expect(page.getByRole("button", { name: "Hitmo", exact: true })).toHaveAttribute("aria-pressed", "true");
});

test("searches every provider concurrently by default and preserves partial results", async ({ page }) => {
  await musicFixture(page, false);
  const called: string[] = [];
  let releaseHitmo: () => void = () => {};
  const hitmoGate = new Promise<void>(resolve => { releaseHitmo = resolve; });
  await page.route("**/api/v2/search?**", async route => {
    const provider = new URL(route.request().url()).searchParams.get("provider") ?? "";
    called.push(provider);
    if (provider === "hitmo") await hitmoGate;
    if (provider === "lossless") return route.fulfill({ status: 502, json: { error: "Provider offline" } });
    return route.fulfill({ json: { tracks: [{ provider, id: 42, title: `${provider} recording`, title_version: "", duration: 90, artist: { name: "Artist" }, album: { title: "Album" } }], albums: [] } });
  });
  await page.goto("/search?q=Artist");
  await expect(page.getByRole("button", { name: "All providers", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect.poll(() => new Set(called).size).toBe(3);
  await expect(page.getByText("deezer recording", { exact: true })).toBeVisible();
  releaseHitmo();
  await expect(page.getByText("hitmo recording", { exact: true })).toBeVisible();
  await expect(page.getByText("Lossless.am: Provider offline", { exact: true })).toBeVisible({ timeout: 15000 });
  await page.getByRole("button", { name: "Hitmo", exact: true }).click();
  await expect(page).toHaveURL(/provider=hitmo/);
  await expect(page.getByText("deezer recording", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "All providers", exact: true }).click();
  await expect(page.getByText("deezer recording", { exact: true })).toBeVisible();
});
