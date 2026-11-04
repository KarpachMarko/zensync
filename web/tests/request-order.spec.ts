import { test, expect } from "@playwright/test";

test("Requests and Activity share status sections and active-stage ordering", async ({
  page,
}) => {
  const states = [
    "queued",
    "completed",
    "failed",
    "validating",
    "importing",
    "awaiting_scan",
    "downloading",
  ];
  const rows = states.map((state) => ({
    id: state,
    title: state,
    state,
    owner: "admin",
    kind: "track",
    artist: "Artist",
    target: "lossless:track:main:42",
    decision: "approved",
    quality: ["lossless", "320", "128"],
    reason: "",
    upgrade: false,
    stage: "Stage",
  }));
  await page.route("**/api/v2/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/setup"))
      return route.fulfill({ json: { required: false } });
    if (path.endsWith("/session"))
      return route.fulfill({
        json: {
          csrf: "fixture",
          user: {
            id: "admin",
            username: "admin",
            role: "admin",
            quality: ["lossless"],
          },
        },
      });
    if (path.endsWith("/requests"))
      return route.fulfill({
        json: [
          ...rows,
          {
            ...rows[0],
            id: "approval",
            title: "Approval",
            decision: "awaiting_approval",
            state: "awaiting_approval",
          },
        ],
      });
    if (path.endsWith("/activity")) return route.fulfill({ json: rows });
    throw new Error("Unexpected API: " + path);
  });
  const sections = page.locator('[data-slot="status-section"] > button');
  const names = async () =>
    (await sections.allTextContents()).map((text) =>
      text.replace(/\d+$/, "").trim(),
    );
  await page.goto("/requests");
  await expect(sections).toHaveCount(5);
  expect(await names()).toEqual([
    "In progress",
    "Blocked",
    "Requested",
    "Completed",
    "Queued",
  ]);
  const active = page
    .getByRole("button", { name: /^In progress/ })
    .locator("..");
  await expect(active.locator('[data-slot="request"] strong')).toHaveText([
    "downloading",
    "awaiting_scan",
    "importing",
    "validating",
  ]);
  const blocked = page.getByRole("button", { name: /^Blocked/ }).locator("..");
  await expect(blocked.locator('[data-slot="request"] strong')).toHaveText([
    "failed",
  ]);
  await page.goto("/activity");
  await expect(sections).toHaveCount(4);
  expect(await names()).toEqual([
    "In progress",
    "Blocked",
    "Completed",
    "Queued",
  ]);
});
