import { expect, test } from "@playwright/test";
import { alphaId, installFileApi } from "./file-api.fixture";

for (const publicUrl of ["https://drive.example.com/share/fixture-token", "/share/fixture-token"]) {
  test(`copy public share from localhost: ${publicUrl}`, async ({ page }) => {
    await installFileApi(page);
    await page.addInitScript(() => {
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: { writeText: async (value: string) => { (window as typeof window & { copiedText?: string }).copiedText = value; } },
      });
    });
    await page.route(new RegExp(`/api/v1/files/${alphaId}/(shares|grants)`), async (route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 201, json: {
          id: "55555555-5555-4555-8555-555555555555", fileId: alphaId,
          token: "fixture-token", publicUrl, passwordProtected: false,
          permission: "read", createdAt: "2026-10-12T12:00:00Z",
        } });
      }
      return route.fulfill({ json: { items: [] } });
    });
    await page.goto("/files");
    await page.getByRole("row", { name: /alpha\.txt/ }).click();
    await page.getByRole("button", { name: "Compartir elemento seleccionado" }).click();
    await page.getByRole("button", { name: "Crear enlace público", exact: true }).click();
    const expectedUrl = new URL(publicUrl, new URL(page.url()).origin).toString();
    await expect.poll(() => page.evaluate(() => (window as typeof window & { copiedText?: string }).copiedText)).toBe(expectedUrl);
    await expect(page.getByRole("dialog").locator("input[readonly]")).toHaveValue(expectedUrl);
  });
}
