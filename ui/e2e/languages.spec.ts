import { expect, test } from "@playwright/test";
import catalog from "../../localization/catalog.json" with { type: "json" };
import { installFileApi } from "./file-api.fixture";

for (const language of Object.keys(catalog)) {
  test(`${language}: persisted language keeps file actions and user names intact`, async ({ page, isMobile }) => {
    await installFileApi(page);
    const dictionary = catalog[language as keyof typeof catalog] as Record<string, string>;
    const label = (source: string) => dictionary[source] ?? source;
    await page.goto("/settings/appearance");
    const picker = page.locator('select[aria-label="Idioma"]');
    await expect(picker.locator("option")).toHaveCount(10);
    await Promise.all([page.waitForEvent("load"), picker.selectOption(language)]);
    expect(await page.evaluate(() => localStorage.getItem("teldrive-language"))).toBe(language);
    await expect(page.getByRole("heading", { name: label("Apariencia"), exact: true, level: 1 })).toBeVisible();
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("dir", language === "ar" ? "rtl" : "ltr");
    await page.goto("/files");
    const row = page.getByRole("row", { name: /alpha\.txt/ });
    await expect(row).toBeVisible();
    await row.locator('[data-slot="checkbox-control"]').click();
    await page.getByRole("button", { name: label("Renombrar elemento seleccionado"), exact: true }).click();
    const dialog = page.getByRole("dialog", { name: label("Renombrar elemento") });
    await dialog.getByRole("textbox", { name: label("Nuevo nombre") }).fill("Papelera-personal.txt");
    await dialog.getByRole("button", { name: label("Renombrar"), exact: true }).click();
    await expect(page.getByRole("row", { name: /Papelera-personal\.txt/ })).toBeVisible();
    if (isMobile) await page.getByRole("button", { name: label("Abrir navegación"), exact: true }).click();
    await page.getByRole("link", { name: label("Papelera"), exact: true }).click();
    await expect(page).toHaveURL(/\/trash(?:\?|$)/);
  });
}
