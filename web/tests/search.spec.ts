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
    await page
      .locator("main")
      .getByLabel("Search music", { exact: true })
      .fill("Owned recording");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await expect(page).toHaveURL(/search\?q=Owned%20recording/);
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
    await expect(candidates.locator("summary").first()).toHaveText(
      "Candidates for import1",
    );
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
  await expect(candidates.locator("summary").first()).toHaveText(
    "Candidates for import1",
  );
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
