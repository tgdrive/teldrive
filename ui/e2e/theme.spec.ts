import { expect, type Page, test } from "@playwright/test";

async function signedIn(page: Page) {
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture Admin",
          role: "owner",
          disabled: false,
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/channels" && route.request().method() === "GET") {
      return route.fulfill({ json: { items: [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
}

// The lightness of the page background as the browser paints it: oklch(L ...).
async function backgroundLightness(page: Page) {
  return page.evaluate(() => {
    const value = getComputedStyle(document.documentElement).getPropertyValue("--background");
    return Number.parseFloat(value.replace(/^\s*oklch\(/, ""));
  });
}

test("choosing a theme changes the page, and the choice survives a reload", async ({ page }) => {
  await signedIn(page);
  await page.goto("/settings/appearance");
  const html = page.locator("html");
  await expect(page.getByRole("button", { name: "Light" })).toBeVisible();
  expect(await backgroundLightness(page)).toBeLessThan(0.5);

  await page.getByRole("button", { name: "Light" }).click();
  await expect(html).toHaveClass(/\blight\b/);
  await expect(html).not.toHaveClass(/\bdark\b/);
  await expect(html).toHaveAttribute("data-theme", "light");
  // The tokens the page is drawn with, not only the attributes that select them.
  expect(await backgroundLightness(page)).toBeGreaterThan(0.9);
  expect(await html.evaluate((el) => getComputedStyle(el).colorScheme)).toBe("light");

  await page.reload();
  await expect(page.getByRole("button", { name: "Light" })).toBeVisible();
  await expect(html).toHaveAttribute("data-theme", "light");
  expect(await backgroundLightness(page)).toBeGreaterThan(0.9);

  await page.getByRole("button", { name: "Dark" }).click();
  await expect(html).toHaveClass(/\bdark\b/);
  await expect(html).toHaveAttribute("data-theme", "dark");
  expect(await backgroundLightness(page)).toBeLessThan(0.5);
});

test("the header toggle switches the theme", async ({ page }) => {
  await signedIn(page);
  await page.goto("/settings/appearance");
  await page.getByRole("button", { name: "Toggle color theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  expect(await backgroundLightness(page)).toBeGreaterThan(0.9);
});

test("a toast is drawn in the chosen theme", async ({ page }) => {
  await signedIn(page);
  await page.addInitScript(() => localStorage.setItem("theme", "light"));
  // The sync is refused (the fixture answers 404), which is said in a toast.
  await page.goto("/settings/channels");
  await page.getByRole("button", { name: "Discover and sync" }).click();
  await expect(page.getByText("Channel sync failed")).toBeVisible();
  await expect(page.locator("[data-sonner-toaster]")).toHaveAttribute("data-sonner-theme", "light");
});
