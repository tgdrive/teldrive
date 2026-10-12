import { expect, test } from "@playwright/test";
import { installFileApi } from "./file-api.fixture";

test.beforeEach(async ({ page }) => { await installFileApi(page); });

test("changing to daylight preserves navigation and opaque menus", async ({ page, isMobile }, testInfo) => {
  await page.goto("/files");
  const toggle = page.getByRole("button", { name: "Cambiar tema de color", exact: true });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await (isMobile ? toggle.tap() : toggle.click());
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect(page).toHaveURL(/\/files(?:\?|$)/);
  await expect(page.getByText("Not Found", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("row", { name: /alpha\.txt/ })).toBeVisible();
  await page.getByRole("button", { name: "Subir", exact: true }).click();
  const popover = page.locator(".dropdown__popover");
  await expect(popover).toBeVisible();
  const colors = await popover.evaluate(el => {
    const style = getComputedStyle(el);
    return { background: style.backgroundColor, foreground: style.color, overlay: style.getPropertyValue("--overlay") };
  });
  await testInfo.attach("daylight-menu-colors", { body: JSON.stringify(colors), contentType: "application/json" });
  expect(colors.background).not.toBe("rgba(0, 0, 0, 0)");
  await page.screenshot({ path: testInfo.outputPath("daylight-menu.png") });
  await page.keyboard.press("Escape");
  if (isMobile) {
    await page.getByRole("button", { name: "Abrir navegación", exact: true }).tap();
    await expect(page.getByRole("dialog", { name: "Navegación" }).locator("aside")).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await page.getByRole("link", { name: "Papelera", exact: true }).tap();
  } else await page.getByRole("link", { name: "Papelera", exact: true }).click();
  await expect(page).toHaveURL(/\/trash(?:\?|$)/);
  await toggle.click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await toggle.click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect(page).toHaveURL(/\/trash(?:\?|$)/);
});

test("older bookmarks redirect to the current drive in daylight", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("theme", "light"));
  for (const path of ["/my-drive?path=%2F", "/recent", "/settings/account"]) {
    await page.goto(path);
    await expect(page).toHaveURL(path.startsWith("/settings") ? /\/settings(?:\?|$)/ : /\/files\?/);
    await expect(page.getByText("Not Found", { exact: true })).toHaveCount(0);
  }
});
