import { test, expect } from "@playwright/test";

function downloadedAudio() {
  const rate = 8000, dataLength = rate * 204 * 2;
  const wav = Buffer.alloc(44 + dataLength);
  wav.write("RIFF"); wav.writeUInt32LE(36 + dataLength, 4); wav.write("WAVE", 8);
  wav.write("fmt ", 12); wav.writeUInt32LE(16, 16); wav.writeUInt16LE(1, 20);
  wav.writeUInt16LE(1, 22); wav.writeUInt32LE(rate, 24); wav.writeUInt32LE(rate * 2, 28);
  wav.writeUInt16LE(2, 32); wav.writeUInt16LE(16, 34); wav.write("data", 36); wav.writeUInt32LE(dataLength, 40);
  return wav;
}

test("failed download can be listened to and its exact duration mismatch explicitly accepted", async ({ page }) => {
  let approved = false;
  const posts: unknown[] = [];
  await page.route("**/api/v2/**", async (route) => {
    const request = route.request(), path = new URL(request.url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["lossless"] } } });
    if (path === "/api/v2/activity") return route.fulfill({ json: [{ id: "job", title: "Waves", state: approved ? "queued" : "failed", stage: "Downloaded duration differs", canReview: !approved }] });
    if (path.endsWith("/review-audio/file")) {
      const body = downloadedAudio();
      const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers().range ?? "");
      if (range) {
        const start = Number(range[1]), end = range[2] ? Math.min(Number(range[2]), body.length - 1) : body.length - 1;
        return route.fulfill({ status: 206, contentType: "audio/wav", headers: { "Accept-Ranges": "bytes", "Content-Range": `bytes ${start}-${end}/${body.length}` }, body: body.subarray(start, end + 1) });
      }
      return route.fulfill({ contentType: "audio/wav", headers: { "Accept-Ranges": "bytes" }, body });
    }
    if (path === "/api/v2/tasks/job/review-audio") {
      if (request.method() === "POST") { posts.push(request.postDataJSON()); approved = true; return route.fulfill({ json: { ok: true } }); }
      return route.fulfill({ json: { title: "Waves", artist: "Ganju", stage: "Expected 324s, measured 204s", canAccept: true, reviewURL: "/imports/migration?recording=source", candidates: [{ quality: "lossless", checksum: "exact-file-hash", expected: 324, measured: 204.069 }] } });
    }
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/activity");
  await page.getByRole("button", { name: "Review recording", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Review failed recording" });
  await expect(dialog.getByText(/Catalog 5:24 → downloaded 3:24/)).toBeVisible();
  await expect(dialog.getByRole("link", { name: /Review match/ })).toHaveAttribute("href", "/imports/migration?recording=source");
  await dialog.getByRole("button", { name: "Listen to downloaded file" }).click();
  const audio = dialog.locator("audio");
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
  await audio.evaluate((element: HTMLAudioElement) => { element.currentTime = 100; element.dispatchEvent(new Event("timeupdate")); });
  await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.currentTime)).toBeGreaterThan(90);
  await expect(audio).not.toHaveAttribute("controls");
  expect(posts).toHaveLength(0);
  await dialog.getByRole("button", { name: "Accept duration difference" }).click();
  expect(posts).toHaveLength(0);
  await dialog.getByRole("button", { name: "Accept this file and retry" }).click();
  await expect(dialog).toHaveCount(0);
  expect(posts).toEqual([{ quality: "lossless", checksum: "exact-file-hash" }]);
  await expect(page.getByText("Duration difference accepted for this file.", { exact: false })).toBeVisible();
});

test("country restriction offers recording review without a bypass approval", async ({ page }) => {
  await page.route("**/api/v2/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup")) return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session")) return route.fulfill({ json: { csrf: "fixture", user: { id: "user", username: "user", role: "member", quality: ["128"] } } });
    if (path === "/api/v2/activity") return route.fulfill({ json: [{ id: "job", title: "Make It Rain", state: "failed", stage: "Deezer denied this recording for the configured account's country", canReview: true }] });
    if (path.endsWith("/review-audio")) return route.fulfill({ json: { title: "Make It Rain", artist: "Ed Sheeran", stage: "Country restriction", canAccept: true, reviewURL: "/search?q=Ed+Sheeran+Make+It+Rain", candidates: [] } });
    throw new Error("Unexpected API: " + path);
  });
  await page.goto("/activity");
  await page.getByRole("button", { name: "Review recording", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Review failed recording" });
  await expect(dialog.getByText(/country restriction requires another accessible recording/)).toBeVisible();
  await expect(dialog.getByRole("button", { name: /Accept/ })).toHaveCount(0);
  await expect(dialog.getByRole("link", { name: /Review match/ })).toHaveAttribute("href", "/search?q=Ed+Sheeran+Make+It+Rain");
});
