import { readFileSync } from "node:fs";
import { test, expect, type Page } from "@playwright/test";
import type { MetadataEdit, DuplicateGroup } from "../src/lib/metadata";
const song = {
  id: "one",
  title: "Song",
  artist: "Band",
  album: "Album",
  duration: 120,
};
async function fixture(page: Page, admin = true) {
  let saved: MetadataEdit = {
    title: "Song",
    artist: "Band",
    album: "Album",
    coverUrl: "",
    lyrics: "",
    syncedLyrics: "",
    lyricsSource: "",
  };
  let puts = 0;
  const merges: unknown[] = [];
  const deletes: unknown[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const url = new URL(route.request().url());
    const p = url.pathname;
    if (p === "/api/v2/setup")
      return route.fulfill({ json: { required: false } });
    if (p === "/api/v2/session")
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
    if (p === "/api/v2/library")
      return route.fulfill({ json: { songs: [song], nextOffset: -1 } });
    if (p === "/api/v2/library/browse") {
      const view = url.searchParams.get("view"),
        id = url.searchParams.get("id");
      const collection =
        view === "artists" && !id
          ? { id: "artist", name: "Band", songCount: 1 }
          : view === "playlists"
            ? { id: "playlist", name: "Favorites", songCount: 1 }
            : { id: "album", name: "Album", artist: "Band", songCount: 1 };
      const showSongs =
        !!id &&
        (view !== "artists" || url.searchParams.get("contents") === "songs");
      return route.fulfill({
        json: {
          collections: showSongs ? [] : [collection],
          songs: showSongs ? [song] : [],
          name: id
            ? view === "artists"
              ? "Band"
              : view === "playlists"
                ? "Favorites"
                : "Album"
            : "",
          nextOffset: -1,
        },
      });
    }
    if (p === "/api/v2/metadata/jobs") return route.fulfill({ json: [] });
    if (p === "/api/v2/metadata/suggestions")
      return route.fulfill({
        json: {
          values:
            url.searchParams.get("field") === "artist"
              ? ["Band", "Canonical band"]
              : ["Album", "Canonical album"],
          limited: false,
        },
      });
    if (p === "/api/v2/metadata/songs/one") {
      if (route.request().method() === "PUT") {
        saved = route.request().postDataJSON() as MetadataEdit;
        puts++;
        return route.fulfill({ status: 202, json: { id: "job" } });
      }
      return route.fulfill({
        json: { edit: saved, saved: puts > 0, duration: 120 },
      });
    }
    if (p === "/api/v2/deletion-requests") {
      deletes.push(route.request().postDataJSON());
      return route.fulfill({ status: 201, json: { id: "deletion" } });
    }
    if (p === "/api/v2/metadata/merge") {
      merges.push(route.request().postDataJSON());
      return route.fulfill({
        status: 202,
        json: { id: "merge-job", songs: 3 },
      });
    }
    if (p === "/api/v2/metadata/duplicates") {
      const groups: DuplicateGroup[] = [
        {
          kind: "song",
          reason: "Matching title and duration",
          members: [
            { id: "one", label: "Song", song },
            { id: "two", label: "Song", song: { ...song, id: "two" } },
          ],
        },
        ...(["album", "artist"] as const).map((kind) => ({
          kind,
          reason: "Matching normalized names",
          members: [
            {
              id: `${kind}-one`,
              albumArtists: kind === "album" ? [{ id: "artist-one", name: "Band" }] : undefined,
              artists: kind === "album" ? [{ id: "artist-one", name: "Band", albumArtist: "Band" }] : undefined,
              label: kind === "album" ? "Band — Album" : "Band",
              count: 3,
            },
            {
              id: `${kind}-two`,
              albumArtists: kind === "album" ? [{ id: "artist-two", name: "band" }] : undefined,
              artists: kind === "album" ? [{ id: "artist-two", name: "band", albumArtist: "band" }] : undefined,
              label: kind === "album" ? "Band — Album!" : "band",
              count: 3,
            },
          ],
        })),
      ];
      return route.fulfill({ json: { scanned: 6, limited: false, groups } });
    }
    if (p === "/api/v2/metadata/lyrics")
      return route.fulfill({
        json: [
          {
            id: 123,
            artistName: "Band",
            trackName: "Song",
            albumName: "Album",
            duration: 120,
            plainLyrics: "Words",
            syncedLyrics: "[00:01]Words",
          },
        ],
      });
    if (p === "/api/v2/metadata/covers")
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
    return route.fulfill({ json: [] });
  });
  await page.route("https://archive.org/cover.jpg", (route) =>
    route.fulfill({
      contentType: "image/svg+xml",
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" />',
    }),
  );
  return { edit: () => saved, puts: () => puts, merges, deletes };
}
test("library root switches all four views and every song list offers edit and delete", async ({
  page,
}) => {
  await fixture(page);
  await page.goto("/library");
  await expect(page.getByRole("tab")).toHaveCount(4);
  await expect(
    page.getByRole("button", { name: "Edit metadata: Song", exact: true }),
  ).toBeVisible();
  for (const view of ["Playlists", "Albums", "Artists"]) {
    await page.getByRole("tab", { name: view, exact: true }).click();
    if (view === "Playlists")
      await page.getByRole("link", { name: "Favorites", exact: true }).click();
    else {
      await page
        .getByRole("link", {
          name: view === "Artists" ? "Band" : "Album — Band",
          exact: true,
        })
        .click();
      if (view === "Artists")
        await page
          .getByRole("link", { name: "Album — Band", exact: true })
          .click();
    }
    await expect(
      page.getByRole("button", { name: "Edit metadata: Song", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Delete song: Song", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Edit metadata: Song", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
  }
});
test("compact TanStack editor supports keyboard autocomplete and provider selection before save", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/library");
  await page
    .getByRole("button", { name: "Edit metadata: Song", exact: true })
    .click();
  await page.getByLabel("Title", { exact: true }).fill("Corrected");
  await page.getByRole("combobox", { name: "Artist", exact: true }).fill("Can");
  await page
    .getByRole("option", { name: "Canonical band", exact: true })
    .click();
  await expect(
    page.getByRole("combobox", { name: "Artist", exact: true }),
  ).toHaveValue("Canonical band");
  await page.getByRole("combobox", { name: "Album", exact: true }).fill("Can");
  await page
    .getByRole("combobox", { name: "Album", exact: true })
    .press("ArrowDown");
  await page
    .getByRole("combobox", { name: "Album", exact: true })
    .press("ArrowDown");
  await page
    .getByRole("combobox", { name: "Album", exact: true })
    .press("Enter");
  await expect(
    page.getByRole("combobox", { name: "Album", exact: true }),
  ).toHaveValue("Canonical album");
  await page.getByRole("tab", { name: "Lyrics", exact: true }).click();
  await expect(page.getByLabel("Search lyrics", { exact: true })).toHaveValue("Band Song");
  await page.getByRole("button", { name: "Use lyrics", exact: true }).click();
  await expect(page.getByLabel("Plain lyrics", { exact: true })).toHaveValue(
    "Words",
  );
  await page.getByRole("tab", { name: "Artwork", exact: true }).click();
  await expect(page.getByAltText("Current cover", { exact: true })).toHaveAttribute("src", "/api/v2/metadata/songs/one/cover");
  await page.getByRole("button", { name: "Use cover", exact: true }).click();
  await expect(page.getByAltText("Selected cover", { exact: true })).toHaveAttribute("src", "https://archive.org/cover.jpg");
  expect(state.puts()).toBe(0);
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(state.edit()).toMatchObject({
    title: "Corrected",
    artist: "Canonical band",
    album: "Canonical album",
    lyricsSource: "LRCLIB:123",
    coverUrl: "https://archive.org/cover.jpg",
  });
});
test("utilities resolve duplicate songs and confirm album and artist merges", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/utilities");
  await page
    .getByRole("button", { name: "Review duplicates", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Delete song: Song", exact: true })
    .first()
    .click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Delete song", exact: true })
    .click();
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  expect(state.deletes).toEqual([{ songId: "one" }]);
  await expect(page.getByText("Album artist IDs differ · review artists first", { exact: true })).toBeVisible();
  await expect(page.getByText("Artist: Band · ID: artist-one · Album artist: Band", { exact: true })).toBeVisible();
  for (const kind of ["album", "artist"]) {
    await page
      .getByRole("button", { name: `Merge into selected ${kind}`, exact: true })
      .click();
    expect(state.merges.length).toBe(kind === "album" ? 0 : 1);
    await page
      .getByRole("button", { name: "Confirm merge", exact: true })
      .click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  }
  expect(state.merges).toEqual([
    { kind: "album", source: "album-two", target: "album-one" },
    { kind: "artist", source: "artist-two", target: "artist-one" },
  ]);
});
test("members retain deletion requests but cannot edit metadata or use utilities", async ({
  page,
}) => {
  await fixture(page, false);
  await page.goto("/library");
  await expect(
    page.getByRole("button", { name: /Edit metadata:/ }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Request deletion: Song", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Utilities", exact: true }),
  ).toHaveCount(0);
  await page.goto("/utilities");
  await expect(page.getByRole("alert")).toContainText(
    "require an administrator",
  );
});

test("artist songs and compact editor work on mobile", async ({ page }) => {
  await fixture(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/library/artists?collection=artist");
  await page
    .getByRole("button", { name: "Show all songs", exact: true })
    .click();
  await expect(page).toHaveURL(/contents=songs/);
  await page
    .getByRole("button", { name: "Edit metadata: Song", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.evaluate(async (element) => {
    await Promise.all(
      element.getAnimations().map((animation) => animation.finished),
    );
  });
  await page.screenshot({
    path: "/tmp/zensync-compact-metadata-editor.png",
    animations: "disabled",
  });
  const box = await dialog.boundingBox();
  expect(box?.width).toBeLessThanOrEqual(390);
  expect(box?.height).toBeLessThan(360);
  await page.getByLabel("Title", { exact: true }).fill("");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByLabel("Title", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
});

test("screenshot albums and combined artist credits are reviewable without destructive collaboration merges", async ({ page }) => {
  const state = await fixture(page);
  const dataset = JSON.parse(readFileSync(new URL("../../internal/app/testdata/screenshot-metadata.json", import.meta.url), "utf8")) as {
    albumTitle: string; albumArtists: string[]; combinedArtists: string[]; standaloneArtist: string;
  };
  await page.route("**/api/v2/metadata/duplicates*", (route) => route.fulfill({ json: {
    scanned: 12, limited: false,
    groups: [{
      kind: "album", reason: "Matching normalized album artist and album title",
      members: dataset.albumArtists.map((artist, index) => ({
        id: `elf-${index}`, label: `${artist} — ${dataset.albumTitle}`, count: 1,
        albumArtists: [{ id: "slava", name: artist }],
        tracks: [{ ...song, artist: index === 0 ? dataset.combinedArtists[0] : artist }],
      })),
    }],
    creditIssues: dataset.combinedArtists.map((name, index) => ({
      id: `combined-${index}`, name, role: "artist",
      parts: name.split(/, | x /i), knownArtists: [dataset.standaloneArtist],
      tracks: [{ ...song, artist: name }],
    })),
  }}));
  await page.goto("/utilities?tool=duplicates");
  await expect(page.getByText("SLAVA MARLOW — ЭЛЬФ 1", { exact: true })).toBeVisible();
  await expect(page.getByText("Slava Marlow — ЭЛЬФ 1", { exact: true })).toBeVisible();
  await expect(page.getByText("Album artist IDs differ · review artists first", { exact: true })).toHaveCount(0);
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  }
  const review = page.getByRole("region", { name: "Combined artist credit review" });
  for (const name of dataset.combinedArtists) await expect(review.getByText(name, { exact: true })).toBeVisible();
  await expect(review.getByRole("button", { name: /Merge/ })).toHaveCount(0);
  await expect(review.getByRole("button", { name: "Separate artist credits", exact: true })).toHaveCount(dataset.combinedArtists.length);
  await review.getByRole("button", { name: "Review affected songs", exact: true }).first().click();
  await review.getByRole("button", { name: "Separate artist credits", exact: true }).first().click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByLabel("Artist 1", { exact: true })).toHaveValue("Aleksey Konstantinovich Uzenjuk");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(state.puts()).toBe(0);
  expect(state.merges).toEqual([]);
});

test("Skate combined-credit repair proposes known artists, validates and queues one explicit correction", async ({ page }) => {
  await fixture(page);
  const writes: unknown[] = [];
  let scanComplete = false;
  await page.route("**/api/v2/metadata/jobs", (route) => {
    if (writes.length) scanComplete = true;
    return route.fulfill({ json: writes.length ? [{ id: "credit-job", state: "completed", done: 1, total: 1, message: "Credits updated; scan requested" }] : [] });
  });
  await page.route("**/api/v2/metadata/duplicates*", (route) => route.fulfill({ json: {
    scanned: 3, limited: false, groups: [],
    creditIssues: scanComplete ? [] : [{ id: "combined-vacio-lsp", name: "Vacio, ЛСП", role: "artist",
      parts: ["Vacio", "ЛСП"], knownArtists: ["Vacio", "ЛСП"],
      tracks: [{ ...song, title: "Skate", album: "Skate", artist: "Vacio, ЛСП • Васильев Николай Михайлович, Олег Вадимович Савченко" }],
    }],
  }}));
  await page.route("**/api/v2/metadata/credits/separate", async (route) => {
    writes.push(route.request().postDataJSON());
    await route.fulfill({ status: 202, json: { id: "credit-job", songs: 1 } });
  });
  await page.goto("/utilities?tool=duplicates");
  await page.getByRole("button", { name: "Separate artist credits", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Artist 1", { exact: true })).toHaveValue("Vacio");
  await expect(dialog.getByLabel("Artist 2", { exact: true })).toHaveValue("ЛСП");
  expect(writes).toEqual([]);
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  }
  await dialog.getByRole("button", { name: "Move artist 2 up", exact: true }).click();
  await expect(dialog.getByLabel("Artist 1", { exact: true })).toHaveValue("ЛСП");
  await dialog.getByRole("button", { name: "Move artist 2 up", exact: true }).click();
  await dialog.getByRole("button", { name: "Add artist", exact: true }).click();
  await dialog.getByLabel("Artist 3", { exact: true }).fill("Extra artist");
  await dialog.getByRole("button", { name: "Move artist 3 up", exact: true }).click();
  await expect(dialog.getByLabel("Artist 2", { exact: true })).toHaveValue("Extra artist");
  await dialog.getByRole("button", { name: "Remove artist 2", exact: true }).click();
  await expect(dialog.getByLabel("Artist 2", { exact: true })).toHaveValue("ЛСП");
  await dialog.getByLabel("Artist 2", { exact: true }).fill("Vacio");
  await dialog.getByRole("button", { name: "Confirm separate artists", exact: true }).click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  expect(writes).toEqual([]);
  await dialog.getByLabel("Artist 2", { exact: true }).fill("ЛСП");
  await dialog.getByRole("button", { name: "Confirm separate artists", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(writes).toEqual([{ source: "combined-vacio-lsp", name: "Vacio, ЛСП", role: "artist", artists: ["Vacio", "ЛСП"] }]);
  await expect(page.getByRole("region", { name: "Combined artist credit review" })).toHaveCount(0);
});
