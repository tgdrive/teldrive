import { expect, test } from "@playwright/test";
import { installFileApi } from "./file-api.fixture";

test("appearance applies light tokens and persists across reloads", async ({ page }) => {
  await page.route("**/api/v1/**", async (route) => {
    if (new URL(route.request().url()).pathname === "/api/v1/me") {
      return route.fulfill({ json: { userId: 1, displayName: "Fixture", role: "owner", capabilities: [], premium: false, createdAt: "2026-07-22T12:00:00Z" } });
    }
    if (new URL(route.request().url()).pathname === "/api/v1/files/statistics/drive") {
      return route.fulfill({ json: { totalFiles: 0, totalBytes: 0, openUploads: 0 } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/settings/appearance");
  await page.emulateMedia({ colorScheme: "dark" });
  const root = page.locator("html");
  await expect(root).toHaveAttribute("data-theme", "dark");
  const darkBackground = await root.evaluate((el) => getComputedStyle(el).getPropertyValue("--background"));
  await page.getByRole("button", { name: "Claro", exact: true }).click();
  await expect(root).toHaveAttribute("data-theme", "light");
  await expect(root).not.toHaveClass(/dark/);
  await expect(root).toHaveCSS("color-scheme", "light");
  await expect.poll(() => root.evaluate((el) => getComputedStyle(el).getPropertyValue("--background"))).not.toBe(darkBackground);
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.getByRole("button", { name: "Automático", exact: true }).click();
  await expect(root).toHaveAttribute("data-theme", "dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(root).toHaveAttribute("data-theme", "dark");
  await page.goto("/settings");
  await page.getByRole("button", { name: "Cerrar sesión", exact: true }).click();
  await expect(page.getByText("No se pudo cerrar la sesión", { exact: true })).toBeVisible();
  await expect(page.locator("[data-sonner-toaster]")).toHaveAttribute("data-sonner-theme", "dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("[data-sonner-toaster]")).toHaveAttribute("data-sonner-theme", "light");
});

test("users search is debounced and stays usable while results load", async ({ page }) => {
  const users = [
    { userId: 1, displayName: "Propietario del servidor", role: "owner", disabled: false },
    { userId: 2, displayName: "Alice", username: "alice", role: "user", disabled: false },
  ];
  const searches: (string | null)[] = [];
  let releaseSearch!: () => void;
  const searchResponse = new Promise<void>((resolve) => {
    releaseSearch = resolve;
  });

  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          ...users[0],
          displayName: "Fixture Admin",
          capabilities: ["system.manageUsers", "system.owner"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/admin/users") {
      const search = url.searchParams.get("search");
      searches.push(search);
      if (search) await searchResponse;
      return route.fulfill({
        json: search ? users.filter((user) => user.displayName.includes(search)) : users,
      });
    }
    return route.fulfill({ status: 404, json: {} });
  });

  await page.goto("/settings/users");
  await expect(page.getByText("Propietario del servidor", { exact: true })).toBeVisible();
  expect(searches).toEqual([null]);

  const input = page.getByRole("textbox", { name: "Buscar usuarios" });
  await input.fill("A");
  await page.waitForTimeout(150);
  await input.fill("Al");
  await page.waitForTimeout(150);
  await input.fill(" Alice ");
  expect(searches).toEqual([null]);
  await expect(input).toHaveValue(" Alice ");

  await expect.poll(() => searches).toEqual([null, "Alice"]);
  await expect(input).toBeVisible();
  await expect(input).toBeFocused();
  await expect(page.getByText("Propietario del servidor", { exact: true })).toBeVisible();
  const response = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === "/api/v1/admin/users" && url.searchParams.get("search") === "Alice";
  });
  releaseSearch();
  await (await response).finished();
  await expect(page.getByText("Propietario del servidor", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Alice", { exact: true })).toBeVisible();

  // Whitespace-only changes should not issue another search.
  await input.fill("Alice");
  await page.waitForTimeout(350);
  expect(searches).toEqual([null, "Alice"]);

  // Clearing restores the cached unfiltered list without an unnecessary request.
  await input.fill("");
  await expect(page.getByText("Propietario del servidor", { exact: true })).toBeVisible();
  expect(searches).toEqual([null, "Alice"]);
});

test("legacy theme preference survives upgrading without appearance settings", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("theme", "light"));
  await installFileApi(page);
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/settings/appearance");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem("teldrive-v2-appearance")!).mode)).toBe("light");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
});
