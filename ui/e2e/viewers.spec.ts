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
        json: {
          userId: 1,
          displayName: "Reader",
          premium: true,
          role: "user",
          capabilities: ["files.read", "files.write", "files.share"],
          createdAt: now,
        },
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
    const content = path.match(/^\/v1\/files\/([^/]+)\/content(?:\/[^/]+)?$/)?.[1];
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

async function openPdfWorkspace(page: Page) {
  await installViewerApi(page, []);
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.pdf");
  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf" });
  await expect(dialog.locator(".pdfViewer .page canvas").first()).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Next PDF page", exact: true })).toBeEnabled();
  return dialog;
}

async function pdfTool(page: Page, name: string) {
  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf" });
  const direct = dialog.getByRole("button", { name, exact: true });
  if (await direct.isVisible()) {
    await direct.click();
    return;
  }
  const tools = dialog.getByRole("button", { name: "PDF reader tools", exact: true });
  await tools.click();
  await page.getByRole("button", { name, exact: true }).click();
  await expect(page.locator('[data-slot="popover-dialog"]')).toHaveCount(0);
}

async function slider(page: Page, name: string, value: number) {
  await expect(page.getByRole("slider", { name, exact: true })).toBeEnabled();
  await page.getByRole("slider", { name, exact: true }).evaluate((element, value) => {
    const input = element as HTMLInputElement;
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, String(value));
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }, value);
}

test("PDF page fields, fit presets, search closing, and menu shortcuts stay consistent", async ({ page }) => {
  const dialog = await openPdfWorkspace(page);
  const input = dialog.getByRole("textbox", { name: "PDF page number", exact: true });
  const viewport = dialog.locator("[data-pdf-viewer-container]");
  const field = input.locator("..");
  const inputBox = (await input.boundingBox())!;
  const fieldBox = (await field.boundingBox())!;
  expect(inputBox.width).toBeLessThanOrEqual(fieldBox.width);
  expect(inputBox.x + inputBox.width).toBeLessThanOrEqual(fieldBox.x + fieldBox.width + 1);
  await input.fill("999");
  await input.press("Enter");
  await expect(input).toHaveValue("2");
  await input.fill("999");
  await input.press("Escape");
  await expect(input).toHaveValue("2");
  await input.fill("0");
  await input.press("Enter");
  await expect(input).toHaveValue("1");
  await input.fill("");
  await input.press("Enter");
  await expect(input).toHaveValue("1");
  await expect.poll(() => viewport.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);

  if ((page.viewportSize()?.width || 0) >= 1024) {
    const before = (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width;
    await dialog.getByRole("button", { name: "Toggle PDF sidebar", exact: true }).click();
    await expect.poll(async () => (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width).toBeGreaterThan(before + 100);
  }
  let manualZoom: number | undefined;
  if ((page.viewportSize()?.width || 0) >= 768) {
    await dialog.getByRole("button", { name: "Zoom in", exact: true }).click();
    manualZoom = (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width;
  }
  await page.setViewportSize({ width: 390, height: 844 });
  if (manualZoom !== undefined) {
    await expect.poll(async () => (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width).toBeCloseTo(manualZoom, 0);
    await pdfTool(page, "Fit width");
  }
  await expect.poll(() => viewport.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
  const tools = dialog.getByRole("button", { name: "PDF reader tools", exact: true });
  await tools.click();
  await page.getByRole("button", { name: "Fit page", exact: true }).focus();
  const beforeArrow = await input.inputValue();
  await page.keyboard.press("ArrowRight");
  await expect(input).toHaveValue(beforeArrow);
  await page.getByRole("button", { name: "Fit page", exact: true }).click();
  await expect(page.locator('[data-slot="popover-dialog"]')).toHaveCount(0);
  await expect.poll(async () => {
    const p = (await dialog.locator(".pdfViewer .page").first().boundingBox())!;
    const c = (await viewport.boundingBox())!;
    return p.height - c.height;
  }).toBeLessThanOrEqual(1);

  await dialog.getByRole("button", { name: "Search in PDF", exact: true }).click();
  await dialog.getByRole("textbox", { name: "Find in PDF", exact: true }).fill("TelDrive");
  await expect(dialog.locator(".textLayer .highlight")).toHaveCount(1);
  await dialog.getByRole("button", { name: "Search in PDF", exact: true }).click();
  await expect(dialog.locator("[data-pdf-findbar]")).toHaveCount(0);
  await expect(dialog.locator(".textLayer .highlight")).toHaveCount(0);
});

test("PDF annotation controls synchronize colors, font size, ink settings, undo, and exports", async ({ page }) => {
  const dialog = await openPdfWorkspace(page);
  await pdfTool(page, "Add text");
  await expect(dialog.locator(".annotationEditorLayer.freetextEditing").first()).toBeVisible();
  await slider(page, "PDF text font size", 18);
  await dialog.getByRole("button", { name: "Use annotation color #60a5fa", exact: true }).click();
  const box = (await dialog.locator(".pdfViewer .page").first().boundingBox())!;
  await page.mouse.click(box.x + box.width * 0.25, box.y + Math.min(box.height * 0.38, 240));
  const editor = dialog.locator(".freeTextEditor [contenteditable=true]").last();
  await expect(editor).toBeVisible();
  await editor.fill("Saved PDF annotation");
  await expect(editor).toHaveCSS("color", "rgb(96, 165, 250)");
  await expect(editor).toHaveAttribute("style", /18px/);

  await pdfTool(page, "Draw");
  await expect(dialog.locator(".annotationEditorLayer.inkEditing").first()).toBeVisible();
  // Text and ink remember their own defaults rather than sharing a stale swatch.
  await expect(dialog.getByRole("button", { name: "Use annotation color #000000", exact: true })).toHaveAttribute("aria-pressed", "true");
  await dialog.getByRole("button", { name: "Use annotation color #60a5fa", exact: true }).click();
  await slider(page, "PDF ink stroke width", 5);
  await slider(page, "PDF ink opacity", 50);
  const inkBox = (await dialog.locator(".pdfViewer .page").first().boundingBox())!;
  await page.mouse.move(inkBox.x + inkBox.width * 0.2, inkBox.y + Math.min(inkBox.height * 0.6, 350));
  await page.mouse.down();
  await page.mouse.move(inkBox.x + inkBox.width * 0.5, inkBox.y + Math.min(inkBox.height * 0.65, 380), { steps: 12 });
  await page.mouse.up();
  const blueStrokes = () => dialog.locator(".pdfViewer .page svg").evaluateAll((elements) => elements.filter((el) => getComputedStyle(el).stroke === "rgb(96, 165, 250)").length);
  await expect.poll(blueStrokes).toBeGreaterThan(0);
  await pdfTool(page, "Undo PDF edit");
  await expect.poll(blueStrokes).toBe(0);
  await pdfTool(page, "Redo PDF edit");
  await expect.poll(blueStrokes).toBeGreaterThan(0);

  const downloaded = page.waitForEvent("download");
  await pdfTool(page, (page.viewportSize()?.width || 0) >= 1280 ? "Save edited PDF copy" : "Save copy");
  const download = await downloaded;
  const exported = readFileSync((await download.path())!).toString("latin1");
  expect(exported).toMatch(/\/Subtype\s*\/FreeText/);
  expect(exported).toMatch(/\/Subtype\s*\/Ink/);
  expect(exported).toMatch(/\/CA\s+0\.5/);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("dialog", { name: "Unsaved PDF changes", exact: true })).toHaveCount(0);
});

test("PDF hand tool pans and closing unsaved edits requires a choice", async ({ page }) => {
  const dialog = await openPdfWorkspace(page);
  const viewport = dialog.locator("[data-pdf-viewer-container]");
  await pdfTool(page, "Hand tool");
  await expect(dialog.locator("[data-pdf-reader]")).toHaveAttribute("data-pdf-hand-active", "true");
  const box = (await viewport.boundingBox())!;
  const start = await viewport.evaluate((el) => el.scrollTop);
  await page.mouse.move(box.x + box.width * 0.7, box.y + box.height * 0.7);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.7, box.y + box.height * 0.7 - 60, { steps: 10 });
  await page.mouse.up();
  await expect.poll(() => viewport.evaluate((el) => el.scrollTop)).toBeGreaterThan(start + 30);
  await pdfTool(page, "Add text");
  await viewport.evaluate((el) => el.scrollTo(0, 0));
  const pageBox = (await dialog.locator(".pdfViewer .page").first().boundingBox())!;
  await page.mouse.click(pageBox.x + pageBox.width * 0.25, pageBox.y + Math.min(pageBox.height * 0.4, 240));
  const editor = dialog.locator(".freeTextEditor [contenteditable=true]").last();
  await editor.fill("Unsaved annotation");
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  const prompt = page.getByRole("dialog", { name: "Unsaved PDF changes", exact: true });
  await expect(prompt).toBeVisible();
  await prompt.getByRole("button", { name: "Keep editing", exact: true }).click();
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await expect(prompt).toBeVisible();
  await prompt.getByRole("button", { name: "Discard changes", exact: true }).click();
  await expect(dialog).toBeHidden();
});

test("PDF save failures stay visible and do not dismiss unsaved changes", async ({ page }) => {
  // Fault-inject the worker's SaveDocument RPC, not the application component.
  await page.addInitScript(() => {
    const NativeWorker = window.Worker;
    window.Worker = class extends NativeWorker {
      postMessage(message: { action?: string; callbackId?: number; sourceName?: string; targetName?: string }, transfer: Transferable[] = []) {
        if (message.action === "SaveDocument") {
          queueMicrotask(() => this.dispatchEvent(new MessageEvent("message", { data: {
            sourceName: message.targetName, targetName: message.sourceName,
            callback: 2, callbackId: message.callbackId,
            reason: { name: "UnknownErrorException", message: "Simulated save failure", details: "Regression test" },
          } })));
          return;
        }
        super.postMessage(message, transfer);
      }
    };
  });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const dialog = await openPdfWorkspace(page);
  await pdfTool(page, "Add text");
  const box = (await dialog.locator(".pdfViewer .page").first().boundingBox())!;
  await page.mouse.click(box.x + box.width * 0.25, box.y + Math.min(box.height * 0.4, 240));
  await dialog.locator(".freeTextEditor [contenteditable=true]").last().fill("Do not lose this edit");
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  const prompt = page.getByRole("dialog", { name: "Unsaved PDF changes", exact: true });
  await prompt.getByRole("button", { name: "Save copy and close", exact: true }).click();
  await expect(prompt.getByRole("alert")).toContainText("Simulated save failure");
  await expect(prompt).toBeVisible();
  await prompt.getByRole("button", { name: "Keep editing", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Simulated save failure");
  expect(errors).toEqual([]);
});

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

  const pageInput = dialog.getByRole("textbox", { name: "PDF page number" });
  await expect(pageInput).toHaveValue("1");
  await dialog.getByRole("button", { name: "Next PDF page" }).click();
  await expect(pageInput).toHaveValue("2");
  await page.keyboard.press("=");
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
    await dialog.getByRole("button", { name: "PDF reader tools" }).click();
    await page.getByRole("button", { name: "Save copy" }).click();
  }
  await expect
    .poll(async () => (await editedDownload).suggestedFilename())
    .toBe("reader-sample-edited.pdf");

  expect(writes).toEqual([]);
  // Escape peels the overlay stack: tool popover, annotation tool, then reader.
  // Mobile has no physical Escape key, so fall back to the close button to
  // keep teardown deterministic once the keyboard path has been exercised.
  for (let attempt = 0; attempt < 5 && (await dialog.isVisible()); attempt += 1) {
    await page.keyboard.press("Escape");
    await page.waitForTimeout(200);
  }
  if (await dialog.isVisible())
    await dialog.getByRole("button", { name: "Close PDF reader" }).click();
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
  expect(writes).toEqual([]);
  await settleBrowserLayout(page);
  expect(errors).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  expect(errors).toEqual([]);
});
