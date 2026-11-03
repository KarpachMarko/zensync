import { test, expect } from "@playwright/test";
import type { Detail, Source } from "../src/lib/migration";

for (const width of [320, 390, 768, 980, 1280]) {
  test(`import rows and recording review fit ${width}px without overlap`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 });
    const recording = { id: 42, title: "A very long recording title with featured artists and additional version information", title_version: "", duration: 180, artist: { name: "Artist with a long collaboration credit" }, album: { title: "An album with a long name" }, preview: "https://cdnt-preview.dzcdn.net/mobile.wav" };
    const wave = Buffer.alloc(44 + 8000 * 31 * 2);
    wave.write("RIFF"); wave.writeUInt32LE(wave.length - 8, 4); wave.write("WAVE", 8);
    wave.write("fmt ", 12); wave.writeUInt32LE(16, 16); wave.writeUInt16LE(1, 20); wave.writeUInt16LE(1, 22);
    wave.writeUInt32LE(8000, 24); wave.writeUInt32LE(16000, 28); wave.writeUInt16LE(2, 32); wave.writeUInt16LE(16, 34);
    wave.write("data", 36); wave.writeUInt32LE(wave.length - 44, 40);
    await page.route("https://cdnt-preview.dzcdn.net/mobile.wav", (route) => route.fulfill({ contentType: "audio/wav", body: wave }));
    const songs: Source[] = Array.from({ length: 3 }, (_, index) => ({
      id: `song-${index}`,
      metadata: { title: index === 0 ? "Taste (feat. Offset and several other collaborators with a very long title)" : "AnUnbrokenSongTitleThatMustWrapRatherThanCoverTheStatusAndButtons" + index,
        artist: "Artist & Another artist with a long credit", album: "A long album title (Original Motion Picture Soundtrack)", duration: 180, favorite: false, missing: false },
      serverId: "", candidate: index === 0 ? recording : null, matchState: index === 1 ? "ambiguous" : "unresolved", deferred: false, favoriteState: "", requestId: "", decision: "", jobState: "",
    }));
    const detail: Detail = { migration: { id: "migration", name: "Library.xml", state: "open", favorites: false, targetUser: "", error: "", report: { songs: 3, playlists: 0, unplaylisted: 3, missing: 0, excluded: 0, warnings: [] } }, songs, playlists: [] };
    await page.route("**/api/v2/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
      if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
      if (path === "/api/v2/imports/migration") return route.fulfill({ json: detail });
      if (path.endsWith("/candidates")) return route.fulfill({ json: { deezer: [recording], library: [{ id: "local", title: recording.title, artist: recording.artist.name, album: recording.album.title, duration: 180 }] } });
      throw new Error("Unexpected API: " + path);
    });
    await page.goto("/imports/migration");
    const rows = page.locator('[data-slot="import-song"]');
    await expect(rows).toHaveCount(3);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
    for (const row of await rows.all()) {
      const geometry = await row.evaluate((element) => {
        const metadata = element.querySelector('button[aria-haspopup="dialog"]');
        const badge = element.querySelector('[data-slot="badge"]');
        const actions = element.querySelector('[data-slot="import-song-actions"]');
        if (!(metadata instanceof HTMLElement) || !badge || !actions) throw new Error("Missing row content");
        const overlap = (a: DOMRect, b: DOMRect) => a.left < b.right - 1 && a.right > b.left + 1 && a.top < b.bottom - 1 && a.bottom > b.top + 1;
        return { overflow: metadata.scrollWidth > metadata.clientWidth + 1,
          overlap: overlap(metadata.getBoundingClientRect(), badge.getBoundingClientRect()) || overlap(metadata.getBoundingClientRect(), actions.getBoundingClientRect()) };
      });
      expect(geometry).toEqual({ overflow: false, overlap: false });
    }
    if (width < 1024) {
      await page.getByRole("button", { name: "Open navigation", exact: true }).click();
      const navigation = page.getByRole("dialog", { name: "Navigation", exact: true });
      await expect(navigation.getByRole("link", { name: "Imports", exact: true })).toBeVisible();
      await navigation.getByRole("button", { name: "Close navigation", exact: true }).click();
      await expect(navigation).not.toBeVisible();
    }
    await page.screenshot({ path: testInfo.outputPath("import-layout.png"), fullPage: true, animations: "disabled" });
    await rows.first().getByRole("button", { name: "Review recording", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("button", { name: "Use this recording", exact: true })).toBeVisible();
    const bounds = await dialog.boundingBox();
    expect(bounds?.x).toBeGreaterThanOrEqual(-1);
    expect((bounds?.x ?? 0) + (bounds?.width ?? 0)).toBeLessThanOrEqual(width + 1);
    expect(await dialog.evaluate((element) => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await dialog.getByRole("button", { name: /^Play 30-second preview/ }).first().click();
    const player = dialog.getByRole("region", { name: "Preview player", exact: true });
    await expect(player).toBeVisible();
    expect(await player.evaluate((element) => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await page.screenshot({ path: testInfo.outputPath("review-layout.png"), animations: "disabled" });
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
    await expect(dialog).not.toBeVisible();
  });
}
