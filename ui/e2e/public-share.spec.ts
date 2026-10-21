import { expect, type Page, test } from "@playwright/test";
import { startDownloadServer } from "./fixtures/download-server";
import { installPreferencesApi } from "./fixtures/preferences";

const now = "2026-10-18T12:00:00Z";
const token = "tds_fixtureToken123";
const folderId = "a1111111-1111-4111-8111-111111111111";
const fileId = "a2222222-2222-4222-8222-222222222222";
// A type without an in-browser preview, so a double-click still starts a download.
const fileName = "[Anime Time] Monster - 15.zip";

function entry(id: string, name: string, kind: "file" | "folder", parentId?: string) {
  return {
    id,
    parentId,
    name,
    kind,
    mimeType: kind === "folder" ? "drive/folder" : "application/zip",
    size: kind === "folder" ? undefined : 11,
    encryption: false,
    status: "active",
    modTime: now,
    generation: 1,
    createdAt: now,
    updatedAt: now,
  };
}

const folder = entry(folderId, "Monster", "folder");
const file = entry(fileId, fileName, "file", folderId);

// Mock metadata only: native downloads bypass Chromium's route interception
// and are served by startDownloadServer instead.
async function installPublicShareApi(page: Page, shared: ReturnType<typeof entry>) {
  const prefix = `/api/v1/public/shares/${token}`;
  await page.route(
    (url) => url.pathname.startsWith("/api/"),
    async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === prefix) {
        return route.fulfill({
          json: {
            id: "b3333333-3333-4333-8333-333333333333",
            file: shared,
            passwordProtected: false,
            permission: "read",
          },
        });
      }
      if (url.pathname === `${prefix}/files`) {
        return route.fulfill({ json: { items: [file] } });
      }
      return route.fulfill({ status: 404, json: { code: "not_found", title: "not found" } });
    },
  );
  // Registered last so it wins over the catch-all above (Playwright runs the newest route first).
  await installPreferencesApi(page);
}

test("opening a file in a shared folder requests the content URL with a single /api prefix", async ({
  page,
  baseURL,
}) => {
  await installPublicShareApi(page, folder);
  if (!baseURL) throw new Error("Missing Playwright base URL");
  const expected = `/api/v1/public/shares/${token}/files/${fileId}/content/${encodeURIComponent(fileName)}?download=1`;
  const server = await startDownloadServer(baseURL, expected);
  try {
    await page.goto(`${server.origin}/share/${token}`);
    const row = page.getByTitle(fileName);
    await expect(row).toBeVisible();

    const download = page.waitForEvent("download");
    await row.dblclick();
    const completed = await download;
    expect(await completed.failure()).toBeNull();
    expect(server.requests.map((request) => request.url)).toEqual([expected]);
    expect(completed.suggestedFilename()).toBe(fileName);
    await expect(page.getByText(/no longer exists/i)).toHaveCount(0);
  } finally {
    await server.close();
  }
});

test("opening a single-file share requests the root content URL with a single /api prefix", async ({
  page,
  baseURL,
}) => {
  const rootFile = entry(fileId, fileName, "file");
  await installPublicShareApi(page, rootFile);
  if (!baseURL) throw new Error("Missing Playwright base URL");
  const expected = `/api/v1/public/shares/${token}/content/${encodeURIComponent(fileName)}?download=1`;
  const server = await startDownloadServer(baseURL, expected);
  try {
    await page.goto(`${server.origin}/share/${token}`);
    const row = page.getByTitle(fileName);
    await expect(row).toBeVisible();

    const download = page.waitForEvent("download");
    await row.dblclick();
    const completed = await download;
    expect(await completed.failure()).toBeNull();
    expect(server.requests.map((request) => request.url)).toEqual([expected]);
    expect(completed.suggestedFilename()).toBe(fileName);
  } finally {
    await server.close();
  }
});
