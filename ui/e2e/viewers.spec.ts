import { readFileSync } from "node:fs";
import { expect, type Page, test } from "@playwright/test";
import { strToU8, zipSync } from "fflate";
import { installPreferencesApi } from "./fixtures/preferences";
import { makeLargePdf } from "./fixtures/viewers/large-pdf";

const now = "2026-08-01T12:00:00Z";
const pdfId = "71111111-1111-4111-8111-111111111111";
const epubId = "72222222-2222-4222-8222-222222222222";
const pdf = readFileSync(new URL("./fixtures/viewers/sample.pdf", import.meta.url));
const epub = makeEpub();
const files = [
  file(pdfId, "reader-sample.pdf", "application/pdf", pdf.byteLength),
  file(epubId, "reader-sample.epub", "application/epub+zip", epub.byteLength),
];
type StateWrite = { fileId: string; body: Record<string, unknown> };

function file(id: string, name: string, mimeType: string, size: number) {
  return {
    id,
    name,
    kind: "file",
    status: "active",
    generation: 1,
    mimeType,
    size,
    modTime: now,
    createdAt: now,
    updatedAt: now,
    encryption: true,
  };
}

function makeEpub() {
  const fixture = (path: string) =>
    readFileSync(new URL(`./fixtures/viewers/epub-src/${path}`, import.meta.url));
  return Buffer.from(
    zipSync({
      mimetype: [strToU8("application/epub+zip"), { level: 0 }],
      "META-INF/container.xml": fixture("META-INF/container.xml"),
      "OEBPS/content.opf": fixture("OEBPS/content.opf"),
      "OEBPS/nav.xhtml": fixture("OEBPS/nav.xhtml"),
      "OEBPS/chapter-one.xhtml": fixture("OEBPS/chapter-one.xhtml"),
      "OEBPS/chapter-two.xhtml": fixture("OEBPS/chapter-two.xhtml"),
    }),
  );
}

async function installViewerApi(page: Page, writes: StateWrite[]) {
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api/, "");
    const method = request.method();
    if (path === "/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Reader",
          premium: true,
          role: "user",
          capabilities: ["files.read", "files.write", "files.share"],
          createdAt: now,
        },
      });
    if (path === "/v1/files" && method === "GET") return route.fulfill({ json: { items: files } });
    if (path === "/v1/files/statistics/drive")
      return route.fulfill({
        json: {
          totalFiles: 2,
          totalFolders: 0,
          totalBytes: pdf.byteLength + epub.byteLength,
          trashedFiles: 0,
          activeShares: 0,
          openUploads: 0,
        },
      });
    const content = path.match(/^\/v1\/files\/([^/]+)\/content(?:\/[^/]+)?$/)?.[1];
    if (content === pdfId)
      return route.fulfill({
        body: pdf,
        contentType: "application/pdf",
        headers: new URL(request.url()).searchParams.has("download")
          ? { "content-disposition": 'attachment; filename="reader-sample.pdf"' }
          : {},
      });
    if (content === epubId)
      return route.fulfill({ body: epub, contentType: "application/epub+zip" });
    const state = path.match(/^\/v1\/files\/([^/]+)\/view-state$/)?.[1];
    if (state && method === "GET") return route.fulfill({ status: 204 });
    if (state && method === "PUT") {
      const body = request.postDataJSON() as Record<string, unknown>;
      writes.push({ fileId: state, body });
      return route.fulfill({ json: { fileId: state, ...body, updatedAt: now } });
    }
    return route.fulfill({
      status: 404,
      json: { error: { code: "not_found", message: `${method} ${path}` } },
    });
  });
  await installPreferencesApi(page);
}

async function openFile(page: Page, name: string) {
  const row = page.getByRole("row", { name: new RegExp(name) });
  await expect(row).toBeVisible();
  await row.focus();
  await page.keyboard.press("Enter");
}

async function settleBrowserLayout(page: Page) {
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );
}

async function openPdf(page: Page, theme: "light" | "dark" | "system" = "system") {
  await installViewerApi(page, []);
  await installPreferencesApi(page, { appearance: { theme } });
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.pdf");
  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf", exact: true });
  await expect
    .poll(
      () =>
        dialog
          .locator('[data-pdf-page="1"] img')
          .first()
          .evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0),
      { timeout: 45_000 },
    )
    .toBe(true);
  return dialog;
}

test("PDFium WASM opens a read-only reader with navigation, search, zoom and thumbnails", async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  const requests: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("request", (request) => requests.push(request.url()));
  const dialog = await openPdf(page);
  await expect(dialog.locator("iframe,.pdfViewer,[contenteditable=true]")).toHaveCount(0);
  const input = dialog.getByRole("textbox", { name: "PDF page number", exact: true });
  await expect(input).toHaveValue("1");
  await dialog.getByRole("button", { name: "Next PDF page", exact: true }).click();
  await expect(input).toHaveValue("2");
  await input.fill("1");
  await input.press("Enter");
  await expect(input).toHaveValue("1");
  const zoom = dialog.getByRole("combobox", { name: "PDF zoom options" });
  await zoom.selectOption("1.5");
  await expect(zoom.locator('option[value=""]')).toHaveText("150%");
  await zoom.selectOption("width");
  await dialog.getByRole("button", { name: "Search in PDF", exact: true }).click();
  await dialog.getByRole("textbox", { name: "Find in PDF", exact: true }).fill("Second Page");
  await expect(dialog.locator("[data-pdf-findbar] output")).toHaveText("1 / 1", {
    timeout: 15_000,
  });
  await dialog.getByRole("button", { name: "Close PDF search", exact: true }).click();
  await dialog.getByRole("button", { name: "Toggle PDF sidebar", exact: true }).first().click();
  await dialog.getByRole("button", { name: "Go to page 1", exact: true }).click();
  await expect(input).toHaveValue("1");
  await testInfo.attach("pdf-reader", {
    body: await dialog.screenshot({ path: testInfo.outputPath("pdf-reader.png") }),
    contentType: "image/png",
  });
  expect(requests.some((url) => url.includes(".wasm"))).toBe(true);
  expect(
    requests.filter((url) => /pdfcraft|pdfjs|github\.com|cdn\.jsdelivr\.net|unpkg\.com/.test(url)),
  ).toEqual([]);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(errors).toEqual([]);
});

for (const theme of ["light", "dark"] as const) {
  test(`PDF reader follows explicit ${theme} theme regardless of the OS`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: theme === "light" ? "dark" : "light" });
    const dialog = await openPdf(page, theme);
    await expect(dialog.locator("[data-pdf-reader]")).toHaveAttribute("data-reader-theme", theme);
    const colors = await dialog.locator("[data-pdf-header]").evaluate((element) => {
      const reference = document.createElement("span");
      reference.style.backgroundColor = "var(--surface)";
      element.append(reference);
      const surface = getComputedStyle(reference).backgroundColor;
      reference.remove();
      return { header: getComputedStyle(element).backgroundColor, surface };
    });
    expect(colors.header).toBe(colors.surface);
    await expect(dialog.locator(".pdf-reader")).toHaveCSS("--epdf-page-background", "#fff");
  });
}

test("PDF reader changes System theme live without reopening or resetting the document", async ({
  page,
}) => {
  let contentRequests = 0;
  page.on("request", (request) => {
    if (request.url().includes(`/files/${pdfId}/content/`)) contentRequests += 1;
  });
  await page.emulateMedia({ colorScheme: "light" });
  const dialog = await openPdf(page);
  await dialog.getByRole("button", { name: "Next PDF page", exact: true }).click();
  const input = dialog.getByRole("textbox", { name: "PDF page number", exact: true });
  await expect(input).toHaveValue("2");
  const initialRequests = contentRequests;
  const header = dialog.locator("[data-pdf-header]");
  const light = await header.evaluate((element) => getComputedStyle(element).backgroundColor);
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(dialog.locator("[data-pdf-reader]")).toHaveAttribute("data-reader-theme", "dark");
  expect(await header.evaluate((element) => getComputedStyle(element).backgroundColor)).not.toBe(
    light,
  );
  await expect(input).toHaveValue("2");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(dialog.locator("[data-pdf-reader]")).toHaveAttribute("data-reader-theme", "light");
  await expect(input).toHaveValue("2");
  expect(contentRequests).toBe(initialRequests);
});

test("PDF content errors are visible and retry recovers without a fallback", async ({ page }) => {
  await installViewerApi(page, []);
  let fail = true;
  await page.route(`**/files/${pdfId}/content/**`, (route) =>
    fail
      ? route.fulfill({ status: 503 })
      : route.fulfill({ body: pdf, contentType: "application/pdf" }),
  );
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.pdf");
  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf", exact: true });
  await expect(dialog.getByRole("alert")).toContainText("HTTP 503", { timeout: 45_000 });
  fail = false;
  await dialog.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "Next PDF page", exact: true })).toBeEnabled({
    timeout: 45_000,
  });
  await expect(dialog.getByRole("alert")).toHaveCount(0);
});

test("PDF text can be selected and copied without enabling editing", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const dialog = await openPdf(page);
  const box = (await dialog.locator('[data-pdf-page="1"]').boundingBox())!;
  // The fixture's first word starts at (72, 680) in 612×792 PDF coordinates.
  await page.mouse.dblclick(box.x + (box.width * 85) / 612, box.y + (box.height * 102) / 792);
  await dialog.getByRole("button", { name: "Copy", exact: true }).click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("TelDrive");
  await expect(dialog.locator('[contenteditable="true"]')).toHaveCount(0);
});

test("PDF thumbnails and main pages stay virtualized in a long document", async ({ page }) => {
  await installViewerApi(page, []);
  await page.route(`**/files/${pdfId}/content/**`, (route) =>
    route.fulfill({ body: makeLargePdf(240), contentType: "application/pdf" }),
  );
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.pdf");
  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf", exact: true });
  const input = dialog.getByRole("textbox", { name: "PDF page number", exact: true });
  await expect(input).toBeVisible({ timeout: 45_000 });
  await input.fill("240");
  await input.press("Enter");
  await expect(input).toHaveValue("240");
  await expect(dialog.locator('[data-pdf-page="240"] img').first()).toBeVisible({
    timeout: 20_000,
  });
  expect(await dialog.locator("[data-pdf-page]").count()).toBeLessThan(15);
  await dialog.getByRole("button", { name: "Toggle PDF sidebar", exact: true }).first().click();
  await expect(dialog.getByRole("button", { name: "Go to page 240", exact: true })).toBeVisible();
  expect(await dialog.getByRole("button", { name: /^Go to page/ }).count()).toBeLessThan(20);
});

test("PDF download link targets the untouched original and read-only interactions never upload", async ({
  page,
}) => {
  const mutations: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") mutations.push(request.url());
  });
  const dialog = await openPdf(page);
  await expect(
    dialog.getByRole("button", { name: /highlight|add text|draw|save|edit/i }),
  ).toHaveCount(0);
  const download = dialog.getByRole("link", { name: "Download original PDF" });
  await expect(download).toHaveAttribute(
    "href",
    `/api/v1/files/${pdfId}/content/reader-sample.pdf?download=1`,
  );
  await expect(download).toHaveAttribute("download", "reader-sample.pdf");
  // Native Chromium downloads bypass page.route mocks. Fetch the actual anchor's
  // target through the mock API to verify that it serves the original bytes.
  const bytes = await download.evaluate(async (anchor: HTMLAnchorElement) => {
    const response = await fetch(anchor.href);
    return [...new Uint8Array(await response.arrayBuffer())];
  });
  expect(Buffer.from(bytes)).toEqual(pdf);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  expect(mutations).toEqual([]);
});

test("EPUB brightness is display-only and Auto theme follows the system", async ({ page }) => {
  const writes: StateWrite[] = [];
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.stack || error.message));
  await installViewerApi(page, writes);
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.epub");
  const dialog = page.getByRole("dialog", { name: "reader-sample.epub" });
  const reader = dialog.locator("[data-epub-reader]");
  await expect(dialog.locator("foliate-view")).toHaveAttribute(
    "data-rendered-content",
    /A Quiet Beginning/,
  );
  await expect(reader).toHaveAttribute("data-reader-theme", "night");
  await dialog.getByRole("button", { name: "Reading settings", exact: true }).click();
  const appearance = page.getByRole("dialog", { name: "Reading appearance" });
  const brightness = appearance.getByRole("slider", { name: "Brightness", exact: true });
  await brightness.focus();
  await brightness.press("Home");
  for (let press = 0; press < 16; press += 1) await brightness.press("ArrowRight");
  await expect(brightness).toHaveValue("130");
  await expect(dialog.locator("[data-page-brightness='130']")).toHaveAttribute(
    "style",
    /brightness\(1\.3\)/,
  );
  await appearance.getByRole("button", { name: "Done" }).click();
  await expect(appearance).toBeHidden();
  await page.emulateMedia({ colorScheme: "light" });
  await expect(reader).toHaveAttribute("data-reader-theme", "paper");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(reader).toHaveAttribute("data-reader-theme", "night");
  await dialog.getByRole("button", { name: "Close ebook reader", exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(writes).toEqual([]);
  expect(errors).toEqual([]);
});

test("mobile EPUB navigation opens in a HeroUI drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await installViewerApi(page, []);
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.epub");
  const dialog = page.getByRole("dialog", { name: "reader-sample.epub" });
  await expect(dialog.locator("[data-epub-reader]")).toBeVisible();
  await expect(dialog.locator("foliate-view")).toHaveAttribute(
    "data-rendered-content",
    /A Quiet Beginning/,
  );
  await dialog.getByRole("button", { name: "Open ebook navigation" }).click();
  const drawer = page.getByRole("dialog", { name: "Book navigation" });
  await expect(drawer).toBeVisible();
  await expect(drawer.getByRole("listbox", { name: "Table of contents" })).toBeVisible();
  await drawer.getByRole("option", { name: "Across the Cloud" }).click();
  await expect(drawer).toBeHidden();
  await expect(dialog.getByText("Across the Cloud").first()).toBeVisible();
});

test("EPUB renders in its dedicated reader, navigates without persisting, and closes cleanly", async ({
  page,
}) => {
  const writes: StateWrite[] = [];
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.stack || error.message));
  await installViewerApi(page, writes);
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.epub");
  const dialog = page.getByRole("dialog", { name: "reader-sample.epub" });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("[data-epub-reader]")).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Open ebook navigation" })).toBeVisible();
  await expect(dialog.getByText("TelDrive Reader Fixture").first()).toBeVisible();
  const foliate = dialog.locator("foliate-view");
  await expect
    .poll(() =>
      foliate.evaluate(
        (element) =>
          `${String(element.book?.metadata?.title || "")}:${element.book?.sections?.length ?? 0}`,
      ),
    )
    .toBe("TelDrive Reader Fixture:2");
  await expect(foliate).toHaveAttribute("data-rendered-content", /A Quiet Beginning/);
  await expect
    .poll(() =>
      foliate.evaluate((element) => ({
        flow: element.renderer?.getAttribute("flow"),
        gap: element.renderer?.getAttribute("gap"),
        columns: element.renderer?.getAttribute("max-column-count"),
      })),
    )
    .toEqual({ flow: "paginated", gap: "6%", columns: "2" });
  const headerBox = await dialog.locator("[data-epub-header]").boundingBox();
  const canvasBox = await dialog.locator("[data-epub-canvas]").boundingBox();
  const footerBox = await dialog.locator("[data-epub-footer]").boundingBox();
  expect(headerBox && canvasBox && footerBox).toBeTruthy();
  expect(canvasBox!.y).toBeGreaterThanOrEqual(headerBox!.y + headerBox!.height);
  expect(canvasBox!.y + canvasBox!.height).toBeLessThanOrEqual(footerBox!.y);
  if ((page.viewportSize()?.width ?? 0) >= 1024) {
    const canvasBefore = await dialog.locator("[data-epub-canvas]").boundingBox();
    await dialog.getByRole("button", { name: "Open ebook navigation" }).click();
    const sidebar = dialog.locator("[data-epub-sidebar]");
    await expect(sidebar).toBeVisible();
    await expect(sidebar.getByRole("listbox", { name: "Table of contents" })).toBeVisible();
    const canvasAfter = await dialog.locator("[data-epub-canvas]").boundingBox();
    expect(canvasBefore && canvasAfter).toBeTruthy();
    expect(canvasAfter!.x).toBeGreaterThan(canvasBefore!.x);
    await sidebar.getByRole("option", { name: "Across the Cloud" }).click();
  }
  await settleBrowserLayout(page);
  expect(errors).toEqual([]);
  await dialog.getByRole("button", { name: "Reading settings" }).click();
  await page.getByRole("button", { name: "Night" }).click();
  await expect(dialog.locator("[data-epub-reader]")).toHaveAttribute("data-reader-theme", "night");
  const appearance = page.getByRole("dialog", { name: "Reading appearance" });
  await appearance.getByRole("button", { name: "Done" }).click();
  await expect(appearance).toBeHidden();
  await settleBrowserLayout(page);
  expect(errors).toEqual([]);
  await dialog.getByRole("button", { name: "Next page" }).click();
  expect(writes).toEqual([]);
  await settleBrowserLayout(page);
  expect(errors).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  expect(errors).toEqual([]);
});
