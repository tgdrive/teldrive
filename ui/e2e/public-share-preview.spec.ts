import { readFileSync } from "node:fs";
import { expect, type Page, type Route, test } from "@playwright/test";
import { startDownloadServer } from "./fixtures/download-server";
import { installPreferencesApi } from "./fixtures/preferences";

const now = "2026-10-21T12:00:00Z";
const token = "tds_previewToken123";
const shareId = "b3333333-3333-4333-8333-333333333333";
const folderId = "a1111111-1111-4111-8111-111111111111";
const webm = readFileSync(new URL("./fixtures/viewers/sample.webm", import.meta.url));

type Fixture = { id: string; name: string; mimeType: string };

// One file per media family the logged-in browser previews, plus one it does not.
const fixtures: Record<string, Fixture> = {
  mkv: {
    id: "a2000000-0000-4000-8000-000000000001",
    name: "Monster - 01.mkv",
    mimeType: "video/x-matroska",
  },
  mp4: { id: "a2000000-0000-4000-8000-000000000002", name: "Clip.mp4", mimeType: "video/mp4" },
  webm: { id: "a2000000-0000-4000-8000-000000000003", name: "Sample.webm", mimeType: "video/webm" },
  audio: { id: "a2000000-0000-4000-8000-000000000004", name: "Song.mp3", mimeType: "audio/mpeg" },
  image: { id: "a2000000-0000-4000-8000-000000000005", name: "Photo.png", mimeType: "image/png" },
  zip: {
    id: "a2000000-0000-4000-8000-000000000006",
    name: "Archive.zip",
    mimeType: "application/zip",
  },
};

function entry(
  id: string,
  name: string,
  kind: "file" | "folder",
  mimeType: string,
  parentId?: string,
) {
  return {
    id,
    parentId,
    name,
    kind,
    mimeType,
    size: kind === "folder" ? undefined : webm.byteLength,
    encryption: false,
    status: "active",
    modTime: now,
    generation: 1,
    createdAt: now,
    updatedAt: now,
  };
}

const folder = entry(folderId, "Series", "folder", "drive/folder");
const files = Object.values(fixtures).map((f) => entry(f.id, f.name, "file", f.mimeType, folderId));

type ShareApi = {
  contentRequests: { url: string; range?: string; cookie?: string }[];
  sessions: number;
  // Order in which the page hit the session and content endpoints.
  order: string[];
};

// Serves the share metadata and a real, seekable webm for every content request so the
// <video> element has something to decode. Range handling mirrors the server's 206 responses.
async function installShareApi(
  page: Page,
  options: { shared?: ReturnType<typeof entry>; passwordProtected?: boolean } = {},
): Promise<ShareApi> {
  const shared = options.shared ?? folder;
  const prefix = `/api/v1/public/shares/${token}`;
  const stats: ShareApi = { contentRequests: [], sessions: 0, order: [] };
  const serveContent = async (route: Route) => {
    const request = route.request();
    const headers = request.headers();
    stats.order.push("content");
    stats.contentRequests.push({
      url: new URL(request.url()).pathname + new URL(request.url()).search,
      range: headers.range,
      cookie: headers.cookie,
    });
    const range = /^bytes=(\d+)-(\d*)$/.exec(headers.range ?? "");
    if (!range) {
      return route.fulfill({
        status: 200,
        body: webm,
        headers: { "Accept-Ranges": "bytes", "Content-Type": "application/octet-stream" },
      });
    }
    const start = Number(range[1]);
    const end = range[2] ? Math.min(Number(range[2]), webm.byteLength - 1) : webm.byteLength - 1;
    return route.fulfill({
      status: 206,
      body: webm.subarray(start, end + 1),
      headers: {
        "Accept-Ranges": "bytes",
        "Content-Range": `bytes ${start}-${end}/${webm.byteLength}`,
        "Content-Type": "application/octet-stream",
      },
    });
  };
  await page.route(
    (url) => url.pathname.startsWith("/api/"),
    async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === prefix) {
        return route.fulfill({
          json: {
            id: shareId,
            file: shared,
            passwordProtected: Boolean(options.passwordProtected),
            permission: "read",
          },
        });
      }
      if (url.pathname === `${prefix}/files`) return route.fulfill({ json: { items: files } });
      if (url.pathname === `${prefix}/download-session`) {
        stats.sessions += 1;
        stats.order.push("session");
        return route.fulfill({ status: 204 });
      }
      if (
        url.pathname.startsWith(`${prefix}/content/`) ||
        url.pathname.includes(`${prefix}/files/`)
      ) {
        return serveContent(route);
      }
      return route.fulfill({ status: 404, json: { code: "not_found", title: "not found" } });
    },
  );
  // Registered last so it wins over the catch-all above (Playwright runs the newest route first).
  await installPreferencesApi(page);
  return stats;
}

const contentPath = (f: Fixture) =>
  `/api/v1/public/shares/${token}/files/${f.id}/content/${encodeURIComponent(f.name)}`;

test.describe("public share previews", () => {
  test("a video opens in the player instead of downloading", async ({ page }) => {
    const stats = await installShareApi(page);
    const downloads: string[] = [];
    page.on("download", (download) => downloads.push(download.suggestedFilename()));
    await page.goto(`/share/${token}`);

    await page.getByTitle(fixtures.webm.name).dblclick();

    const video = page.locator("[data-content-viewer] video");
    await expect(video).toBeVisible();
    await expect(video).toHaveAttribute("src", contentPath(fixtures.webm));
    // Real decoding: the fixture is a valid webm and the mock honours Range requests.
    await expect
      .poll(() => video.evaluate((element: HTMLVideoElement) => element.readyState))
      .toBeGreaterThanOrEqual(2);
    await expect
      .poll(() => video.evaluate((element: HTMLVideoElement) => element.duration))
      .toBeGreaterThan(2);
    expect(downloads).toEqual([]);
    expect(stats.contentRequests.some((request) => request.url.includes("download=1"))).toBe(false);
  });

  for (const key of ["mkv", "mp4"] as const) {
    test(`a .${key} file opens a video player pointing at the public content URL`, async ({
      page,
    }) => {
      await installShareApi(page);
      const downloads: string[] = [];
      page.on("download", (download) => downloads.push(download.suggestedFilename()));
      await page.goto(`/share/${token}`);

      await page.getByTitle(fixtures[key].name).dblclick();

      await expect(page.locator("[data-content-viewer] video")).toHaveAttribute(
        "src",
        contentPath(fixtures[key]),
      );
      expect(downloads).toEqual([]);
    });
  }

  test("audio and images preview too", async ({ page }) => {
    await installShareApi(page);
    await page.goto(`/share/${token}`);

    await page.getByTitle(fixtures.audio.name).dblclick();
    await expect(page.locator("[data-content-viewer] audio")).toHaveAttribute(
      "src",
      contentPath(fixtures.audio),
    );
    await page.getByRole("button", { name: /close/i }).click();
    await expect(page.locator("[data-content-viewer]")).toHaveCount(0);

    await page.getByTitle(fixtures.image.name).dblclick();
    await expect(page.locator("[data-content-viewer] img")).toHaveAttribute(
      "src",
      contentPath(fixtures.image),
    );
  });

  test("files without a preview still download", async ({ page, baseURL }) => {
    await installShareApi(page);
    if (!baseURL) throw new Error("Missing Playwright base URL");
    const expected = `${contentPath(fixtures.zip)}?download=1`;
    const server = await startDownloadServer(baseURL, expected);
    try {
      await page.goto(`${server.origin}/share/${token}`);
      const download = page.waitForEvent("download");
      await page.getByTitle(fixtures.zip.name).dblclick();
      const completed = await download;
      expect(await completed.failure()).toBeNull();
      expect(completed.suggestedFilename()).toBe(fixtures.zip.name);
      expect(server.requests.map((request) => request.url)).toEqual([expected]);
      await expect(page.locator("[data-content-viewer]")).toHaveCount(0);
    } finally {
      await server.close();
    }
  });

  test("a single-file share previews through the root content URL", async ({ page }) => {
    const root = entry(fixtures.webm.id, fixtures.webm.name, "file", fixtures.webm.mimeType);
    await installShareApi(page, { shared: root });
    await page.goto(`/share/${token}`);

    await page.getByTitle(fixtures.webm.name).dblclick();

    await expect(page.locator("[data-content-viewer] video")).toHaveAttribute(
      "src",
      `/api/v1/public/shares/${token}/content/${encodeURIComponent(fixtures.webm.name)}`,
    );
  });

  test("the preview's download control is a real link, so its address can be copied", async ({
    page,
    baseURL,
  }) => {
    await installShareApi(page);
    if (!baseURL) throw new Error("Missing Playwright base URL");
    const expected = `${contentPath(fixtures.webm)}?download=1`;
    const server = await startDownloadServer(baseURL, expected);
    try {
      await page.goto(`${server.origin}/share/${token}`);
      await page.getByTitle(fixtures.webm.name).dblclick();

      const link = page.locator("[data-content-viewer]").getByRole("link", { name: /download/i });
      await expect(link).toBeVisible();
      await expect(link).toHaveAttribute("href", expected);
      await expect(link).toHaveAttribute("download", fixtures.webm.name);

      const download = page.waitForEvent("download");
      await link.click();
      const completed = await download;
      expect(await completed.failure()).toBeNull();
      expect(completed.suggestedFilename()).toBe(fixtures.webm.name);
      expect(server.requests.map((request) => request.url)).toEqual([expected]);
    } finally {
      await server.close();
    }
  });

  test("a password-protected share opens a download session before the player loads", async ({
    page,
  }) => {
    const stats = await installShareApi(page, { passwordProtected: true });
    // The password prompt is answered through the same endpoint; the mock accepts any value.
    await page.goto(`/share/${token}`);
    await page.getByTitle(fixtures.webm.name).dblclick();

    await expect(page.locator("[data-content-viewer] video")).toBeVisible();
    expect(stats.order[0]).toBe("session");
    expect(stats.order).toContain("content");
    expect(stats.contentRequests.length).toBeGreaterThan(0);
  });

  test("closing the preview returns to the file list with the folder intact", async ({ page }) => {
    await installShareApi(page);
    await page.goto(`/share/${token}`);
    await page.getByTitle(fixtures.webm.name).dblclick();
    await expect(page.locator("[data-content-viewer] video")).toBeVisible();

    await page.getByRole("button", { name: /close/i }).click();

    await expect(page.locator("[data-content-viewer]")).toHaveCount(0);
    await expect(page.getByTitle(fixtures.webm.name)).toBeVisible();
    await expect(page.getByTitle(fixtures.zip.name)).toBeVisible();
  });
});
