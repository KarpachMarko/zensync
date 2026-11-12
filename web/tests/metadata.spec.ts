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
    albumArtist: "Band",
  };
  let puts = 0;
  const merges: unknown[] = [];
  const deletes: unknown[] = [];
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64");
  await page.route("https://archive.org/cover.jpg", (route) => route.fulfill({ contentType: "image/png", body: png }));
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
            url.searchParams.get("field") === "title"
              ? ["Song", "Other song"]
              : url.searchParams.get("field") === "album"
                ? ["Album", "Canonical album"]
                : ["Band", "Canonical band"],
          limited: false,
        },
      });
    if (p === "/api/v2/metadata/songs/one/cover") return route.fulfill({ contentType: "image/png", body: png });
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
  await page.getByRole("combobox", { name: "Artist 1", exact: true }).fill("Can");
  await page
    .getByRole("option", { name: "Canonical band", exact: true })
    .click();
  await expect(
    page.getByRole("combobox", { name: "Artist 1", exact: true }),
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
  await expect.poll(() => page.getByAltText("Current cover", { exact: true }).evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
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
test("utilities merge into a chosen duplicate after an undo window", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/utilities");
  await page
    .getByRole("link", { name: "Review duplicates", exact: true })
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
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1000));
  for (const kind of ["album", "artist"]) {
    const group = page.locator("section").filter({ has: page.getByText(kind, { exact: true }) });
    const merge = group.getByRole("button", { name: "Merge into this", exact: true }).first();
    await merge.click();
    await page.clock.runFor(300);
    await page.getByRole("button", { name: "Undo", exact: true }).click();
    await page.clock.fastForward(5000);
    expect(state.merges.length).toBe(kind === "album" ? 0 : 1);
    await merge.click();
    await page.clock.runFor(300);
    await page.clock.fastForward(4000);
    expect(state.merges.length).toBe(kind === "album" ? 0 : 1);
    await page.clock.fastForward(1000);
    await expect.poll(() => state.merges.length).toBe(kind === "album" ? 1 : 2);
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
  await page.getByLabel("Title", { exact: true }).fill("");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByLabel("Title", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
});

test("albums and combined artist credits are reviewable without destructive collaboration merges", async ({ page }) => {
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
      parts: ["Vacio", "ЛСП", " vacio ", "лсп"], knownArtists: ["Vacio", "ЛСП"],
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
  await expect(dialog.getByLabel("Artist 3", { exact: true })).toHaveCount(0);
  expect(writes).toEqual([]);
  await dialog.getByRole("button", { name: "Move artist 2 up", exact: true }).click();
  await expect(dialog.getByLabel("Artist 1", { exact: true })).toHaveValue("ЛСП");
  await dialog.getByRole("button", { name: "Move artist 2 up", exact: true }).click();
  await dialog.getByRole("button", { name: "Add artist", exact: true }).click();
  await dialog.getByLabel("Artist 3", { exact: true }).fill("Extra artist");
  await dialog.getByRole("button", { name: "Move artist 3 up", exact: true }).click();
  await expect(dialog.getByLabel("Artist 2", { exact: true })).toHaveValue("Extra artist");
  await dialog.getByRole("button", { name: "Remove artist 2", exact: true }).click();
  await expect(dialog.getByLabel("Artist 2", { exact: true })).toHaveValue("ЛСП");
  await expect(dialog.getByLabel("Artist 3", { exact: true })).toHaveCount(0);
  await dialog.getByLabel("Artist 2", { exact: true }).fill("Vacio");
  await dialog.getByLabel("Artist 2", { exact: true }).press("Tab");
  await dialog.getByRole("button", { name: "Confirm separate artists", exact: true }).click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  expect(writes).toEqual([]);
  await dialog.getByLabel("Artist 2", { exact: true }).fill("ЛСП");
  await dialog.getByLabel("Artist 2", { exact: true }).press("Tab");
  await dialog.getByRole("button", { name: "Confirm separate artists", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(writes).toEqual([{ source: "combined-vacio-lsp", name: "Vacio, ЛСП", role: "artist", artists: ["Vacio", "ЛСП"] }]);
  await expect(page.getByRole("region", { name: "Combined artist credit review" })).toHaveCount(0);
});

test("metadata artwork and all identity suggestions recover from gateway HTML responses", async ({ page }) => {
  await fixture(page);
  let suggestionsFail = true;
  let coverSearchFail = true;
  let imageFail = true;
  await page.route("**/api/v2/metadata/suggestions?**", (route) => {
    if (suggestionsFail) return route.fulfill({ status: 403, contentType: "text/html", body: "<!DOCTYPE html><title>Access check</title>" });
    const field = new URL(route.request().url()).searchParams.get("field");
    return route.fulfill({ json: { values: [field === "title" ? "Song" : field === "album" ? "Album" : "Band"], limited: false } });
  });
  await page.route("**/api/v2/metadata/covers?**", (route) => coverSearchFail
    ? route.fulfill({ status: 403, contentType: "text/html", body: "<!DOCTYPE html><title>Access check</title>" })
    : route.fulfill({ json: [{ id: "cover", title: "Album", artist: "Band", url: "https://archive.org/cover.jpg", source: "https://musicbrainz.org/release/cover" }] }));
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64");
  await page.route("**/api/v2/metadata/songs/one/cover", (route) => imageFail
    ? route.fulfill({ status: 404, json: { error: "No artwork" }, headers: { "Cache-Control": "no-store" } })
    : route.fulfill({ contentType: "image/png", body: png }));
  await page.goto("/library?editSong=one");
  const dialog = page.getByRole("dialog");
  for (const label of ["Title", "Artist 1", "Album", "Album artist 1"]) await expect(dialog.getByRole("combobox", { name: label, exact: true })).toBeVisible();
  const retries = dialog.getByRole("button", { name: "Retry suggestions", exact: true });
  await expect(retries).toHaveCount(4);
  suggestionsFail = false;
  for (let remaining = 4; remaining > 0; remaining--) {
    await retries.first().click();
    await expect(retries).toHaveCount(remaining - 1);
  }
  for (const [label, option] of [["Title", "Song"], ["Artist 1", "Band"], ["Album", "Album"], ["Album artist 1", "Band"]]) {
    await dialog.getByRole("combobox", { name: label, exact: true }).click();
    await dialog.getByRole("option", { name: option, exact: true }).click();
  }
  await dialog.getByRole("tab", { name: "Artwork", exact: true }).click();
  await expect(dialog.getByText("Artwork unavailable", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("alert")).toContainText("HTTP 403");
  await expect(dialog.getByText(/Unexpected token/)).toHaveCount(0);
  imageFail = false;
  await dialog.getByRole("button", { name: "Retry image", exact: true }).click();
  await expect.poll(() => dialog.getByAltText("Current cover", { exact: true }).evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
  coverSearchFail = false;
  await dialog.getByRole("button", { name: "Retry cover search", exact: true }).click();
  await expect.poll(() => dialog.getByAltText("Album cover", { exact: true }).evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
});
test("a new frontend deployment offers reload without automatically discarding work", async ({ page }) => {
  await fixture(page);
  await page.route("http://127.0.0.1:18081/", (route) => route.fulfill({ contentType: "text/html", body: '<!DOCTYPE html><script type="module" src="/assets/new-deployment.js"></script>' }));
  await page.goto("/library");
  await expect(page.getByRole("button", { name: "Reload application", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Edit metadata: Song", exact: true }).click();
  await page.getByLabel("Title", { exact: true }).fill("Unsaved title");
  await expect(page.getByLabel("Title", { exact: true })).toHaveValue("Unsaved title");
  await expect(page).toHaveURL(/editSong=one/);
});

test("metadata history lives in Activity while Library and Duplicates show at most three active rows", async ({ page }) => {
  await fixture(page);
  const jobs = [
    { id: "done", title: "Edit metadata / Finished", state: "completed", done: 1, total: 1, message: "Completed history" },
    { id: "failed", title: "Edit metadata / Failed", state: "failed", done: 0, total: 1, message: "Could not read source" },
    { id: "queued-a", title: "Queued A", state: "queued", done: 0, total: 1, message: "Waiting" },
    { id: "running", title: "Running", state: "running", done: 1, total: 4, message: "Writing tags" },
    { id: "queued-b", title: "Queued B", state: "queued", done: 0, total: 1, message: "Waiting" },
    { id: "queued-c", title: "Queued C", state: "queued", done: 0, total: 1, message: "Waiting" },
  ];
  const retries: unknown[] = [];
  await page.route("**/api/v2/metadata/jobs", async (route) => {
    if (route.request().method() === "POST") {
      retries.push(route.request().postDataJSON());
      jobs[1].state = "queued";
      return route.fulfill({ json: { ok: true } });
    }
    return route.fulfill({ json: jobs });
  });
  await page.route("**/api/v2/activity", (route) => route.fulfill({ json: jobs.map((job) => ({
    id: job.id, kind: "metadata", title: job.title, state: job.state, stage: job.message,
  })) }));
  for (const path of ["/library", "/utilities?tool=duplicates"]) {
    await page.goto(path);
    const updates = page.getByRole("region", { name: "Active library updates" });
    await expect(updates.locator('[data-slot="activity-row"]')).toHaveCount(3);
    await expect(updates.locator("strong")).toHaveText(["Library updates", "Running", "Queued A", "Queued B"]);
    await expect(page.getByText("Completed history", { exact: true })).toHaveCount(0);
    await expect(page.getByText("Could not read source", { exact: true })).toHaveCount(0);
  }
  await page.goto("/activity");
  await page.getByRole("button", { name: /^Completed/ }).click();
  await expect(page.locator('[data-slot="activity-row"]')).toHaveCount(6);
  await expect(page.getByText("Edit metadata / Finished", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Actions for Edit metadata / Failed", exact: true }).click();
  await page.getByRole("menuitem", { name: "Retry", exact: true }).click();
  expect(retries).toEqual([{ id: "failed" }]);
});

test("duplicate type filter is retained in the URL and separates credit issues from duplicates", async ({ page }) => {
  await fixture(page);
  const groups: DuplicateGroup[] = (["song", "album", "artist"] as const).map((kind) => ({
    kind, reason: "Fixture", members: [{ id: kind + "-one", label: kind + " first", count: 1, song }, { id: kind + "-two", label: kind + " second", count: 1, song: { ...song, id: "two" } }],
  }));
  await page.route("**/api/v2/metadata/duplicates*", (route) => route.fulfill({ json: {
    scanned: 6, limited: false, groups,
    creditIssues: [{ id: "combined", name: "Vacio, ЛСП", role: "artist", parts: ["Vacio", "ЛСП"], knownArtists: ["Vacio", "ЛСП"], tracks: [song] }],
  } }));
  await page.goto("/utilities?tool=duplicates");
  const filter = page.getByRole("combobox", { name: "Show", exact: true });
  await filter.click(); await page.getByRole("option", { name: "Duplicate albums", exact: true }).click();
  await expect(page).toHaveURL(/kind=album/);
  await expect(page.getByText("album first", { exact: true })).toBeVisible();
  await expect(page.getByText("artist first", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Combined artist credit review" })).toHaveCount(0);
  await page.reload();
  await expect(filter).toHaveText("Duplicate albums");
  for (const label of ["Duplicate artists", "Duplicate songs"]) {
    await filter.click(); await page.getByRole("option", { name: label, exact: true }).click();
    await expect(filter).toHaveText(label);
  }
  await filter.click(); await page.getByRole("option", { name: "Combined artist credits", exact: true }).click();
  await expect(page.getByRole("region", { name: "Combined artist credit review" })).toBeVisible();
  await expect(page.getByText("album first", { exact: true })).toHaveCount(0);
  await filter.click(); await page.getByRole("option", { name: "All issues", exact: true }).click();
  await expect(page.getByText("album first", { exact: true })).toBeVisible();
  await expect(page.getByText("artist first", { exact: true })).toBeVisible();
});

test("artwork URLs can be saved during a provider failure", async ({ page }) => {
  const state = await fixture(page);
  await page.route("**/api/v2/metadata/covers?**", (route) => route.fulfill({ status: 502, json: { error: "Metadata search timed out" } }));
  await page.goto("/library?editSong=one");
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("tab", { name: "Artwork", exact: true }).click();
  await expect(dialog.locator('input:not([type="file"]):visible')).toHaveCount(1);
  const input = dialog.getByLabel("Search artwork or image URL", { exact: true });
  await input.focus();
  const image = dialog.getByAltText("Current cover", { exact: true });
  await expect.poll(() => image.evaluate((element) => (element as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
  await input.fill("https://archive.org/cover.jpg");
  await dialog.getByRole("button", { name: "Use URL", exact: true }).click();
  await expect(dialog.getByAltText("Selected cover", { exact: true })).toHaveAttribute("src", "https://archive.org/cover.jpg");
  await dialog.getByRole("button", { name: "Save changes", exact: true }).click();
  expect(state.edit()).toMatchObject({ coverUrl: "https://archive.org/cover.jpg", removeCover: false });
});

test("cover upload is staged with a preview until Save and removal is explicit", async ({ page }) => {
  const state = await fixture(page);
  const token = "a".repeat(64);
  let uploaded = false;
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64");
  await page.route("**/api/v2/metadata/songs/one/cover-upload", async (route) => {
    expect(route.request().method()).toBe("POST");
    uploaded = true;
    return route.fulfill({ status: 201, json: { id: token } });
  });
  await page.route("**/api/v2/metadata/songs/one/cover-upload/*", (route) => route.fulfill({ contentType: "image/png", body: png }));
  await page.goto("/library?editSong=one");
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("tab", { name: "Artwork", exact: true }).click();
  await dialog.locator('input[type="file"]').setInputFiles({ name: "cover.png", mimeType: "image/png", buffer: png });
  await expect(dialog.getByAltText("Uploaded cover", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  expect(uploaded).toBe(true);
  expect(state.puts()).toBe(0);
  await dialog.getByRole("button", { name: "Save changes", exact: true }).click();
  expect(state.edit()).toMatchObject({ coverUploadId: token, coverUrl: "", removeCover: false });

  await page.getByRole("button", { name: "Edit metadata: Song", exact: true }).click();
  await dialog.getByRole("tab", { name: "Artwork", exact: true }).click();
  await dialog.getByRole("button", { name: "Remove cover", exact: true }).click();
  await expect(dialog.getByText(/Embedded cover will be removed/)).toBeVisible();
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await dialog.getByRole("button", { name: "Keep current cover", exact: true }).click();
  await expect(dialog.getByAltText("Current cover", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Remove cover", exact: true }).click();
  await dialog.getByRole("button", { name: "Save changes", exact: true }).click();
  expect(state.edit()).toMatchObject({ removeCover: true, coverUrl: "", coverUploadId: "" });
});

test("dialog reveals a cover drop overlay and accepts a file on Details", async ({ page }) => {
 await fixture(page);
 await page.route("**/api/v2/metadata/songs/one/cover-upload", route => route.fulfill({status:201,json:{id:"a".repeat(64)}}));
 await page.route("**/api/v2/metadata/songs/one/cover-upload/*", route => route.fulfill({contentType:"image/png",body:Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=","base64")}));
 await page.goto("/library?editSong=one");
 const dialog=page.getByRole("dialog");
 const transfer=await page.evaluateHandle(() => {
  const data=new DataTransfer(); data.items.add(new File([new Uint8Array([1,2,3])],"cover.png",{type:"image/png"})); return data;
 });
 await dialog.dispatchEvent("dragenter",{dataTransfer:transfer});
 await expect(dialog.getByText("Drop a JPEG or PNG to use as the cover")).toBeVisible();
 await dialog.dispatchEvent("drop",{dataTransfer:transfer});
 await expect(dialog.getByText("Drop a JPEG or PNG to use as the cover")).toHaveCount(0);
 await expect(dialog.getByRole("tab",{name:"Artwork",exact:true})).toHaveAttribute("data-state","active");
 await expect(dialog.getByAltText("Uploaded cover",{exact:true})).toBeVisible();
});

test("known combined credits queue together without individual dialogs",async ({page})=>{
 await fixture(page);
 await page.route("**/api/v2/metadata/duplicates*",route=>route.fulfill({json:{scanned:3,limited:false,groups:[],creditIssues:[{id:"combined",name:"Vacio, ЛСП",role:"artist",parts:["Vacio","ЛСП"],knownArtists:["Vacio","ЛСП"],tracks:[song]}]}}));
 let calls=0;
 await page.route("**/api/v2/metadata/credits/separate-known",route=>{calls++;return route.fulfill({status:202,json:{id:"batch",songs:1,issues:1}})});
 await page.goto("/utilities?tool=duplicates");
 await page.getByRole("button",{name:"Queue all known artist fixes (1)",exact:true}).click();
 await expect(page.getByRole("button",{name:"Known artist fixes queued",exact:true})).toBeDisabled();
 expect(calls).toBe(1);
});

test("bulk credit repair can explicitly include unknown artist names", async ({ page }) => {
 await fixture(page);
 await page.route("**/api/v2/metadata/duplicates*", route => route.fulfill({json:{scanned:2,limited:false,groups:[],creditIssues:[{id:"combined",name:"Vacio, New Artist",role:"artist",parts:["Vacio","New Artist"],knownArtists:["Vacio"],tracks:[song]}]}}));
 let payload: unknown;
 await page.route("**/api/v2/metadata/credits/separate-known", route => { payload=route.request().postDataJSON();return route.fulfill({status:202,json:{id:"batch",songs:1,issues:1}}); });
 await page.goto("/utilities?tool=duplicates");
 await expect(page.getByRole("button",{name:"Queue all known artist fixes (0)",exact:true})).toBeDisabled();
 await page.getByRole("button",{name:"Queue all artist fixes (1)",exact:true}).click();
 await expect(page.getByRole("button",{name:"All artist fixes queued",exact:true})).toBeDisabled();
 expect(payload).toEqual({includeUnknown:true});
});

test("literal artist names can be dismissed across both roles, undone and kept hidden after reload", async ({ page }) => {
 await fixture(page);
 const name="V $ X V PRiNCE";
 let ignored=false;
 const requests: unknown[]=[];
 const issues=(["artist","album artist"] as const).map(role=>({id:"literal",name,role,parts:["V $","V PRiNCE"],knownArtists:["V $","V PRiNCE"],tracks:[song]}));
 await page.route("**/api/v2/metadata/duplicates*",route=>route.fulfill({json:{scanned:46,limited:false,groups:[],creditIssues:ignored?[]:issues}}));
 await page.route("**/api/v2/metadata/dismissals",route=>{const body=route.request().postDataJSON();requests.push(body);ignored=!body.restore;return route.fulfill({json:{ok:true}})});
 await page.goto("/utilities?tool=duplicates");
 await expect(page.getByRole("button",{name:"Keep as one artist",exact:true})).toHaveCount(2);
 await page.getByRole("button",{name:"Keep as one artist",exact:true}).first().click();
 await expect(page.getByRole("button",{name:"Keep as one artist",exact:true})).toHaveCount(0);
 expect(requests[0]).toEqual({kind:"credit",name});
 await page.getByRole("button",{name:"Undo",exact:true}).click();
 await expect(page.getByRole("button",{name:"Keep as one artist",exact:true})).toHaveCount(2);
 expect(requests[1]).toEqual({kind:"credit",name,restore:true});
 await page.getByRole("button",{name:"Keep as one artist",exact:true}).first().click();
 await expect(page.getByRole("button",{name:"Keep as one artist",exact:true})).toHaveCount(0);
 await page.reload();
 await expect(page.getByRole("button",{name:"Keep as one artist",exact:true})).toHaveCount(0);
});

test("duplicate groups have a non-destructive not-duplicates action with undo", async ({ page }) => {
 await fixture(page);
 let ignored=false;
 let payload: unknown;
 const group={kind:"album",reason:"Same normalized title",members:[{id:"album-b",label:"Distinct edition B",count:1},{id:"album-a",label:"Distinct edition A",count:1}]};
 await page.route("**/api/v2/metadata/duplicates*",route=>route.fulfill({json:{scanned:2,limited:false,groups:ignored?[]:[group],creditIssues:[]}}));
 await page.route("**/api/v2/metadata/dismissals",route=>{const body=route.request().postDataJSON();payload=body;ignored=!body.restore;return route.fulfill({json:{ok:true}})});
 await page.goto("/utilities?tool=duplicates");
 await page.getByRole("button",{name:"Not duplicates",exact:true}).click();
 await expect(page.getByText("Distinct edition A",{exact:true})).toHaveCount(0);
 expect(payload).toEqual({kind:"album",ids:["album-b","album-a"]});
 await page.getByRole("button",{name:"Undo",exact:true}).click();
 await expect(page.getByText("Distinct edition A",{exact:true})).toBeVisible();
});

test("metadata editor uses one row per artist and keeps literal slashes inside a row",async ({page})=>{
 const state=await fixture(page);
 await page.route("**/api/v2/metadata/songs/one",async route=>{
  if(route.request().method()==="PUT") return route.fallback();
  return route.fulfill({json:{duration:120,searchArtist:"May Waves",edit:{...state.edit(),artist:"May Waves / Guest",albumArtist:"May Waves",artistNames:["May Waves","Guest"],albumArtistNames:["May Waves"]}}});
 });
 await page.goto("/library?editSong=one");
 const dialog=page.getByRole("dialog");
 await expect(dialog.getByRole("combobox",{name:"Artist 1",exact:true})).toHaveValue("May Waves");
 await expect(dialog.getByRole("combobox",{name:"Artist 2",exact:true})).toHaveValue("Guest");
 await dialog.getByRole("combobox",{name:"Artist 1",exact:true}).fill("May Wave$");
 await dialog.getByRole("button",{name:"Add artist",exact:true}).click();
 await dialog.getByRole("button",{name:"Save changes",exact:true}).click();
 await expect(dialog.getByRole("combobox",{name:"Artist 3",exact:true})).toHaveAttribute("aria-invalid","true");
 await dialog.getByRole("combobox",{name:"Artist 3",exact:true}).fill("AC/DC");
 await dialog.getByRole("button",{name:"Move artist 2 up",exact:true}).click();
 await dialog.getByRole("button",{name:"Save changes",exact:true}).click();
 await expect(dialog).toHaveCount(0);
 expect(state.edit()).toMatchObject({artist:"Guest / May Wave$ / AC/DC",artistNames:["Guest","May Wave$","AC/DC"],albumArtistNames:["May Waves"]});
});

test("Library manual merges search unrelated artist and album names and require confirmation", async ({ page }) => {
  const state = await fixture(page);
  for (const kind of ["artist", "album"] as const) {
    await page.goto(`/library/${kind}s`);
    await page.getByRole("button", { name: `Merge ${kind}: ${kind === "artist" ? "Band" : "Album"}`, exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("button", { name: "Confirm merge", exact: true })).toBeDisabled();
    await page.route("**/api/v2/library/browse?**", (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.get("q")) return route.fallback();
      return route.fulfill({ json: { collections: [{ id: `${kind}-destination`, name: "Эльдар Джарахов", songCount: 2 }], songs: [], nextOffset: -1 } });
    });
    await dialog.getByLabel(`Find destination ${kind}`, { exact: true }).fill("Джарахов");
    await dialog.getByRole("button", { name: "Search", exact: true }).click();
    await dialog.getByRole("button", { name: /Эльдар Джарахов ·/ }).click();
    expect(state.merges).toHaveLength(kind === "artist" ? 0 : 1);
    await dialog.getByRole("button", { name: "Confirm merge", exact: true }).click();
    await expect(dialog).toHaveCount(0);
  }
  expect(state.merges).toEqual([
    { kind: "artist", source: "artist", target: "artist-destination" },
    { kind: "album", source: "album", target: "album-destination" },
  ]);
});

test("Library song replacement explicitly keeps existing album and playlist references", async ({ page }) => {
  const state = await fixture(page);
  await page.goto("/library");
  await page.getByRole("button", { name: "Merge song: Song", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(/playlist references are preserved/)).toBeVisible();
  await page.route("**/api/v2/library?**", (route) => route.fulfill({ json: { songs: [{ ...song, id: "kept-recording", title: "Kept recording" }], nextOffset: -1 } }));
  await dialog.getByLabel("Find destination song", { exact: true }).fill("Kept");
  await dialog.getByRole("button", { name: "Search", exact: true }).click();
  await dialog.getByRole("button", { name: /Kept recording/ }).click();
  expect(state.merges).toEqual([]);
  await dialog.getByRole("button", { name: "Confirm merge", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(state.merges).toEqual([{ kind: "song", source: "one", target: "kept-recording" }]);
});

test("duplicate merge applies every other entry to the clicked destination", async ({ page }) => {
  const state = await fixture(page);
  await page.route("**/api/v2/metadata/duplicates*", (route) => route.fulfill({ json: {
    scanned: 3, limited: false, groups: [{ kind: "artist", reason: "Similar names", members: [
      { id: "first", label: "First" }, { id: "second", label: "Second" }, { id: "kept", label: "Kept" },
    ] }], creditIssues: [],
  } }));
  await page.route("**/api/v2/metadata/jobs", (route) => route.fulfill({ json: [{ id: "merge-job", state: "completed", done: 1, total: 1 }] }));
  await page.goto("/utilities?tool=duplicates");
  await expect(page.getByRole("button", { name: "Merge into this", exact: true }).last()).toBeVisible();
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1000));
  await page.getByRole("button", { name: "Merge into this", exact: true }).last().click();
  await page.clock.fastForward(5000);
  await expect.poll(() => state.merges).toEqual([
    { kind: "artist", source: "first", target: "kept" },
    { kind: "artist", source: "second", target: "kept" },
  ]);
});

test("leaving duplicate review cancels a delayed merge", async ({ page }) => {
  const state = await fixture(page);
  await page.goto("/utilities?tool=duplicates");
  await expect(page.getByRole("button", { name: "Merge into this", exact: true }).last()).toBeVisible();
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1000));
  await page.getByRole("button", { name: "Merge into this", exact: true }).first().click();
  await page.getByRole("link", { name: "Library", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Library", exact: true })).toBeVisible();
  await page.clock.fastForward(5000);
  expect(state.merges).toEqual([]);
});

test("metadata utilities apply missing and selected updates without approval", async ({ page }) => {
  await fixture(page);
  const requests: { mode: string; songIds: string[]; albumIds: string[] }[] = [];
  await page.route("**/api/v2/metadata/enrichment", route => {
    if (route.request().method() === "GET") return route.fulfill({ json: { songs: [{ ...song, albumId: "album-one", albumArtist: "Band" }], limited: false } });
    requests.push(route.request().postDataJSON() as typeof requests[number]);
    return route.fulfill({ status: 202, json: { id: "job", songs: 1 } });
  });
  await page.goto("/utilities");
  await page.getByRole("link", { name: "Covers and lyrics", exact: true }).click();
  await page.getByRole("button", { name: "Fetch all missing covers", exact: true }).click();
  await expect.poll(() => requests.length).toBe(1);
  await page.getByRole("button", { name: "Fetch all missing lyrics", exact: true }).click();
  await expect.poll(() => requests.length).toBe(2);
  await expect(page.getByRole("button", { name: "Replace selected covers", exact: true })).toBeDisabled();
  await page.getByRole("checkbox").check();
  await page.getByRole("button", { name: "Replace selected covers", exact: true }).click();
  await expect.poll(() => requests.length).toBe(3);
  expect(requests[2]).toEqual({ mode: "force-covers", songIds: [], albumIds: ["album-one"] });
  await page.getByRole("button", { name: "Clear selection", exact: true }).click();
  await page.getByRole("tab", { name: "Songs", exact: true }).click();
  await page.getByRole("checkbox").check();
  await page.getByRole("button", { name: "Correct selected track order", exact: true }).click();
  await expect.poll(() => requests.length).toBe(4);
  expect(requests[3]).toEqual({ mode: "album-order", songIds: ["one"], albumIds: [] });
  expect(requests.slice(0,2).map(request => request.mode)).toEqual(["missing-covers", "missing-lyrics"]);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("metadata utilities are restricted to administrators", async ({ page }) => {
  await fixture(page, false);
  await page.goto("/utilities/metadata");
  await expect(page.getByRole("alert")).toContainText("require an administrator");
  await expect(page.getByRole("button", { name: "Fetch all missing covers", exact: true })).toHaveCount(0);
});

test("metadata utility filters without submitting and opens missing-cover editing", async ({ page }) => {
  await fixture(page);
  await page.route("**/api/v2/metadata/enrichment", route => route.fulfill({ json: { songs: [
    { ...song, albumId: "missing", albumArtist: "Band" },
    { ...song, id: "covered", title: "Covered song", album: "Other album", albumId: "covered-album", albumArtist: "Other artist", coverArt: "cover" },
  ], limited: false } }));
  await page.goto("/utilities/metadata?view=songs");
  await expect(page.getByRole("button", { name: "Search", exact: true })).toHaveCount(0);
  await page.getByRole("switch", { name: "Missing covers only" }).check();
  await expect(page.getByRole("button", { name: "Edit cover: Covered song", exact: true })).toHaveCount(0);
  await page.getByRole("textbox", { name: "Filter library" }).fill("unmatched");
  await expect(page.getByText("No matching items.")).toBeVisible();
  await page.getByRole("textbox", { name: "Filter library" }).fill("Band");
  await page.getByRole("button", { name: "Edit cover: Song", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
});
