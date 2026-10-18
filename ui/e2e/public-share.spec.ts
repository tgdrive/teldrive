import { expect, type Page, test } from "@playwright/test";
import { installPreferencesApi } from "./fixtures/preferences";

const now = "2026-10-18T12:00:00Z";
const token = "tds_fixtureToken123";
const folderId = "a1111111-1111-4111-8111-111111111111";
const fileId = "a2222222-2222-4222-8222-222222222222";
const fileName = "[Anime Time] Monster - 15.mkv";

function entry(id: string, name: string, kind: "file" | "folder", parentId?: string) {
  return {
    id,
    parentId,
    name,
    kind,
    mimeType: kind === "folder" ? "drive/folder" : "video/x-matroska",
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

// Mocks the public-share API. Every request that reaches the content endpoints is
// recorded so tests can assert on the exact URL the page asked for. Anything that
// doesn't match a real server route answers 404, like the real server does (for
// example a doubled "/api/api/..." prefix).
async function installPublicShareApi(page: Page, shared: ReturnType<typeof entry>) {
  const contentRequests: URL[] = [];
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
      if (
        url.pathname === `${prefix}/files/${fileId}/content/${encodeURIComponent(fileName)}` ||
        url.pathname === `${prefix}/content/${encodeURIComponent(fileName)}`
      ) {
        contentRequests.push(url);
        return route.fulfill({
          status: 200,
          contentType: "video/x-matroska",
          headers: {
            "content-disposition": `attachment; filename="${encodeURIComponent(fileName)}"`,
          },
          body: "hello world",
        });
      }
      if (url.pathname.startsWith("/api/api/")) contentRequests.push(url);
      return route.fulfill({ status: 404, json: { code: "not_found", title: "not found" } });
    },
  );
  // Registered last so it wins over the catch-all above (Playwright runs the newest route first).
  await installPreferencesApi(page);
  return contentRequests;
}

test("opening a file in a shared folder requests the content URL with a single /api prefix", async ({
  page,
}) => {
  const requests = await installPublicShareApi(page, folder);
  await page.goto(`/share/${token}`);
  const row = page.getByTitle(fileName);
  await expect(row).toBeVisible();

  const download = page.waitForEvent("download", { timeout: 10_000 }).catch(() => undefined);
  await row.dblclick();
  await expect.poll(() => requests.length).toBeGreaterThan(0);

  expect(requests.map((url) => url.pathname)).toEqual([
    `/api/v1/public/shares/${token}/files/${fileId}/content/${encodeURIComponent(fileName)}`,
  ]);
  expect(requests[0].searchParams.get("download")).toBe("1");
  expect((await download)?.suggestedFilename()).toBe(fileName);
  await expect(page.getByText(/no longer exists/i)).toHaveCount(0);
});

test("opening a single-file share requests the root content URL with a single /api prefix", async ({
  page,
}) => {
  const rootFile = entry(fileId, fileName, "file");
  const requests = await installPublicShareApi(page, rootFile);
  await page.goto(`/share/${token}`);
  const row = page.getByTitle(fileName);
  await expect(row).toBeVisible();

  const download = page.waitForEvent("download", { timeout: 10_000 }).catch(() => undefined);
  await row.dblclick();
  await expect.poll(() => requests.length).toBeGreaterThan(0);

  expect(requests.map((url) => url.pathname)).toEqual([
    `/api/v1/public/shares/${token}/content/${encodeURIComponent(fileName)}`,
  ]);
  expect((await download)?.suggestedFilename()).toBe(fileName);
});
