import { expect, test } from "@playwright/test";
import { installFileApi } from "./file-api.fixture";

test.beforeEach(async ({ page, isMobile }) => {
  test.skip(!isMobile, "Touchscreen scenarios require a touch-enabled browser context");
  await installFileApi(page);
});

for (const width of [320, 390, 768]) {
  test(`touch actions remain reachable at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 844 });
    await page.goto("/files");
    await page.getByRole("button", { name: "Abrir navegación", exact: true }).tap();
    await expect(page.getByRole("link", { name: "Papelera", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Cerrar navegación", exact: true }).last().tap();
    await page.getByRole("button", { name: "Vista de cuadrícula", exact: true }).tap();
    await page.getByRole("row", { name: /alpha\.txt/ }).locator('[data-slot="checkbox-control"]').tap();
    await expect(page.getByRole("row", { name: /alpha\.txt/ })).toHaveAttribute("aria-selected", "true");
    const toolbar = page.getByRole("group", { name: "Acciones de archivos seleccionados" });
    for (const button of await toolbar.getByRole("button").all()) {
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    }
    await page.getByRole("button", { name: "Renombrar elemento seleccionado", exact: true }).tap();
    const rename = page.getByRole("dialog", { name: "Renombrar elemento" });
    await rename.getByRole("textbox", { name: "Nuevo nombre" }).fill("tactil.txt");
    await rename.getByRole("button", { name: "Renombrar", exact: true }).tap();
    await expect(rename).toBeHidden();
    await expect(page.getByText("tactil.txt", { exact: true })).toBeVisible();
    await page.getByRole("row", { name: /tactil\.txt/ }).locator('[data-slot="checkbox-control"]').tap();
    await page.getByRole("button", { name: "Borrar selección", exact: true }).tap();
    await page.getByRole("button", { name: "Vista de lista", exact: true }).tap();
    await page.getByRole("row", { name: /Destino/ }).tap();
    await expect(page.getByRole("row", { name: /tactil\.txt/ })).toBeHidden();
    await page.getByRole("button", { name: "Subir una carpeta", exact: true }).tap();
    await expect(page.getByRole("row", { name: /tactil\.txt/ })).toBeVisible();
  });
}

test("touch upload menu opens the system file chooser", async ({ page }) => {
  await page.goto("/files");
  await page.getByRole("button", { name: "Subir", exact: true }).tap();
  const chooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Subir archivos" }).tap();
  const chooser = await chooserPromise;
  await chooser.setFiles({name: "movil.txt", mimeType: "text/plain", buffer: Buffer.from("subida táctil")});
  await expect(page.getByTestId("upload-shelf")).toBeVisible();
  await page.getByRole("button", { name: "Contraer subidas" }).tap();
  await page.getByRole("button", { name: "Expandir subidas" }).tap();
});

test("touch settings navigation and palette controls work", async ({ page }) => {
  await page.goto("/settings/appearance");
  await page.getByRole("button", { name: "Rojo Mega", exact: true }).tap();
  await page.getByRole("button", { name: "Oscuro", exact: true }).tap();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.getByRole("button", { name: "Abrir menú de configuración", exact: true }).tap();
  await page.getByRole("link", { name: "Rclone", exact: true }).tap();
  await expect(page.getByRole("heading", { name: "Rclone", exact: true, level: 1 })).toBeVisible();
  await page.getByLabel("Clave de API de rclone", {exact: true}).fill("test-key");
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Descargar rclone.conf", exact: true }).tap();
  expect((await downloadPromise).suggestedFilename()).toBe("rclone.conf");
});
