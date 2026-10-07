import { expect, type Page, test } from "@playwright/test";

const at = "2026-07-22T12:00:00Z";

function entry(id: string, name: string, kind: "file" | "folder", size?: number) {
  return {
    id,
    name,
    kind,
    status: "active",
    generation: 1,
    modTime: at,
    createdAt: at,
    updatedAt: at,
    encryption: false,
    ...(size === undefined ? {} : { size }),
  };
}

async function installFileApi(page: Page) {
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^\/api/, "");
    if (path === "/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture User",
          username: "fixture",
          premium: true,
          role: "owner",
          capabilities: ["files.read", "files.write"],
          createdAt: at,
        },
      });
    }
    if (path === "/v1/files" && route.request().method() === "GET") {
      return route.fulfill({
        json: {
          items: [
            entry("11111111-1111-4111-8111-111111111111", "Media", "folder", 2469606195),
            entry("22222222-2222-4222-8222-222222222222", "Empty", "folder", 0),
            // What a server from before folder sizes sends: no size at all.
            entry("33333333-3333-4333-8333-333333333333", "Legacy", "folder"),
            entry("44444444-4444-4444-8444-444444444444", "movie.mkv", "file", 1503238553),
          ],
        },
      });
    }
    return route.fulfill({ status: 404, json: {} });
  });
}

for (const view of ["list", "grid"] as const) {
  test(`a folder says what it holds in the ${view} view`, async ({ page }) => {
    await installFileApi(page);
    await page.goto(`/files?view=${view}`);
    const row = (name: string) => page.getByRole("row", { name: new RegExp(name) });
    await expect(row("Media")).toContainText("Folder · 2.3 GB");
    await expect(row("Empty")).toContainText("Folder · 0 B");
    await expect(row("movie.mkv")).toContainText("1.4 GB");
    // No size from the server: the folder is a folder, and nothing is made up.
    await expect(row("Legacy")).toContainText("Folder");
    await expect(row("Legacy")).not.toContainText("·");
  });
}
