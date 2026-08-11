import { readFileSync } from "node:fs";
import { expect, type Page, test } from "@playwright/test";
import { strToU8, zipSync } from "fflate";

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
type ViewerApiStats = { pdfContentRequests: number };

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

async function settleBrowserLayout(page: Page) {
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );
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

async function installViewerApi(
  page: Page,
  writes: StateWrite[],
  initialStates: Partial<Record<string, Record<string, unknown>>> = {},
  stats?: ViewerApiStats,
) {
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api/, "");
    const method = request.method();
    if (path === "/v1/me") {
      return route.fulfill({
        json: { userId: 1, displayName: "Reader", premium: true, createdAt: now },
      });
    }
    if (path === "/v1/files" && method === "GET") {
      return route.fulfill({ json: { items: files } });
    }
    if (path === "/v1/files/statistics/drive") {
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
    }
    const content = path.match(/^\/v1\/files\/([^/]+)\/content$/)?.[1];
    if (content === pdfId) {
      if (stats) stats.pdfContentRequests += 1;
      return route.fulfill({ body: pdf, contentType: "application/pdf" });
    }
    if (content === epubId) {
      return route.fulfill({ body: epub, contentType: "application/epub+zip" });
    }
    const state = path.match(/^\/v1\/files\/([^/]+)\/view-state$/)?.[1];
    if (state && method === "GET") {
      const initial = initialStates[state];
      return initial
        ? route.fulfill({
            json: {
              fileId: state,
              kind: state === pdfId ? "pdf" : "ebook",
              position: {},
              preferences: {},
              bookmarks: [],
              updatedAt: now,
              ...initial,
            },
          })
        : route.fulfill({ status: 204 });
    }
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
}

async function openFile(page: Page, name: string) {
  const row = page.getByRole("row", { name: new RegExp(name) });
  await expect(row).toBeVisible();
  await row.focus();
  await page.keyboard.press("Enter");
}

test("PDF opens in the Teldrive PDF.js workspace with navigation and search", async ({ page }) => {
  const writes: StateWrite[] = [];
  const errors: string[] = [];
  const stats: ViewerApiStats = { pdfContentRequests: 0 };
  page.on("pageerror", (error) => errors.push(error.message));
  await installViewerApi(
    page,
    writes,
    {
      [pdfId]: {
        position: { pageNumber: 1 },
        preferences: {
          scaleValue: "page-width",
          rotation: 0,
          sidebarOpen: true,
          sidebarTab: "thumbnails",
        },
      },
    },
    stats,
  );
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.pdf");

  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf" });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("[data-pdf-reader]")).toBeVisible();
  await expect(dialog.locator("foliate-view")).toHaveCount(0);
  await expect.poll(() => dialog.locator(".pdfViewer .page").count()).toBe(2);
  await expect(dialog.locator(".pdfViewer .textLayer").first()).toBeVisible();
  const initialContentRequests = stats.pdfContentRequests;
  const initialStateWrites = writes.length;

  const pageInput = dialog.getByRole("textbox", { name: "PDF page number" });
  await expect(pageInput).toHaveValue("1");
  await dialog.getByRole("button", { name: "Next PDF page" }).click();
  await expect(pageInput).toHaveValue("2");
  await page.keyboard.press("=");
  await expect.poll(() => writes.length, { timeout: 2_000 }).toBeGreaterThan(initialStateWrites);
  expect(stats.pdfContentRequests).toBe(initialContentRequests);

  const viewportWidth = page.viewportSize()?.width ?? 0;
  if (viewportWidth >= 1024) {
    await expect(dialog.getByRole("button", { name: "Go to page 2" })).toBeVisible();
  } else {
    await dialog.getByRole("button", { name: "Open PDF sidebar" }).click();
    const navigation = page.getByRole("dialog", { name: "Document navigation" });
    await expect(navigation).toBeVisible();
    await expect(navigation.getByRole("button", { name: "Go to page 2" })).toBeVisible();
    await navigation.getByRole("button", { name: "Close" }).click();
  }

  await dialog.getByRole("button", { name: "Search in PDF" }).click();
  const search = dialog.getByRole("textbox", { name: "Find in PDF" });
  await search.fill("Second Page");
  await expect
    .poll(async () => dialog.locator("[data-pdf-findbar]").innerText())
    .toMatch(/1\s*\/\s*1/);

  await page.keyboard.press("Escape");
  await expect(dialog.locator("[data-pdf-findbar]")).toBeHidden();

  if (viewportWidth >= 1280) {
    await dialog.getByRole("button", { name: "Highlight" }).click();
  } else {
    await dialog.getByRole("button", { name: "PDF reader tools" }).click();
    await page.getByRole("button", { name: "Highlight" }).click();
  }
  await expect(dialog.locator(".textLayer.highlighting").first()).toBeVisible();

  const editedDownload = page.waitForEvent("download");
  if (viewportWidth >= 1280) {
    await dialog.getByRole("button", { name: "Save edited PDF copy" }).click();
  } else {
    await page.getByRole("button", { name: "Save copy" }).click();
  }
  await expect
    .poll(async () => (await editedDownload).suggestedFilename())
    .toBe("reader-sample-edited.pdf");

  await expect.poll(() => writes.some((write) => write.fileId === pdfId)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  expect(errors).toEqual([]);
});

test("mobile EPUB navigation opens in a HeroUI drawer", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const writes: StateWrite[] = [];
  await installViewerApi(page, writes);
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.epub");

  const dialog = page.getByRole("dialog", { name: "reader-sample.epub" });
  await expect(dialog.locator("[data-epub-reader]")).toBeVisible();
  const foliate = dialog.locator("foliate-view");
  await expect(foliate).toHaveAttribute("data-rendered-content", /A Quiet Beginning/);

  const menu = dialog.getByRole("button", { name: "Open ebook navigation" });
  await expect(menu).toBeVisible();
  await menu.click();

  const drawer = page.getByRole("dialog", { name: "Book navigation" });
  await expect(drawer).toBeVisible();
  await expect(drawer.getByRole("listbox", { name: "Table of contents" })).toBeVisible();
  await drawer.getByRole("option", { name: "Across the Cloud" }).click();
  await expect(drawer).toBeHidden();
  await expect(dialog.getByText("Across the Cloud").first()).toBeVisible();
});

test("EPUB renders in its dedicated reader, navigates, persists, and closes cleanly", async ({
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

  await expect(foliate).toBeVisible();
  const headerBox = await dialog.locator("[data-epub-header]").boundingBox();
  const canvasBox = await dialog.locator("[data-epub-canvas]").boundingBox();
  const footerBox = await dialog.locator("[data-epub-footer]").boundingBox();
  expect(headerBox && canvasBox && footerBox).toBeTruthy();
  expect(canvasBox!.y).toBeGreaterThanOrEqual(headerBox!.y + headerBox!.height);
  expect(canvasBox!.y + canvasBox!.height).toBeLessThanOrEqual(footerBox!.y);

  const viewportWidth = page.viewportSize()?.width ?? 0;
  if (viewportWidth >= 1024) {
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
  await expect.poll(() => writes.some((write) => write.fileId === epubId)).toBe(true);
  await settleBrowserLayout(page);
  expect(errors).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  expect(errors).toEqual([]);
});
