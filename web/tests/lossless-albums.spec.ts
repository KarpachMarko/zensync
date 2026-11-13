import { test, expect, type Page } from "@playwright/test";
import type { Album, Track } from "../src/lib/api";

const albums: Album[] = [
  { id: 42, title: "Deezer album", artist: { name: "Artist" }, trackCount: 2 },
  { provider: "lossless", source: "main", sourceId: "42", id: 42, title: "Lossless album", artist: { name: "Artist" }, trackCount: 2 },
  { provider: "lossless", source: "alt", sourceId: "42", id: 42, title: "Alternative album", artist: { name: "Artist" }, trackCount: 2 },
  { provider: "lossless", source: "qobuz", sourceId: "opaque-42", id: 0, title: "Qobuz album", artist: { name: "Artist" }, trackCount: 2 },
];
const tracks: Track[] = [1, 2].map(id => ({ id, provider: "lossless", source: "main", title: `Album track ${id}`, title_version: "", duration: 120, artist: { name: "Artist" }, album: { title: "Lossless album" }, link: "" }));
async function fixture(page: Page) {
  const targets: string[] = [];
  const searches: URL[] = [];
  await page.route("**/api/v2/**", async route => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v2/setup") return route.fulfill({ json: { required: false } });
    if (url.pathname === "/api/v2/session") return route.fulfill({ json: { csrf: "fixture", user: { id: "member", username: "listener", role: "member", direct: false, quality: ["128"] } } });
    if (url.pathname === "/api/v2/library") return route.fulfill({ json: [] });
    if (url.pathname === "/api/v2/search") {
      searches.push(url);
      const provider = url.searchParams.get("provider");
      if (url.searchParams.get("q")?.startsWith("https://lossless.am/app/album/")) {
        expect(provider).toBe("lossless");
        return route.fulfill({ json: { tracks, album: albums[1], albums: [], warnings: [] } });
      }
      return route.fulfill({ json: { tracks: [], albums: provider === "deezer" ? [albums[0]] : provider === "lossless" ? albums.slice(1) : [], warnings: [] } });
    }
    if (url.pathname === "/api/v2/requests") {
      targets.push((route.request().postDataJSON() as { target: string }).target);
      return route.fulfill({ status: 201, json: { id: "album-request" } });
    }
    throw new Error("Unexpected API: " + url.pathname);
  });
  return { targets, searches };
}

test("album requests preserve provider/source identity for matching IDs", async ({ page }) => {
  const { targets } = await fixture(page);
  await page.goto("/search?q=album");
  const results = page.getByRole("region", { name: "Album results" });
  const rows = results.locator("article");
  await expect(rows).toHaveCount(4);
  const lossless = rows.filter({ has: page.getByText("Lossless album", { exact: true }) });
  await expect(lossless).toContainText("Lossless.am");
  await lossless.getByRole("button", { name: "Request album", exact: true }).click();
  await expect(lossless.getByRole("button", { name: "Album requested", exact: true })).toBeDisabled();
  const deezer = rows.filter({ has: page.getByText("Deezer album", { exact: true }) });
  const alternative = rows.filter({ has: page.getByText("Alternative album", { exact: true }) });
  await expect(deezer.getByRole("button", { name: "Request album", exact: true })).toBeEnabled();
  await expect(alternative.getByRole("button", { name: "Request album", exact: true })).toBeEnabled();
  await alternative.getByRole("button", { name: "Request album", exact: true }).click();
  await deezer.getByRole("button", { name: "Request album", exact: true }).click();
  await rows.filter({ has: page.getByText("Qobuz album", { exact: true }) }).getByRole("button", { name: "Request album", exact: true }).click();
  expect(targets).toEqual(["lossless:album:main:42", "lossless:album:alt:42", "deezer:album:42", "lossless:album:qobuz:opaque-42"]);
});

test("Lossless album links open their tracks and submit one full-album request", async ({ page }) => {
  const { targets, searches } = await fixture(page);
  await page.goto("/search?q=album");
  const row = page.getByRole("region", { name: "Album results" }).locator("article").filter({ has: page.getByText("Lossless album", { exact: true }) });
  await row.getByRole("link", { name: "View tracks", exact: true }).click();
  await expect(page).toHaveURL(url => url.searchParams.get("q") === "https://lossless.am/app/album/main/42");
  await expect(page.getByRole("heading", { name: "Lossless album", exact: true })).toBeVisible();
  const results = page.getByRole("region", { name: "Candidates for import", exact: true });
  await expect(results.getByText("Album track 1", { exact: true })).toBeVisible();
  await expect(results.getByText("Album track 2", { exact: true })).toBeVisible();
  await expect(results.getByRole("button", { name: "Play stream of Album track 1", exact: true })).toBeEnabled();
  await results.getByRole("button", { name: "Request album", exact: true }).click();
  await expect(results.getByRole("button", { name: "Included in album", exact: true })).toHaveCount(2);
  expect(targets).toEqual(["lossless:album:main:42"]);
  expect(searches.filter(url => url.searchParams.get("q")?.startsWith("https://")).map(url => url.searchParams.get("provider"))).toEqual(["lossless"]);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Lossless album", exact: true })).toBeVisible();
});
