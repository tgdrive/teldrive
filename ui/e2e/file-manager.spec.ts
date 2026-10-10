import { expect, test } from "@playwright/test";
import { alphaId, betaId, installFileApi } from "./file-api.fixture";

test.beforeEach(async ({ page }) => installFileApi(page));

test("upload menu preserves folder hierarchy and exposes byte-weighted tree progress", async ({
  page,
}) => {
  const folderRequests: Array<{ name: string; parentId?: string }> = [];
  const uploadRequests: Array<{ name: string; parentId?: string; preferredPartSize: number }> = [];
  page.on("request", (request) => {
    if (request.method() !== "POST") return;
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith("/api/v1/folders")) folderRequests.push(request.postDataJSON());
    if (pathname.endsWith("/api/v1/uploads")) uploadRequests.push(request.postDataJSON());
  });

  await page.goto("/files");
  await page.getByRole("button", { name: "Subir", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Subir archivos" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Subir carpeta" })).toBeVisible();

  const folderChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Subir carpeta" }).click();
  const folderChooser = await folderChooserPromise;
  await folderChooser.setFiles("e2e/fixtures/Destination");
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("teldrive.uploads.v3")))
    .toBeNull();

  const tree = page.getByRole("treegrid", { name: "Cola de subidas" });
  await expect(tree).toBeVisible();
  await expect(tree.getByRole("row", { name: /Destination/ })).toBeVisible();
  await expect(tree.getByText("2026", { exact: true })).toBeVisible();
  await expect(tree.getByText("cover.jpg", { exact: true })).toBeVisible();
  await expect(tree.getByText("detail.jpg", { exact: true })).toBeVisible();
  await expect.poll(() => uploadRequests.length).toBe(2);

  expect(folderRequests.map((request) => request.name)).toEqual(["Destination", "2026"]);
  expect(folderRequests[1].parentId).toBeTruthy();
  expect(uploadRequests.every((request) => Boolean(request.parentId))).toBe(true);
  expect(uploadRequests.every((request) => request.preferredPartSize === 512 * 1024 * 1024)).toBe(
    true,
  );
  await expect(page.getByRole("progressbar", { name: "Progreso total de subida" })).toHaveAttribute(
    "aria-valuenow",
    "100",
  );
  const destinationBatch = tree.getByRole("row", { name: /Destination/ });
  await tree.getByRole("button", { name: "Contraer Destination" }).click();
  await expect(destinationBatch).toHaveAttribute("aria-expanded", "false");
  await tree.getByRole("button", { name: "Expandir Destination" }).click();
  await expect(destinationBatch).toHaveAttribute("aria-expanded", "true");

  const shelf = page.getByTestId("upload-shelf");
  await expect(shelf).toHaveScreenshot("upload-tree-expanded.png");
  await page.getByRole("button", { name: "Contraer subidas" }).click();
  await expect(shelf).toHaveScreenshot("upload-tree-collapsed.png");
});

test("upload queue is ephemeral across a browser reload", async ({ page }) => {
  await page.goto("/files");
  await page.getByRole("button", { name: "Subir", exact: true }).click();
  const fileChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Subir archivos" }).click();
  const fileChooser = await fileChooserPromise;
  await fileChooser.setFiles("e2e/fixtures/Destination/cover.jpg");

  const shelf = page.getByTestId("upload-shelf");
  await expect(shelf).toBeVisible();
  const queue = shelf.getByRole("treegrid", { name: "Cola de subidas" });
  await expect(queue.getByRole("row", { name: /cover\.jpg/ })).toBeVisible();
  await expect(queue.getByText("1 file", { exact: true })).toHaveCount(0);
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("teldrive.uploads.v3")))
    .toBeNull();

  await page.reload();
  await expect(page.getByTestId("upload-shelf")).toHaveCount(0);
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("teldrive.uploads.v3")))
    .toBeNull();
});

test("plain multi-file uploads render as flat rows without a batch hierarchy", async ({ page }) => {
  await page.goto("/files");
  await page.getByRole("button", { name: "Subir", exact: true }).click();
  const fileChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Subir archivos" }).click();
  const fileChooser = await fileChooserPromise;
  await fileChooser.setFiles([
    { name: "first.txt", mimeType: "text/plain", buffer: Buffer.from("first") },
    { name: "second.txt", mimeType: "text/plain", buffer: Buffer.from("second") },
  ]);

  const queue = page.getByTestId("upload-shelf").getByRole("treegrid", { name: "Cola de subidas" });
  await expect(queue.getByRole("row", { name: /first\.txt/ })).toBeVisible();
  await expect(queue.getByRole("row", { name: /second\.txt/ })).toBeVisible();
  await expect(queue.getByText("2 files", { exact: true })).toHaveCount(0);
});

test("upload settings use an encryption switch and normalize chunk size", async ({ page }) => {
  await page.goto("/settings/uploads");
  const encryption = page.getByRole("switch", { name: "Cifrar archivos subidos" });
  await expect(encryption).not.toBeChecked();
  await encryption.press("Space");
  await expect(encryption).toBeChecked();

  const partSize = page.getByRole("textbox", { name: /Tamaño preferido de fragmento en MiB/ });
  await expect(partSize).toHaveValue("512");
  await partSize.fill("521");
  await partSize.blur();
  await expect(partSize).toHaveValue("528");
  await expect
    .poll(() =>
      page.evaluate(() => JSON.parse(localStorage.getItem("teldrive.upload-settings.v2") || "{}")),
    )
    .toMatchObject({ encryption: true, preferredPartSize: 528 * 1024 * 1024 });

  await partSize.fill("3000");
  await partSize.blur();
  await expect(partSize).toHaveValue("2048");
  await expect
    .poll(() =>
      page.evaluate(() => JSON.parse(localStorage.getItem("teldrive.upload-settings.v2") || "{}")),
    )
    .toMatchObject({ preferredPartSize: 2048 * 1024 * 1024 });
});

test("upload settings retain an existing valid chunk choice", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem(
      "teldrive.upload-settings.v2",
      JSON.stringify({ preferredPartSize: 640 * 1024 * 1024 }),
    );
  });
  await page.goto("/settings/uploads");
  await expect(
    page.getByRole("textbox", { name: /Tamaño preferido de fragmento en MiB/ }),
  ).toHaveValue("640");
});

test("React Aria file selection supports replacement, ranges, select all, and escape", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop keyboard and modifier behavior");
  await page.goto("/files");
  const alpha = page.getByRole("row", { name: /alpha\.txt/ });
  const beta = page.getByRole("row", { name: /beta\.txt/ });

  await alpha.click();
  await expect(alpha).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("button", { name: "Mover elementos seleccionados", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Renombrar elemento seleccionado" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Duplicar elemento seleccionado" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Descargar archivo seleccionado" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Copiar enlace de descarga del archivo seleccionado" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Cortar elementos seleccionados" })).toBeVisible();
  await beta.click({ modifiers: ["Shift"] });
  await expect(page.getByText("2 seleccionados", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Renombrar elemento seleccionado" })).toBeHidden();
  await expect(page.getByRole("button", { name: "Duplicar elemento seleccionado" })).toBeHidden();
  await expect(
    page.getByRole("button", { name: "Mover elementos seleccionados", exact: true }),
  ).toBeVisible();

  await page.keyboard.press("Control+KeyA");
  await expect(page.getByText("3 seleccionados", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByText(/seleccionados$/)).toBeHidden();
});

test("selected file downloads and copies its attachment URL on an insecure host", async ({
  page,
}) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: () => Promise.reject(new Error("insecure context")) },
    });
    document.execCommand = (command) => {
      if (command !== "copy") return false;
      const selected = document.activeElement;
      if (selected instanceof HTMLTextAreaElement) {
        (window as typeof window & { copiedText?: string }).copiedText = selected.value;
      }
      return true;
    };
  });
  await page.goto("/files");
  await page.getByRole("row", { name: /alpha\.txt/ }).click();

  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Descargar archivo seleccionado" }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("alpha.txt");

  await page
    .getByRole("button", { name: "Copiar enlace de descarga del archivo seleccionado" })
    .click();
  await expect
    .poll(() => page.evaluate(() => (window as typeof window & { copiedText?: string }).copiedText))
    .toBe(`${new URL(page.url()).origin}/api/v1/files/${alphaId}/content/alpha.txt?download=1`);
});

test("React Aria owns directional navigation, range selection, typeahead, and item actions", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop keyboard behavior");
  await page.goto("/files");
  const alpha = page.getByRole("row", { name: /alpha\.txt/ });
  const beta = page.getByRole("row", { name: /beta\.txt/ });

  await alpha.click();
  await page.keyboard.press("ArrowDown");
  await expect(beta).toBeFocused();
  await expect(beta).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("Shift+ArrowUp");
  await expect(page.getByText("2 seleccionados", { exact: true })).toBeVisible();

  await page.keyboard.press("KeyD");
  const destination = page.getByRole("row", { name: /Destino/ });
  await expect(destination).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("navigation", { name: "Carpeta actual" })).toContainText("Destino");
});

test("file operation shortcuts are guarded and update visible state", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop keyboard shortcuts");
  await page.goto("/files");
  await page.getByRole("row", { name: /alpha\.txt/ }).click();

  await page.keyboard.press("F2");
  const rename = page.getByRole("dialog", { name: "Renombrar elemento" });
  await expect(rename).toBeVisible();
  await rename.getByRole("textbox", { name: "Nuevo nombre" }).fill("renamed.txt");
  await rename.getByRole("textbox", { name: "Nuevo nombre" }).press("Enter");
  await expect(page.getByText("renamed.txt", { exact: true })).toBeVisible();

  await page.getByRole("row", { name: /renamed\.txt/ }).click();
  await page.keyboard.press("Delete");
  await expect(page.getByText("renamed.txt", { exact: true })).toBeHidden();

  await page.keyboard.press("Control+Shift+KeyN");
  const createFolder = page.getByRole("dialog", { name: "Crear carpeta" });
  await expect(createFolder).toBeVisible();
  await expect
    .poll(() => createFolder.evaluate((dialog) => dialog.contains(document.activeElement)))
    .toBe(true);
  await page.keyboard.press("Escape");
  await expect(createFolder).toBeHidden();
  await expect(page.getByRole("textbox", { name: "Buscar en esta carpeta" })).toHaveCount(0);
  await expect(page.getByText(/seleccionados$/)).toBeHidden();
});

test("file shortcuts stay within the browser and do not act inside inputs or dialogs", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop keyboard shortcuts");
  await page.goto("/files");
  const alpha = page.getByRole("row", { name: /alpha\.txt/ });
  await alpha.click();
  const search = page.getByRole("textbox", { name: "Buscar archivos" });
  await search.fill("draft");
  await search.press("F2");
  await expect(page.getByRole("dialog", { name: "Renombrar elemento" })).toHaveCount(0);
  await search.press("Control+x");
  await search.press("Delete");
  await expect(alpha).toBeVisible();
  await expect(page.getByText("1 seleccionados", { exact: true })).toBeVisible();
  await alpha.focus();
  await alpha.press("F2");
  const dialog = page.getByRole("dialog", { name: "Renombrar elemento" });
  const name = dialog.getByRole("textbox", { name: "Nuevo nombre" });
  await name.fill("draft.txt");
  await name.press("Control+a");
  await name.press("Delete");
  await expect(name).toHaveValue("");
  await expect(dialog).toBeVisible();
  await name.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(alpha).toBeVisible();
  await expect(page.getByText("1 seleccionados", { exact: true })).toBeVisible();
  await alpha.focus();
  await alpha.press("Escape");
  await expect(page.getByText("1 seleccionados", { exact: true })).toHaveCount(0);
});

test("selected files keep the destination picker alongside clipboard actions", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop move workflow");
  await page.goto("/files");
  await page.getByRole("row", { name: /alpha\.txt/ }).click();
  await expect(
    page.getByRole("button", { name: "Mover elementos seleccionados", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Cortar elementos seleccionados" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Copiar elementos seleccionados" })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Pegar / })).toHaveCount(0);

  await page.getByRole("button", { name: "Mover elementos seleccionados", exact: true }).click();
  const move = page.getByRole("dialog", { name: "Mover 1 elemento" });
  await expect(move).toBeVisible();
  await move.getByRole("row", { name: /Destino/ }).click();
  await move.getByRole("button", { name: "Mover aquí" }).click();
  await expect(move).toBeHidden();
  await expect(page.getByText("alpha.txt", { exact: true })).toBeHidden();

  await page.getByRole("row", { name: /Destino/ }).dblclick();
  await expect(page.getByText("alpha.txt", { exact: true })).toBeVisible();
});

test("split panes cut and copy items directly between folders", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop split view");
  await page.goto("/files");
  await page.getByRole("button", { name: "Abrir vista dividida" }).click();

  const primary = page.getByTestId("file-pane-primary");
  const secondary = page.getByTestId("file-pane-secondary");
  await secondary.getByRole("row", { name: /Destino/ }).dblclick();
  await expect(secondary.getByRole("navigation", { name: "Carpeta actual" })).toContainText(
    "Destino",
  );

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Cortar elementos seleccionados" }).click();
  await expect(
    primary.getByRole("button", { name: /^Pegar 1 elemento del portapapeles$/ }),
  ).toHaveCount(0);
  await expect(secondary.getByText("1 cut", { exact: true })).toBeVisible();
  await expect(secondary.getByRole("button", { name: "Cancelar corte" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Borrar selección" })).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Mover elementos seleccionados", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Mover elementos seleccionados a la papelera" }),
  ).toHaveCount(0);

  await secondary.getByRole("button", { name: "Cancelar corte" }).click();
  await expect(page.getByRole("button", { name: /^Pegar / })).toHaveCount(0);
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeVisible();

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Cortar elementos seleccionados" }).click();
  await secondary.getByRole("button", { name: /^Pegar 1 elemento del portapapeles$/ }).click();
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeHidden();
  await expect(secondary.getByText("alpha.txt", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Pegar / })).toHaveCount(0);

  await secondary.getByRole("row", { name: /alpha\.txt/ }).click();
  await secondary.getByRole("button", { name: "Copiar elementos seleccionados" }).click();
  await expect(
    secondary.getByRole("button", { name: /^Pegar 1 elemento del portapapeles$/ }),
  ).toHaveCount(0);
  await expect(primary.getByText("1 copied", { exact: true })).toBeVisible();
  await primary.getByRole("button", { name: /^Pegar 1 elemento del portapapeles$/ }).click();
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeVisible();
  await expect(secondary.getByText("alpha.txt", { exact: true })).toBeVisible();
  await primary.getByRole("button", { name: "Borrar elementos copiados" }).click();
  await expect(page.getByRole("button", { name: /^Pegar / })).toHaveCount(0);
});

test("cut paste asks before resolving a name conflict", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop split view");
  await page.goto("/files");
  await page.getByRole("button", { name: "Abrir vista dividida" }).click();

  const primary = page.getByTestId("file-pane-primary");
  const secondary = page.getByTestId("file-pane-secondary");
  await secondary.getByRole("row", { name: /Destino/ }).dblclick();

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Copiar elementos seleccionados" }).click();
  await secondary.getByRole("button", { name: /^Pegar 1 elemento del portapapeles$/ }).click();
  await expect(secondary.getByText("alpha.txt", { exact: true })).toBeVisible();
  // The success toast overlaps the action bar and pauses while the pointer is over it.
  await page.getByRole("button", { name: "Cerrar aviso", exact: true }).click();
  await secondary.getByRole("button", { name: "Borrar elementos copiados" }).click();

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Cortar elementos seleccionados" }).click();
  await secondary.getByRole("button", { name: /^Pegar 1 elemento del portapapeles$/ }).click();

  const conflict = page.getByRole("dialog", { name: "El elemento ya existe" });
  await expect(conflict).toBeVisible();
  await expect(conflict.getByRole("button", { name: "Reemplazar" })).toBeVisible();
  await expect(conflict.getByRole("button", { name: "Conservar ambos" })).toBeVisible();
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeVisible();

  await conflict.getByRole("button", { name: "Conservar ambos" }).click();
  await expect(conflict).toBeHidden();
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeHidden();
  await expect(secondary.getByText("alpha.txt", { exact: true })).toBeVisible();
  await expect(secondary.getByText("alpha (1).txt", { exact: true })).toBeVisible();
});

test("split view keeps pane navigation independent and uses browser history", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop split view");
  await page.goto("/files");

  await page.getByRole("button", { name: "Abrir vista dividida" }).click();
  const primary = page.getByTestId("file-pane-primary");
  const secondary = page.getByTestId("file-pane-secondary");
  await expect(primary).toBeVisible();
  await expect(secondary).toBeVisible();
  await expect(secondary.getByRole("button", { name: "Cerrar vista dividida" })).toBeVisible();

  await expect(primary.getByRole("textbox", { name: "Buscar en esta carpeta" })).toHaveCount(0);
  await expect(secondary.getByRole("textbox", { name: "Buscar en esta carpeta" })).toHaveCount(0);
  await expect(secondary.getByRole("button", { name: "Volver" })).toBeVisible();
  await expect(secondary.getByRole("button", { name: "Subir una carpeta" })).toBeDisabled();

  const primaryFolder = primary.getByRole("navigation", { name: "Carpeta actual" });
  const secondaryFolder = secondary.getByRole("navigation", { name: "Carpeta actual" });
  await secondary.getByRole("row", { name: /Destino/ }).dblclick();
  await expect(secondaryFolder).toContainText("Destino");
  await expect(primaryFolder).not.toContainText("Destino");
  await expect(secondary.getByRole("button", { name: "Subir una carpeta" })).toBeEnabled();

  await page.goBack();
  await expect(secondaryFolder).not.toContainText("Destino");
  await expect(primaryFolder).not.toContainText("Destino");
  await expect(page.getByRole("button", { name: "Cerrar vista dividida" })).toBeVisible();

  await page.getByRole("button", { name: "Cerrar vista dividida" }).click();
  await expect(page.getByTestId("file-pane-secondary")).toHaveCount(0);
});

test("touch opens items and exposes an explicit multi-selection control", async ({
  page,
  isMobile,
}) => {
  test.skip(!isMobile, "touch-only file interaction");
  await page.goto("/files");
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth))
    .toBeLessThanOrEqual(1);
  await page
    .getByRole("row", { name: /alpha\.txt/ })
    .locator('[data-slot="checkbox-control"]')
    .tap();
  await expect(page.getByText("1 seleccionados", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Mover elementos seleccionados", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Borrar selección" }).tap();
  await page.getByRole("row", { name: /Destino/ }).tap();
  await expect(page.getByRole("navigation", { name: "Carpeta actual" })).toContainText("Destino");
  await expect(page.getByRole("button", { name: "Subir una carpeta" })).toBeEnabled();
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth))
    .toBeLessThanOrEqual(1);
});
