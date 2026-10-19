import { expect, type Page, test } from "@playwright/test";
import { installPreferencesApi } from "./fixtures/preferences";

async function installSettingsApi(page: Page) {
  await page.route(
    (url) => url.pathname.startsWith("/api/"),
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/v1/me") {
        return route.fulfill({
          json: {
            userId: 1,
            displayName: "Settings User",
            username: "settingsuser",
            premium: false,
            role: "owner",
            capabilities: [],
            createdAt: "2026-04-20T08:00:00Z",
          },
        });
      }
      if (path === "/api/v1/files/statistics/drive") {
        return route.fulfill({ json: { totalFiles: 1, totalBytes: 1, openUploads: 0 } });
      }
      return route.fulfill({ status: 404, json: { code: "not_found", title: "not found" } });
    },
  );
  // Registered last so it wins over the catch-all above (Playwright runs the newest route first).
  await installPreferencesApi(page);
}

// Geometry of the settings content column relative to the two-column settings grid.
async function contentColumn(page: Page) {
  await expect(page.getByRole("heading", { name: "Account", exact: true })).toBeVisible();
  return page.evaluate(() => {
    const main = [...document.querySelectorAll("main")].at(-1) as HTMLElement;
    const grid = main.parentElement as HTMLElement;
    const m = main.getBoundingClientRect();
    const g = grid.getBoundingClientRect();
    return { offset: Math.round(m.left - g.left), width: Math.round(m.width) };
  });
}

const REM = 16;

test("settings content sits beside the navigation on desktop", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await installSettingsApi(page);
  await page.goto("/settings");
  await expect(page.locator("main aside")).toBeVisible();
  const column = await contentColumn(page);
  expect(column.offset).toBeGreaterThanOrEqual(15 * REM);
  expect(column.width).toBeGreaterThan(500);
});

// The grid is `lg:grid-cols-[15rem_minmax(0,1fr)]` and <main> used to rely on auto-placement, so when
// the nav <aside> was not displayed (for example hidden by a content blocker's cosmetic filter),
// <main> fell into the 15rem track and the whole page collapsed into a narrow column.
test("settings content keeps the wide column when the navigation aside is not displayed", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await installSettingsApi(page);
  await page.goto("/settings");
  await page.addStyleTag({ content: "main aside { display: none !important; }" });
  await expect(page.locator("main aside")).toBeHidden();
  const column = await contentColumn(page);
  expect(column.offset).toBeGreaterThanOrEqual(15 * REM);
  expect(column.width).toBeGreaterThan(500);
});

test.beforeEach(async ({page}) => {await page.addInitScript(() => localStorage.setItem("teldrive-language", "en"));});
