import { readFileSync } from "node:fs";
import { expect, type Page, test } from "@playwright/test";
import { installPreferencesApi } from "./fixtures/preferences";

// The media viewer is a full-window surface: no gutter around it, and the player fits the window
// at any size and in either theme (the header's tint is dark, so the viewer keeps a dark scope).
// Uses a public share page, which opens the same preview dialog as the signed-in file manager.

const now = "2026-10-21T12:00:00Z";
const token = "tds_layoutToken123";
const folderId = "a1111111-1111-4111-8111-111111111111";
const fileId = "b0000000-0000-4000-8000-000000000001";
const video = readFileSync(new URL("./fixtures/viewers/sample.webm", import.meta.url));

function entry(id: string, name: string, kind: "file" | "folder", mimeType: string, size?: number) {
  return {
    id,
    parentId: kind === "folder" ? undefined : folderId,
    name,
    kind,
    mimeType,
    size,
    encryption: false,
    status: "active",
    modTime: now,
    generation: 1,
    createdAt: now,
    updatedAt: now,
  };
}

async function installShare(page: Page) {
  const prefix = `/api/v1/public/shares/${token}`;
  await page.route(
    (url) => url.pathname.startsWith("/api/"),
    async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === prefix) {
        return route.fulfill({
          json: {
            id: "c3333333-3333-4333-8333-333333333333",
            file: entry(folderId, "Videos", "folder", "drive/folder"),
            passwordProtected: false,
            permission: "read",
          },
        });
      }
      if (url.pathname === `${prefix}/files`) {
        return route.fulfill({
          json: { items: [entry(fileId, "Clip.mp4", "file", "video/mp4", video.length)] },
        });
      }
      if (url.pathname.startsWith(`${prefix}/files/${fileId}/content/`)) {
        const range = /^bytes=(\d+)-(\d*)$/.exec(route.request().headers().range ?? "");
        if (!range) return route.fulfill({ status: 200, body: video });
        const start = Number(range[1]);
        const end = range[2] ? Math.min(Number(range[2]), video.length - 1) : video.length - 1;
        if (start >= video.length) return route.fulfill({ status: 416 });
        return route.fulfill({
          status: 206,
          body: video.subarray(start, end + 1),
          headers: {
            "Accept-Ranges": "bytes",
            "Content-Range": `bytes ${start}-${end}/${video.length}`,
          },
        });
      }
      return route.fulfill({ status: 404, json: { code: "not_found", title: "not found" } });
    },
  );
  await installPreferencesApi(page);
}

const WINDOW_SIZES = [
  { name: "desktop", width: 1920, height: 1080 },
  { name: "laptop", width: 1280, height: 800 },
  { name: "short and wide", width: 1280, height: 500 },
  { name: "tablet portrait", width: 768, height: 1024 },
  { name: "phone portrait", width: 390, height: 844 },
  { name: "phone landscape", width: 844, height: 390 },
];

for (const colorScheme of ["dark", "light"] as const) {
  for (const size of WINDOW_SIZES) {
    test(`video viewer fits a ${size.name} window (${size.width}x${size.height}) in the ${colorScheme} theme`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme });
      await page.setViewportSize({ width: size.width, height: size.height });
      await installShare(page);
      await page.goto(`/share/${token}`);
      await page.getByTitle("Clip.mp4").dblclick();
      const player = page.locator("[data-content-viewer] video");
      await expect(player).toBeVisible({ timeout: 20_000 });
      await expect
        .poll(() => player.evaluate((element: HTMLVideoElement) => element.currentTime))
        .toBeGreaterThan(0.2);

      const box = (selector: string) =>
        page
          .locator(selector)
          .first()
          .evaluate((element) => {
            const { x, y, right, bottom } = element.getBoundingClientRect();
            return { x, y, right, bottom };
          });
      const dialog = await box("[data-content-viewer] section");
      const header = await box("[data-content-viewer] section > :first-child");
      const surface = await box("[data-content-viewer] .vplayer");
      const sideways = await page.evaluate(
        () => document.documentElement.scrollWidth - window.innerWidth,
      );

      // The dialog is the whole window, with the header flush against the top and sides.
      expect(dialog).toMatchObject({ x: 0, y: 0, right: size.width, bottom: size.height });
      expect(header).toMatchObject({ x: 0, y: 0, right: size.width });
      // The player sits inside the window, without reaching under the header, at every size.
      expect(surface.x).toBeGreaterThanOrEqual(0);
      expect(surface.right).toBeLessThanOrEqual(size.width);
      expect(surface.y).toBeGreaterThanOrEqual(header.bottom - 1);
      expect(surface.bottom).toBeLessThanOrEqual(size.height);
      expect(sideways).toBeLessThanOrEqual(0);
      // The controls are on screen, not cut off.
      const controls = await page
        .getByRole("button", { name: "Settings" })
        .evaluate((element) => element.getBoundingClientRect().bottom);
      expect(controls).toBeLessThanOrEqual(size.height);
      // Light theme: the viewer is scoped dark, so the header's text stays readable.
      const background = await page
        .locator("[data-content-viewer] section")
        .evaluate((element) => getComputedStyle(element).backgroundColor);
      expect(background).not.toBe("oklch(0.982 0 0)");
    });
  }
}
