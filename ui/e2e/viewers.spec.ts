import { readFileSync } from "node:fs";
import { expect, type Page, type Route, test } from "@playwright/test";
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

async function seedDocumentSettings(page: Page) {
  await page.addInitScript(() => {
    if (window === parent && !localStorage.getItem("teldrive.document-settings.v1")) {
      localStorage.setItem(
        "teldrive.document-settings.v1",
        JSON.stringify({ cacheEnabled: true, cacheLimitMiB: 32, pdfBrightness: 100 }),
      );
    }
  });
}

async function cachedDocuments(page: Page) {
  return page.evaluate(async () => {
    if (!(await indexedDB.databases()).some((db) => db.name === "teldrive-documents-v1")) return [];
    return new Promise<Array<{ key: string; etag: string; size: number }>>((resolve, reject) => {
      const request = indexedDB.open("teldrive-documents-v1", 1);
      request.onerror = () => reject(request.error);
      request.onsuccess = () => {
        const db = request.result;
        const tx = db.transaction("metadata");
        const entries = tx.objectStore("metadata").getAll();
        entries.onsuccess = () => resolve(entries.result);
        entries.onerror = () => reject(entries.error);
        tx.oncomplete = () => db.close();
      };
    });
  });
}

async function openPreparedPdf(page: Page) {
  await openFile(page, "reader-sample.pdf");
  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf", exact: true });
  await expect(dialog.getByRole("button", { name: "Next PDF page", exact: true })).toBeEnabled({
    timeout: 45_000,
  });
  return dialog;
}

test("PDF brightness dims only the page and persists without reopening the document", async ({
  page,
}) => {
  let downloads = 0;
  page.on("request", (request) => {
    if (request.url().includes(`/files/${pdfId}/content/`)) downloads++;
  });
  const dialog = await openPdf(page);
  const initialDownloads = downloads;
  await dialog.getByRole("button", { name: "Next PDF page", exact: true }).click();
  await dialog.getByRole("button", { name: "Brightness", exact: true }).click();
  const slider = page.getByRole("slider", { name: "PDF page brightness", exact: true });
  await slider.focus();
  await slider.press("Home");
  await expect(slider).toHaveValue("50");
  await expect(dialog.locator("[data-pdf-reader]")).toHaveAttribute("data-pdf-brightness", "50");
  await expect(dialog.locator("[data-pdf-page-content]").first()).toHaveCSS(
    "filter",
    "brightness(0.5)",
  );
  await expect(dialog.locator("[data-pdf-header]")).toHaveCSS("filter", "none");
  await expect(dialog.locator("[data-pdf-toolbar]")).toHaveCSS("filter", "none");
  await expect(dialog.getByRole("textbox", { name: "PDF page number", exact: true })).toHaveValue(
    "2",
  );
  expect(downloads).toBe(initialDownloads);
  await slider.press("Escape");
  await expect(slider).toBeHidden();
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await page.reload();
  const reopened = await openPreparedPdf(page);
  await expect(reopened.locator("[data-pdf-reader]")).toHaveAttribute("data-pdf-brightness", "50");
});

test("document content cache follows settings despite HTTP no-store, revalidates and replaces stale bytes", async ({
  page,
}) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  let etag = '"v1"';
  let body = pdf;
  let downloads = 0;
  const conditionals: Array<string | undefined> = [];
  await page.route(`**/files/${pdfId}/content/**`, (route) => {
    const conditional = route.request().headers()["if-none-match"];
    conditionals.push(conditional);
    if (conditional === etag)
      return route.fulfill({
        status: 304,
        headers: { ETag: etag, "Cache-Control": "private, no-store" },
      });
    downloads++;
    return route.fulfill({
      body,
      contentType: "application/pdf",
      headers: { ETag: etag, "Cache-Control": "private, no-store" },
    });
  });
  await page.goto("/files?view=list");
  let dialog = await openPreparedPdf(page);
  expect(await cachedDocuments(page)).toHaveLength(1);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await page.reload();
  dialog = await openPreparedPdf(page);
  expect(downloads).toBe(1);
  expect(conditionals).toEqual([undefined, '"v1"']);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  // The file list deliberately still has its old generation. Server validation wins.
  etag = '"v2"';
  body = Buffer.from(pdf.toString().replace("TelDrive PDF Reader", "NewDrive PDF Reader"));
  dialog = await openPreparedPdf(page);
  expect(downloads).toBe(2);
  await dialog.getByRole("button", { name: "Search in PDF", exact: true }).click();
  await dialog.getByRole("textbox", { name: "Find in PDF", exact: true }).fill("NewDrive");
  await expect(dialog.locator("[data-pdf-findbar] output")).toHaveText("1 / 1");
  expect((await cachedDocuments(page))[0].etag).toBe('"v2"');
});

test("document content cache also reuses ebooks without redownloading them", async ({ page }) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  let downloads = 0;
  await page.route(`**/files/${epubId}/content/**`, (route) => {
    if (route.request().headers()["if-none-match"] === '"book-v1"')
      return route.fulfill({ status: 304, headers: { ETag: '"book-v1"' } });
    downloads++;
    return route.fulfill({
      body: epub,
      contentType: "application/epub+zip",
      headers: { ETag: '"book-v1"' },
    });
  });
  await page.goto("/files?view=list");
  for (let i = 0; i < 2; i++) {
    await openFile(page, "reader-sample.epub");
    const dialog = page.getByRole("dialog", { name: "reader-sample.epub", exact: true });
    await expect(dialog.locator("foliate-view")).toHaveAttribute(
      "data-rendered-content",
      /A Quiet Beginning/,
    );
    await dialog.getByRole("button", { name: "Close ebook reader", exact: true }).click();
    await expect(dialog).toBeHidden();
  }
  expect(downloads).toBe(1);
  expect(await cachedDocuments(page)).toHaveLength(1);
});

test("document cache never serves cached bytes after access is revoked", async ({ page }) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  let denied = false;
  await page.route(`**/files/${pdfId}/content/**`, (route) =>
    denied
      ? route.fulfill({ status: 403 })
      : route.fulfill({
          body: pdf,
          contentType: "application/pdf",
          headers: { ETag: '"private"' },
        }),
  );
  await page.goto("/files?view=list");
  const dialog = await openPreparedPdf(page);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  denied = true;
  await openFile(page, "reader-sample.pdf");
  await expect(page.getByRole("alert")).toContainText("HTTP 403");
  await expect(page.locator("[data-pdf-page]")).toHaveCount(0);
  expect(await cachedDocuments(page)).toHaveLength(0);
});

test("documents still open when browser storage is unavailable", async ({ page }) => {
  await seedDocumentSettings(page);
  await page.addInitScript(() => {
    indexedDB.open = () => {
      throw new Error("Storage disabled");
    };
  });
  await installViewerApi(page, []);
  await page.route(`**/files/${pdfId}/content/**`, (route) =>
    route.fulfill({ body: pdf, contentType: "application/pdf", headers: { ETag: '"v1"' } }),
  );
  await page.goto("/files?view=list");
  const dialog = await openPreparedPdf(page);
  await expect(dialog.getByRole("alert")).toHaveCount(0);
});

test("documents still open when the browser document cache is full", async ({ page }) => {
  await seedDocumentSettings(page);
  await page.addInitScript(() => {
    const put = IDBObjectStore.prototype.put;
    IDBObjectStore.prototype.put = function (...args) {
      if (this.name === "content") throw new DOMException("Cache full", "QuotaExceededError");
      return put.apply(this, args);
    };
  });
  await installViewerApi(page, []);
  await page.route(`**/files/${pdfId}/content/**`, (route) =>
    route.fulfill({ body: pdf, contentType: "application/pdf", headers: { ETag: '"v1"' } }),
  );
  await page.goto("/files?view=list");
  const dialog = await openPreparedPdf(page);
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  expect(await cachedDocuments(page)).toHaveLength(0);
});

test("document cache settings clear stored content and disabling stops caching", async ({
  page,
}) => {
  await installViewerApi(page, []);
  let downloads = 0;
  await page.route(`**/files/${pdfId}/content/**`, (route) => {
    downloads++;
    return route.fulfill({ body: pdf, contentType: "application/pdf", headers: { ETag: '"v1"' } });
  });
  await page.goto("/settings/appearance");
  const toggle = page.getByRole("switch", { name: "Keep documents in this browser", exact: true });
  await expect(toggle).not.toBeChecked();
  await toggle.focus();
  await toggle.press("Space");
  await expect(toggle).toBeChecked();
  await page.getByRole("combobox", { name: "Document cache size", exact: true }).selectOption("32");
  await page.goto("/files?view=list");
  let dialog = await openPreparedPdf(page);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await page.goto("/settings/appearance");
  await expect(page.getByLabel("Cached documents", { exact: true })).toContainText("1 documents");
  await page.getByRole("button", { name: "Clear document cache", exact: true }).click();
  await expect(page.getByLabel("Cached documents", { exact: true })).toContainText("0 documents");
  await expect(toggle).toBeEnabled();
  await toggle.focus();
  await toggle.press("Space");
  await expect(toggle).not.toBeChecked();
  await page.goto("/files?view=list");
  dialog = await openPreparedPdf(page);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await openPreparedPdf(page);
  expect(downloads).toBe(3);
  expect(await cachedDocuments(page)).toHaveLength(0);
});

test("clearing document cache in another tab prevents an in-flight fetch from restoring it", async ({
  page,
  context,
}) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  let capture!: (route: Route) => void;
  const pending = new Promise<Route>((resolve) => {
    capture = resolve;
  });
  await page.route(`**/files/${pdfId}/content/**`, (route) => {
    capture(route);
  });
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.pdf");
  const request = await pending;
  const settings = await context.newPage();
  await installViewerApi(settings, []);
  await settings.goto("/settings/appearance");
  await settings.getByRole("button", { name: "Clear document cache", exact: true }).click();
  await expect(
    settings.getByRole("button", { name: "Clear document cache", exact: true }),
  ).toBeEnabled();
  await expect(settings.getByLabel("Cached documents", { exact: true })).toContainText(
    "0 documents",
  );
  await request.fulfill({ body: pdf, contentType: "application/pdf", headers: { ETag: '"v1"' } });
  await expect(page.getByRole("button", { name: "Next PDF page", exact: true })).toBeEnabled({
    timeout: 45_000,
  });
  expect(await cachedDocuments(page)).toHaveLength(0);
  await settings.close();
});

test("document cache partitions accounts and logout clears all cached bytes", async ({ page }) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  let userId = 1;
  let authenticated = true;
  const conditionals: Array<string | undefined> = [];
  await page.route("**/api/v1/me", (route) =>
    authenticated
      ? route.fulfill({
          json: {
            userId,
            displayName: "Reader",
            role: "user",
            premium: true,
            capabilities: ["files.read", "files.write"],
            createdAt: now,
          },
        })
      : route.fulfill({ status: 401 }),
  );
  await page.route("**/api/v1/auth/cookie/logout", (route) => {
    authenticated = false;
    return route.fulfill({ status: 204 });
  });
  await page.route(`**/files/${pdfId}/content/**`, (route) => {
    conditionals.push(route.request().headers()["if-none-match"]);
    return route.fulfill({ body: pdf, contentType: "application/pdf", headers: { ETag: '"v1"' } });
  });
  await page.goto("/files?view=list");
  let dialog = await openPreparedPdf(page);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  userId = 2;
  await page.reload();
  dialog = await openPreparedPdf(page);
  expect(conditionals).toEqual([undefined, undefined]);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await page.goto("/settings");
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  await expect(page).toHaveURL(/\/login/);
  expect(await cachedDocuments(page)).toHaveLength(0);
});

test("document content without an ETag is not persisted", async ({ page }) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  const conditionals: Array<string | undefined> = [];
  await page.route(`**/files/${pdfId}/content/**`, (route) => {
    conditionals.push(route.request().headers()["if-none-match"]);
    return route.fulfill({ body: pdf, contentType: "application/pdf" });
  });
  await page.goto("/files?view=list");
  for (let i = 0; i < 2; i++) {
    const dialog = await openPreparedPdf(page);
    await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  }
  expect(conditionals).toEqual([undefined, undefined]);
  expect(await cachedDocuments(page)).toHaveLength(0);
});

test("document cache evicts least recently opened content when its size limit shrinks", async ({
  page,
}) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  await page.goto("/settings/appearance");
  const limit = page.getByRole("combobox", { name: "Document cache size", exact: true });
  await limit.selectOption("64");
  await expect(limit).toBeEnabled();
  await page.evaluate(
    () =>
      new Promise<void>((resolve, reject) => {
        const request = indexedDB.open("teldrive-documents-v1", 1);
        request.onerror = () => reject(request.error);
        request.onsuccess = () => {
          const db = request.result;
          const tx = db.transaction(["metadata", "content"], "readwrite");
          const blob = new Blob([new Uint8Array(17 * 1024 * 1024)]);
          for (const [key, usedAt] of [
            ["old-document", 1],
            ["recent-document", 2],
          ] as const) {
            tx.objectStore("metadata").put({ key, etag: '"v1"', size: blob.size, usedAt });
            tx.objectStore("content").put(blob, key);
          }
          tx.oncomplete = () => {
            db.close();
            resolve();
          };
          tx.onerror = () => {
            db.close();
            reject(tx.error);
          };
        };
      }),
  );
  await limit.selectOption("32");
  await expect(limit).toBeEnabled();
  await expect
    .poll(async () => (await cachedDocuments(page)).map((entry) => entry.key))
    .toEqual(["recent-document"]);
});

test("public-share PDF content never enters the signed-in document cache", async ({ page }) => {
  await seedDocumentSettings(page);
  await installViewerApi(page, []);
  const token = "tds_cachePreview123";
  const prefix = `/api/v1/public/shares/${token}`;
  let downloads = 0;
  await page.route(`**${prefix}**`, (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === prefix)
      return route.fulfill({
        json: { id: pdfId, file: files[0], passwordProtected: false, permission: "read" },
      });
    if (path.includes("/content/")) {
      expect(route.request().headers()["if-none-match"]).toBeUndefined();
      downloads++;
      return route.fulfill({
        body: pdf,
        contentType: "application/pdf",
        headers: { ETag: '"share-v1"' },
      });
    }
    return route.fulfill({ status: 404 });
  });
  await page.goto(`/share/${token}`);
  for (let i = 0; i < 2; i++) {
    await page.getByTitle("reader-sample.pdf", { exact: true }).dblclick();
    const dialog = page.getByRole("dialog", { name: "reader-sample.pdf", exact: true });
    await expect(dialog.getByRole("button", { name: "Next PDF page", exact: true })).toBeEnabled({
      timeout: 45_000,
    });
    await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  }
  expect(downloads).toBe(2);
  expect(await cachedDocuments(page)).toHaveLength(0);
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
