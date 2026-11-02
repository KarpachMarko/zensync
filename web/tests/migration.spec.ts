import { test, expect } from "@playwright/test";
import path from "node:path";

test("first-access setup, real XML preview, review and reload-safe migration", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Welcome to ZenSync" }),
  ).toBeVisible();
  await page.getByLabel("Username", { exact: true }).fill("ui-admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill("local-ui-test-password");
  await page.getByRole("button", { name: "Create administrator" }).click();
  await expect(
    page.getByRole("heading", { name: "Imports", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Collapse sidebar" }),
  ).toHaveText("");
  await page.getByRole("button", { name: "Collapse sidebar" }).click();
  const icons = await page.locator("aside nav .symbol").evaluateAll((nodes) =>
    nodes.map((n) => {
      const r = n.getBoundingClientRect();
      return r.x + r.width / 2;
    }),
  );
  expect(Math.max(...icons) - Math.min(...icons)).toBeLessThan(1);
  await page.getByRole("button", { name: "Open sidebar search" }).click();
  await page.route("**/api/v2/search?q=album%20query*", (route) =>
    route.fulfill({ json: [] }),
  );
  await page
    .getByRole("search")
    .getByRole("textbox", { name: "Search music" })
    .fill("album query");
  await page
    .getByRole("search")
    .getByRole("textbox", { name: "Search music" })
    .press("Enter");
  await expect(page).toHaveURL(/search\?q=album%20query/);
  await expect(
    page.getByLabel("Song, artist, album or Deezer URL"),
  ).toHaveValue("album query");
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Imports" })
    .click();
  await page.getByRole("link", { name: "New migration" }).click();
  await page
    .getByLabel("Apple Music or iTunes XML export")
    .setInputFiles(path.resolve("../internal/apple/testdata/library.xml"));
  await page.getByRole("button", { name: "Upload and preview" }).click();
  await expect(page.getByText("1 unplaylisted songs")).toBeVisible();
  await expect(page.getByText("1 missing references")).toBeVisible();
  await expect(page.getByRole("checkbox", { name: /Roadtrip/ })).toBeChecked();
  await page.getByRole("button", { name: "Commit migration" }).click();
  await expect(
    page.getByRole("heading", { name: "Complete catalog" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /Outside playlists/ }),
  ).toBeVisible();
  const migrationURL = page.url();
  await page.getByRole("button", { name: /Second song/ }).click();
  await expect(
    page.getByRole("heading", { name: "Second song", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Search or Deezer song URL")).toBeVisible();
  await page.getByRole("button", { name: "Skip for now", exact: true }).click();
  await expect(
    page.getByText("Skipped for now", { exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Resume", exact: true }),
  ).toBeVisible();
  await page.goto(migrationURL);
  await page.getByRole("link", { name: /Roadtrip/ }).click();
  await expect(
    page.getByRole("heading", { name: "Ordered songs" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: /First song/ })).toHaveCount(2);
  await page.goto("/requests");
  await expect(page.getByLabel("Deezer track ID")).toHaveCount(0);
  await page
    .getByRole("link", { name: "Search or paste a Deezer link" })
    .click();
  await expect(
    page.getByLabel("Song, artist, album or Deezer URL"),
  ).toBeVisible();
  // Controlled metadata response verifies direct-link selection without a Deezer account.
  await page.route("**/api/v2/search?**", (route) =>
    route.fulfill({
      json: [
        {
          id: 2,
          title: "Second song",
          title_version: "",
          duration: 2,
          artist: { name: "Test artist" },
          album: { title: "Test album" },
        },
      ],
    }),
  );
  await page
    .getByLabel("Song, artist, album or Deezer URL")
    .fill("https://www.deezer.com/track/2");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Request", exact: true }),
  ).toBeVisible();
  await page.route("**/api/v2/requests", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({
        target: "deezer:track:2",
      });
      await route.fulfill({ status: 201, json: { id: "fixture-request" } });
    } else await route.continue();
  });
  await page.getByRole("button", { name: "Request", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Requested", exact: true }),
  ).toBeDisabled();
  let albumDecision = "awaiting_approval";
  let approvals = 0;
  const albumTracks = [
    {
      id: 11,
      title: "Album song one",
      duration: 120,
      title_version: "",
      artist: { name: "Artist" },
      album: { title: "The album" },
    },
    {
      id: 12,
      title: "Album song two",
      duration: 140,
      title_version: "",
      artist: { name: "Artist" },
      album: { title: "The album" },
    },
  ];
  await page.route("**/api/v2/search?**", async (route) => {
    if (
      new URL(route.request().url()).searchParams
        .get("q")
        ?.includes("/album/42")
    )
      await route.fulfill({
        json: {
          tracks: albumTracks,
          album: {
            id: 42,
            title: "The album",
            artist: { name: "Artist" },
            trackCount: 2,
          },
        },
      });
    else await route.fallback();
  });
  await page.route("**/api/v2/search?**", async (route) => {
    if (
      new URL(route.request().url()).searchParams.get("q") ===
      "Artist album keywords"
    )
      await route.fulfill({
        json: {
          tracks: albumTracks,
          albums: [
            {
              id: 42,
              title: "The album",
              artist: { name: "Artist" },
              trackCount: 2,
            },
          ],
          warnings: [],
        },
      });
    else await route.fallback();
  });
  await page.route("**/api/v2/requests", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({
        target: "deezer:album:42",
      });
      await route.fulfill({ status: 201, json: { id: "fixture-album" } });
    } else
      await route.fulfill({
        json: [
          {
            id: "fixture-album",
            owner: "admin",
            title: "The album",
            artist: "Artist",
            target: "deezer:album:42",
            kind: "album",
            trackCount: 2,
            decision: albumDecision,
            quality: ["lossless", "320", "128"],
            items: albumTracks.map((t) => ({
              title: t.title,
              artist: "Artist",
              state:
                albumDecision === "approved" ? "blocked" : "awaiting_approval",
              detail: "",
            })),
          },
        ],
      });
  });
  await page
    .getByLabel("Song, artist, album or Deezer URL")
    .fill("https://www.deezer.com/album/42");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "The album", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Song, artist, album or Deezer URL")
    .fill("Artist album keywords");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Album results" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Songs", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("region", { name: "Album results" })
      .getByText("The album", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("region", { name: "Album results" })
    .getByRole("button", { name: "Request album", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Album requested", exact: true }),
  ).toBeDisabled();
  await page.route("**/api/v2/requests/fixture-album", async (route) => {
    approvals++;
    expect(route.request().postDataJSON().decision).toBe("approved");
    albumDecision = "approved";
    await route.fulfill({ json: { ok: true } });
  });
  await page.goto("/requests");
  await expect(page.locator("article.request")).toHaveCount(1);
  await page.getByText("2 tracks · 0 available").click();
  await expect(page.getByText("1. Album song one")).toBeVisible();
  await page.getByRole("button", { name: "Approve", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Approve", exact: true }),
  ).toHaveCount(0);
  expect(approvals).toBe(1);
  await page.goto("/settings/connections");
  await expect(
    page.getByRole("heading", { name: "Your personal Navidrome account" }),
  ).toBeVisible();
  await expect(page.getByLabel("Deezer ARL cookie")).toBeVisible();
  await page.goto(migrationURL);
  page.once("dialog", (dialog) => dialog.accept());
  await page
    .getByRole("button", { name: "Finish migration", exact: true })
    .click();
  await expect(
    page.getByText("Navidrome now owns these collections.", { exact: false }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Publish to Navidrome" }),
  ).toHaveCount(0);
  expect(errors).toEqual([]);
});
