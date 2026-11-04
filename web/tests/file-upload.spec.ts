import { test, expect } from "@playwright/test";

test("shadcn file upload supports browsing, removal, drops and validation before submit", async ({ page }) => {
  let uploads = 0;
  let releaseUpload = () => {};
  const pendingUpload = new Promise<void>((resolve) => { releaseUpload = resolve; });
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: {
      csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] },
    } });
    if (path === "/api/v2/imports") {
      if (route.request().method() === "POST") {
        uploads++;
        expect(route.request().headers()["x-import-name"]).toBe("Playlist.TXT");
        expect(route.request().postData()).toContain("Name\tArtist\tAlbum\tTime");
        await pendingUpload;
        return route.fulfill({ json: { id: "uploaded" } });
      }
      return route.fulfill({ json: [] });
    }
    if (path === "/api/v2/imports/uploaded") return route.fulfill({ json: {
      migration: { id: "uploaded", name: "Playlist.TXT", state: "staged", favorites: false, targetUser: "", error: "",
        report: { songs: 1, playlists: 1, unplaylisted: 0, missing: 0, excluded: 0, warnings: [] } },
      songs: [], playlists: [],
    } });
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/imports/new");
  const dropzone = page.getByRole("region", { name: "Drop file here", exact: true });
  const submit = page.getByRole("button", { name: "Upload and preview", exact: true });
  await expect(dropzone).toBeVisible();
  const chooserEvent = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: "Choose file", exact: true }).press("Enter");
  const chooser = await chooserEvent;
  await chooser.setFiles({ name: "library.xml", mimeType: "application/xml", buffer: Buffer.from("<plist/>") });
  const attachment = page.locator('[data-slot="attachment"]');
  await expect(attachment.getByText("library.xml", { exact: true })).toBeVisible();
  await expect(attachment).toHaveAttribute("data-state", "idle");
  await expect(attachment).toContainText("XML · 8 B · Ready to import");
  expect(uploads).toBe(0);
  await page.getByRole("button", { name: "Remove library.xml", exact: true }).click();
  await expect(attachment).toHaveCount(0);
  await expect(dropzone).toBeVisible();

  async function drop(name: string, content: string, type: string) {
    const transfer = await page.evaluateHandle(({ name, content, type }) => {
      const data = new DataTransfer();
      data.items.add(new File([content], name, { type }));
      return data;
    }, { name, content, type });
    try {
      await dropzone.dispatchEvent("dragover", { dataTransfer: transfer });
      await expect(dropzone).toHaveAttribute("data-dragging", "");
      await dropzone.dispatchEvent("drop", { dataTransfer: transfer });
    } finally {
      await transfer.dispose();
    }
  }
  await drop("image.png", "invalid", "image/png");
  await expect(page.getByRole("alert")).toHaveText("Choose an XML or TXT export.");
  expect(uploads).toBe(0);
  await drop("Playlist.TXT", "Name\tArtist\tAlbum\tTime\nSong\tArtist\tAlbum\t100", "text/plain");
  await expect(attachment.getByText("Playlist.TXT", { exact: true })).toBeVisible();
  await expect(attachment).toContainText("TXT");
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(uploads).toBe(0);
  await submit.click();
  await expect(attachment).toHaveAttribute("data-state", "uploading");
  await expect(attachment.getByRole("button", { name: "Remove Playlist.TXT", exact: true })).toBeDisabled();
  await expect(attachment).toContainText("Uploading…");
  releaseUpload();
  await expect(page).toHaveURL(/\/imports\/uploaded$/);
  expect(uploads).toBe(1);
});
