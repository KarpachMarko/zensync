import { test, expect, type Page } from "@playwright/test";

async function fixture(page: Page, admin = true, coversAvailable = false) {
  let saved = false;
  let edit = {
    title: "Song",
    artist: "Band",
    album: "Album",
    coverUrl: "",
    lyrics: "",
    syncedLyrics: "",
    lyricsSource: "",
  };
  await page.route("**/api/v2/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path === "/api/v2/setup")
      return route.fulfill({ json: { required: false } });
    if (path === "/api/v2/session")
      return route.fulfill({
        json: {
          user: {
            id: "admin",
            username: "admin",
            role: admin ? "admin" : "member",
            quality: ["128"],
          },
          csrf: "csrf",
        },
      });
    if (path === "/api/v2/library")
      return route.fulfill({
        json: {
          songs: [
            {
              id: "one",
              title: "Song",
              artist: "Band",
              album: "Album",
              duration: 120,
            },
          ],
          nextOffset: -1,
        },
      });
    if (path === "/api/v2/metadata/songs/one") {
      if (route.request().method() === "PUT") {
        edit = route.request().postDataJSON() as typeof edit;
        saved = true;
        return route.fulfill({ json: { ok: true } });
      }
      if (route.request().method() === "DELETE") {
        saved = false;
        edit = { ...edit, title: "Song", lyrics: "", syncedLyrics: "" };
        return route.fulfill({ json: { ok: true } });
      }
      return route.fulfill({ json: { edit, saved } });
    }
    if (path === "/api/v2/metadata/duplicates")
      return route.fulfill({
        json: {
          scanned: 50000,
          limited: true,
          groups: [
            {
              kind: "song",
              reason: "Matching artist and title, duration within two seconds",
              members: [
                { id: "one", label: "Band — Song" },
                { id: "two", label: "Band — Song (another file)" },
              ],
            },
          ],
        },
      });
    if (path === "/api/v2/metadata/lyrics")
      return route.fulfill({
        json: [
          {
            id: 123,
            artistName: "Band",
            trackName: "Song",
            albumName: "Album",
            duration: 120,
            plainLyrics: "Test words",
            syncedLyrics: "[00:01]Test words",
            instrumental: false,
          },
        ],
      });
    if (path === "/api/v2/metadata/covers" && coversAvailable)
      return route.fulfill({
        json: [
          {
            id: "release",
            title: "Album",
            artist: "Band",
            url: "https://archive.org/cover.jpg",
            source: "https://musicbrainz.org/release/release",
          },
        ],
      });
    if (path === "/api/v2/metadata/covers")
      return route.fulfill({
        status: 502,
        json: { error: "Metadata provider is busy; try again later" },
      });
    return route.fulfill({ json: [] });
  });
  return { saved: () => saved, edit: () => edit };
}

test("metadata edits, lyrics selection, persistence and reset", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/metadata");
  await page.getByRole("button", { name: "Edit Song", exact: true }).click();
  await expect(page).toHaveURL(/song=one/);
  await page.getByLabel("Title", { exact: true }).fill("Corrected song");
  await page.getByLabel("Search lyrics", { exact: true }).fill("Band Song");
  await page.getByLabel("Search lyrics", { exact: true }).press("Enter");
  await page.getByText("Preview lyrics", { exact: true }).click();
  await expect(
    page.getByText("[00:01]Test words", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Use lyrics", exact: true }).click();
  await expect(page.getByLabel("Plain lyrics", { exact: true })).toHaveValue(
    "Test words",
  );
  expect(state.saved()).toBe(false);
  await page
    .getByRole("button", { name: "Save metadata", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Restore source metadata", exact: true }),
  ).toBeVisible();
  expect(state.edit().title).toBe("Corrected song");
  expect(state.edit().lyricsSource).toBe("LRCLIB:123");
  await page.reload();
  await expect(page.getByLabel("Title", { exact: true })).toHaveValue(
    "Corrected song",
  );
  await page.getByLabel("Search covers", { exact: true }).press("Enter");
  await expect(page.getByRole("alert")).toContainText("provider is busy");
  await page
    .getByRole("button", { name: "Restore source metadata", exact: true })
    .click();
  await expect(page.getByLabel("Title", { exact: true })).toHaveValue("Song");
});

test("duplicate candidates disclose partial scans and support review", async ({
  page,
}) => {
  await fixture(page);
  await page.goto("/metadata?tab=duplicates");
  await expect(page.getByText(/Partial scan/)).toBeVisible();
  await expect(
    page.getByText(/Nothing is merged or deleted automatically/),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Review", exact: true })
    .first()
    .click();
  await expect(page.getByLabel("Title", { exact: true })).toHaveValue("Song");
});

test("members cannot open metadata management", async ({ page }) => {
  await fixture(page, false);
  await page.goto("/metadata");
  await expect(page.getByRole("alert")).toContainText(
    "requires an administrator",
  );
  await expect(
    page.getByRole("link", { name: "Metadata", exact: true }),
  ).toHaveCount(0);
});

test("cover candidates fill the editor and persist after explicit save", async ({
  page,
}) => {
  const state = await fixture(page, true, true);
  await page.route("https://archive.org/cover.jpg", (route) =>
    route.fulfill({
      status: 200,
      contentType: "image/svg+xml",
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" />',
    }),
  );
  await page.goto("/metadata?song=one");
  await page.getByLabel("Search covers", { exact: true }).press("Enter");
  await expect(
    page.getByRole("link", { name: "Release details" }),
  ).toHaveAttribute("href", "https://musicbrainz.org/release/release");
  await page.getByRole("button", { name: "Use cover", exact: true }).click();
  await expect(page.getByLabel("Cover URL", { exact: true })).toHaveValue(
    "https://archive.org/cover.jpg",
  );
  expect(state.saved()).toBe(false);
  await page
    .getByRole("button", { name: "Save metadata", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Restore source metadata" }),
  ).toBeVisible();
  expect(state.edit().coverUrl).toBe("https://archive.org/cover.jpg");
});
