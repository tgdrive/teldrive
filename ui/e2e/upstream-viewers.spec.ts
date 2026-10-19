// Regression coverage retained from tgdrive/teldrive main (67bf180).
import { readFileSync } from "node:fs";
import { expect, type Page, test } from "@playwright/test";
import { strToU8, zipSync } from "fflate";
import { installPreferencesApi } from "./fixtures/preferences";
import { makeLargePdf } from "./fixtures/viewers/large-pdf";

const now = "2026-08-01T12:00:00Z";
const pdfId = "71111111-1111-4111-8111-111111111111";
const epubId = "72222222-2222-4222-8222-222222222222";
const outlinePdfId = "73333333-3333-4333-8333-333333333333";
const pdf = readFileSync(new URL("./fixtures/viewers/sample.pdf", import.meta.url));
const outlinePdf = readFileSync(new URL("./fixtures/viewers/outline.pdf", import.meta.url));
const epub = makeEpub();

const files = [
  file(pdfId, "reader-sample.pdf", "application/pdf", pdf.byteLength),
  file(outlinePdfId, "outline-sample.pdf", "application/pdf", outlinePdf.byteLength),
  file(epubId, "reader-sample.epub", "application/epub+zip", epub.byteLength),
];

type StateWrite = { fileId: string; body: Record<string, unknown> };
type ViewerApiStats = { pdfContentRequests: number };

// Unlike programmatic scrollTop updates, wheel events exercise the browser's
// normal asynchronous scrolling path (including compositor scheduling).
async function measureWheelScrolling(page: Page, selector: string) {
  const target = page.locator(selector);
  await target.hover();
  const measurement = target.evaluate(async (element) => {
    const frames: number[] = [];
    const from = element.scrollTop;
    let previous = performance.now();
    const start = previous;
    await new Promise<void>((resolve) => {
      const tick = (now: number) => {
        frames.push(now - previous);
        previous = now;
        if (now - start < 1800) requestAnimationFrame(tick);
        else resolve();
      };
      requestAnimationFrame(tick);
    });
    frames.sort((a, b) => a - b);
    return {
      p95FrameMs: frames[Math.floor(frames.length * 0.95)],
      frames: frames.length,
      distancePx: element.scrollTop - from,
    };
  });
  await Promise.all([
    measurement,
    (async () => {
      for (let i = 0; i < 24; i++) {
        await page.mouse.wheel(0, 450);
        await page.waitForTimeout(25);
      }
    })(),
  ]);
  return measurement;
}

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
  pdfContent = pdf,
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
          totalFiles: 3,
          totalFolders: 0,
          totalBytes: pdf.byteLength + outlinePdf.byteLength + epub.byteLength,
          trashedFiles: 0,
          activeShares: 0,
          openUploads: 0,
        },
      });
    }
    const content = path.match(/^\/v1\/files\/([^/]+)\/content(?:\/[^/]+)?$/)?.[1];
    if (content === pdfId) {
      if (stats) stats.pdfContentRequests += 1;
      return route.fulfill({ body: pdfContent, contentType: "application/pdf" });
    }
    if (content === outlinePdfId) {
      return route.fulfill({ body: outlinePdf, contentType: "application/pdf" });
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
              kind: state === epubId ? "ebook" : "pdf",
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
  await installPreferencesApi(page);
}

async function openFile(page: Page, name: string) {
  const row = page.getByRole("row", { name: new RegExp(name) });
  await expect(row).toBeVisible();
  await row.focus();
  await page.keyboard.press("Enter");
}

async function openPdfWorkspace(page: Page, name = "reader-sample.pdf", content = pdf) {
  await installViewerApi(page, [], {}, undefined, content);
  await page.goto("/files?view=list");
  await openFile(page, name);
  const dialog = page.getByRole("dialog", { name });
  try {
    await expect(dialog.locator(".pdfViewer .page canvas").first()).toBeVisible({ timeout: 15000 });
  } catch (error) {
    const state = await dialog.locator("[data-pdf-viewer-container]").evaluate((element) => ({
      size: [element.clientWidth, element.clientHeight],
      scroll: [element.scrollWidth, element.scrollHeight],
      pages: [...element.querySelectorAll(".pdfViewer .page")].map((page) => ({
        size: [page.clientWidth, page.clientHeight],
        canvas: page.querySelectorAll("canvas").length,
      })),
    }));
    throw new Error(`PDF canvas did not render: ${JSON.stringify(state)}`, { cause: error });
  }
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

test("PDF text selection does not show the native floating highlight action", async ({ page }) => {
  const dialog = await openPdfWorkspace(page);
  const text = dialog.locator(".textLayer span").filter({ hasText: "TelDrive" }).first();
  await expect(text).toBeVisible();
  const selectionColor = await text.evaluate((element) => {
    const layer = element.closest(".textLayer")!;
    const reference = document.createElement("span");
    reference.style.backgroundColor = "color-mix(in srgb, Highlight 35%, transparent)";
    document.body.append(reference);
    const systemSelection = getComputedStyle(reference).backgroundColor;
    reference.remove();
    return {
      selected: getComputedStyle(element, "::selection").backgroundColor,
      systemSelection,
      nativeSelection: !layer.classList.contains("selectionRendering"),
    };
  });
  expect(selectionColor.nativeSelection).toBe(true);
  expect(selectionColor.selected).toBe(selectionColor.systemSelection);
  await text.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
  });
  const highlight = dialog.locator(".highlightButton");
  await expect(highlight).toHaveCount(0);
  await text.evaluate((element) =>
    element.dispatchEvent(new PointerEvent("pointerup", { bubbles: true })),
  );
  await expect(page.getByRole("button", { name: "PDF selection actions" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Copy", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Find in PDF", exact: true }).click();
  await expect(dialog.getByRole("textbox", { name: "Find in PDF" })).toHaveValue(/TelDrive/);
});

async function slider(page: Page, name: string, value: number) {
  const control = page.getByRole("slider", { name, exact: true });
  await expect(control).toBeEnabled();
  await control.focus();
  await control.press("Home");
  const minimum = Number(await control.getAttribute("min"));
  const step =
    name === "PDF ink stroke width"
      ? 0.5
      : name === "PDF ink opacity" || name === "PDF page brightness"
        ? 5
        : 1;
  for (let current = minimum; current < value; current += step) await control.press("ArrowRight");
  await expect(control).toHaveValue(String(value));
}

test("large PDF sidebar scrolling and outline navigation performance", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium", "Desktop profile");
  await installViewerApi(page, [], {}, undefined, makeLargePdf());
  await page.goto("/files?view=list");
  await openFile(page, "reader-sample.pdf");
  const dialog = page.getByRole("dialog", { name: "reader-sample.pdf" });
  await expect(dialog.locator(".pdfViewer .page canvas").first()).toBeVisible({ timeout: 20_000 });
  const panel = dialog.getByRole("tabpanel", { name: "Pages", exact: true });
  const cpuSession = process.env.TELDRIVE_PDF_CPU_PROFILE
    ? await page.context().newCDPSession(page)
    : null;
  if (cpuSession) {
    await cpuSession.send("Profiler.enable");
    await cpuSession.send("Profiler.start");
  }
  const profile = await panel.evaluate(async (panel) => {
    const element = panel.querySelector("[data-pdf-thumbnails]") || panel;
    // Measure the headless browser's idle refresh cadence before attributing
    // missed frames to PDF rendering or scrolling.
    const idleFrames: number[] = [];
    await new Promise<void>((resolve) => {
      let prior = performance.now();
      const start = prior;
      const tick = (now: number) => {
        idleFrames.push(now - prior);
        prior = now;
        if (now - start < 1000) requestAnimationFrame(tick);
        else resolve();
      };
      requestAnimationFrame(tick);
    });
    idleFrames.sort((a, b) => a - b);
    const frames: number[] = [];
    const longTasks: number[] = [];
    const longFrames: {
      duration: number;
      layoutMs: number;
      scripts: { ms: number; source: string; invoker: string }[];
    }[] = [];
    const frameObserver = new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        const frame = entry as PerformanceEntry & {
          renderStart: number;
          styleAndLayoutStart: number;
          scripts: { duration: number; sourceURL: string; invoker: string }[];
        };
        longFrames.push({
          duration: frame.duration,
          layoutMs: frame.styleAndLayoutStart
            ? frame.startTime + frame.duration - frame.styleAndLayoutStart
            : 0,
          scripts: (frame.scripts || [])
            .map((script) => ({
              ms: script.duration,
              source: script.sourceURL?.split("/").at(-1) || "browser",
              invoker: script.invoker,
            }))
            .sort((a, b) => b.ms - a.ms)
            .slice(0, 4),
        });
      }
    });
    if (PerformanceObserver.supportedEntryTypes.includes("long-animation-frame")) {
      frameObserver.observe({ type: "long-animation-frame", buffered: false });
    }
    const observer = new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) longTasks.push(entry.duration);
    });
    observer.observe({ type: "longtask", buffered: false });
    let previous = performance.now();
    const start = previous;
    await new Promise<void>((resolve) => {
      const tick = (now: number) => {
        frames.push(now - previous);
        previous = now;
        element.scrollTop = ((now - start) / 2000) * 18000;
        if (now - start < 2000) requestAnimationFrame(tick);
        else resolve();
      };
      requestAnimationFrame(tick);
    });
    observer.disconnect();
    frameObserver.disconnect();
    longFrames.sort((a, b) => b.duration - a.duration);
    frames.sort((a, b) => a - b);
    return {
      idleP95FrameMs: idleFrames[Math.floor(idleFrames.length * 0.95)],
      idleFrames: idleFrames.length,
      mountedThumbnails: element.querySelectorAll("canvas").length,
      p95FrameMs: frames[Math.floor(frames.length * 0.95)],
      frames: frames.length,
      longTasks,
      longFrames: longFrames.slice(0, 12),
    };
  });
  if (cpuSession) {
    const { profile: cpuProfile } = await cpuSession.send("Profiler.stop");
    const counts = new Map<number, number>();
    for (const id of cpuProfile.samples || []) counts.set(id, (counts.get(id) || 0) + 1);
    const hot = cpuProfile.nodes
      .map((node) => ({
        name: node.callFrame.functionName,
        source: node.callFrame.url.split("/").at(-1),
        line: node.callFrame.lineNumber + 1,
        samples: counts.get(node.id) || 0,
      }))
      .filter((node) => node.samples)
      .sort((a, b) => b.samples - a.samples)
      .slice(0, 20);
    console.log("PDF CPU profile", JSON.stringify(hot));
    await cpuSession.detach();
  }
  console.log("PDF performance", JSON.stringify(profile));
  expect(profile.mountedThumbnails).toBeLessThan(16);
  await testInfo.attach("pdf-performance", {
    body: JSON.stringify(profile, null, 2),
    contentType: "application/json",
  });
  const thumbnails = dialog.locator("[data-pdf-thumbnails]");
  const wheelThumbnails = await measureWheelScrolling(page, "[data-pdf-thumbnails]");
  console.log("PDF thumbnail wheel scrolling", JSON.stringify(wheelThumbnails));
  await testInfo.attach("pdf-thumbnail-wheel-performance", {
    body: JSON.stringify(wheelThumbnails),
    contentType: "application/json",
  });
  await expect(thumbnails).toHaveAttribute("role", "grid");
  expect(await thumbnails.getByRole("row").count()).toBeLessThan(16);
  await thumbnails.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  expect(await thumbnails.getByRole("row").count()).toBeLessThan(16);
  await expect(dialog.getByRole("button", { name: "Go to page 240", exact: true })).toBeVisible();
  await expect
    .poll(() =>
      dialog
        .getByRole("button", { name: "Go to page 240", exact: true })
        .locator("canvas")
        .evaluate((canvas) => canvas.width),
    )
    .toBeGreaterThan(0);
  // A thumbnail is usable only after the canvas has actually finished rendering.
  await expect(
    dialog.getByRole("button", { name: "Go to page 240", exact: true }).locator("canvas"),
  ).not.toHaveClass(/opacity-0/, { timeout: 15_000 });
  await dialog.getByRole("button", { name: "Go to page 240", exact: true }).click();
  await expect(dialog.getByRole("textbox", { name: "PDF page number", exact: true })).toHaveValue(
    "240",
  );
  // A paused PDF.js render must resume after scrolling stops, not leave a blank page.
  await expect
    .poll(() =>
      dialog
        .locator('.pdfViewer .page[data-page-number="240"] canvas')
        .first()
        .evaluate((canvas) => canvas.width * canvas.height),
    )
    .toBeGreaterThan(0);
  await dialog.getByRole("tab", { name: "Outline", exact: true }).click();
  const width = (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width;
  await dialog.getByRole("button", { name: "Middle chapter", exact: true }).click();
  await expect(dialog.getByRole("textbox", { name: "PDF page number", exact: true })).toHaveValue(
    "121",
  );
  await expect
    .poll(async () => (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width)
    .toBeCloseTo(width, 0);
  const mainCpuSession = process.env.TELDRIVE_PDF_CPU_PROFILE
    ? await page.context().newCDPSession(page)
    : null;
  if (mainCpuSession) {
    await mainCpuSession.send("Profiler.enable");
    await mainCpuSession.send("Profiler.start");
  }
  const mainProfile = await dialog
    .locator("[data-pdf-viewer-container]")
    .evaluate(async (element) => {
      const frames: number[] = [];
      let previous = performance.now();
      const start = previous;
      const top = element.scrollTop;
      await new Promise<void>((resolve) => {
        const tick = (now: number) => {
          frames.push(now - previous);
          previous = now;
          element.scrollTop = top + (now - start) * 2;
          if (now - start < 2000) requestAnimationFrame(tick);
          else resolve();
        };
        requestAnimationFrame(tick);
      });
      frames.sort((a, b) => a - b);
      return { frames: frames.length, p95FrameMs: frames[Math.floor(frames.length * 0.95)] };
    });
  if (mainCpuSession) {
    const { profile: cpuProfile } = await mainCpuSession.send("Profiler.stop");
    const counts = new Map<number, number>();
    for (const id of cpuProfile.samples || []) counts.set(id, (counts.get(id) || 0) + 1);
    console.log(
      "PDF main CPU profile",
      JSON.stringify(
        cpuProfile.nodes
          .map((node) => ({
            name: node.callFrame.functionName,
            source: node.callFrame.url.split("/").at(-1),
            line: node.callFrame.lineNumber + 1,
            samples: counts.get(node.id) || 0,
          }))
          .filter((node) => node.samples)
          .sort((a, b) => b.samples - a.samples)
          .slice(0, 20),
      ),
    );
    await mainCpuSession.detach();
  }
  console.log("PDF main scrolling", JSON.stringify(mainProfile));
  await testInfo.attach("pdf-main-performance", {
    body: JSON.stringify(mainProfile),
    contentType: "application/json",
  });
  const wheelMain = await measureWheelScrolling(page, "[data-pdf-viewer-container]");
  console.log("PDF main wheel scrolling", JSON.stringify(wheelMain));
  await testInfo.attach("pdf-main-wheel-performance", {
    body: JSON.stringify(wheelMain),
    contentType: "application/json",
  });
});

test("long PDF keeps raster and JS heap bounded across distant page jumps", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium", "Chromium memory metrics");
  const dialog = await openPdfWorkspace(page, "reader-sample.pdf", makeLargePdf(240));
  const input = dialog.getByRole("textbox", { name: "PDF page number", exact: true });
  const client = await page.context().newCDPSession(page);
  await client.send("Performance.enable");

  const sample = async (pageNumber: number) => {
    await client.send("HeapProfiler.collectGarbage");
    const [performanceMetrics, rasters] = await Promise.all([
      client.send("Performance.getMetrics"),
      dialog.locator(".pdfViewer").evaluate((viewer) => {
        const canvases = [...viewer.querySelectorAll("canvas")].filter(
          (canvas) => canvas.width > 0 && canvas.height > 0,
        );
        return {
          count: canvases.length,
          bytes: canvases.reduce((sum, canvas) => sum + canvas.width * canvas.height * 4, 0),
        };
      }),
    ]);
    return {
      page: pageNumber,
      ...rasters,
      heapBytes: performanceMetrics.metrics.find((metric) => metric.name === "JSHeapUsedSize")
        ?.value,
    };
  };

  const samples = [await sample(1)];
  for (const destination of [120, 240, 1, 120, 240]) {
    await input.fill(String(destination));
    await input.press("Enter");
    await expect(input).toHaveValue(String(destination));
    await expect(
      dialog.locator(`.pdfViewer .page[data-page-number="${destination}"] canvas`).first(),
    ).toBeVisible({ timeout: 15_000 });
    samples.push(await sample(destination));
  }
  await client.detach();
  console.log("PDF memory", JSON.stringify(samples));
  await testInfo.attach("pdf-memory", {
    body: JSON.stringify(samples, null, 2),
    contentType: "application/json",
  });

  // Canvas resources must not grow with all 240 pages or repeated navigation.
  expect(Math.max(...samples.map((entry) => entry.count))).toBeLessThan(20);
  expect(Math.max(...samples.map((entry) => entry.bytes))).toBeLessThan(256 * 1024 * 1024);
  const firstHeap = samples[0].heapBytes;
  const lastHeap = samples.at(-1)?.heapBytes;
  expect(firstHeap).toBeDefined();
  expect(lastHeap).toBeDefined();
  expect(lastHeap! - firstHeap!).toBeLessThan(64 * 1024 * 1024);
});

test("PDF page fields, fit presets, search closing, and menu shortcuts stay consistent", async ({
  page,
}) => {
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
  await expect
    .poll(() => viewport.evaluate((el) => el.scrollWidth - el.clientWidth))
    .toBeLessThanOrEqual(1);

  if ((page.viewportSize()?.width || 0) >= 1024) {
    const firstPage = dialog.locator(".pdfViewer .page").first();
    const before = (await firstPage.boundingBox())!;
    await dialog.getByRole("button", { name: "Toggle PDF sidebar", exact: true }).click();
    await expect
      .poll(async () => (await firstPage.boundingBox())!.width)
      .toBeCloseTo(before.width, 0);
    await expect.poll(async () => (await firstPage.boundingBox())!.x).toBeCloseTo(before.x, 0);
    await dialog.getByRole("button", { name: "Toggle PDF sidebar", exact: true }).click();
    await expect.poll(async () => (await firstPage.boundingBox())!.x).toBeCloseTo(before.x, 0);
  }
  let manualZoom: number | undefined;
  if ((page.viewportSize()?.width || 0) >= 768) {
    await dialog.getByRole("button", { name: "Zoom in", exact: true }).click();
    manualZoom = (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width;
  }
  await page.setViewportSize({ width: 390, height: 844 });
  if (manualZoom !== undefined) {
    await expect
      .poll(async () => (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width)
      .toBeCloseTo(manualZoom, 0);
    await pdfTool(page, "Fit width");
  }
  await expect
    .poll(() => viewport.evaluate((el) => el.scrollWidth - el.clientWidth))
    .toBeLessThanOrEqual(1);
  const tools = dialog.getByRole("button", { name: "PDF reader tools", exact: true });
  await tools.click();
  await page.getByRole("button", { name: "Fit page", exact: true }).focus();
  const beforeArrow = await input.inputValue();
  await page.keyboard.press("ArrowRight");
  await expect(input).toHaveValue(beforeArrow);
  await page.getByRole("button", { name: "Fit page", exact: true }).click();
  await expect(page.locator('[data-slot="popover-dialog"]')).toHaveCount(0);
  await expect
    .poll(async () => {
      const p = (await dialog.locator(".pdfViewer .page").first().boundingBox())!;
      const c = (await viewport.boundingBox())!;
      return p.height - c.height;
    })
    .toBeLessThanOrEqual(1);

  await dialog.getByRole("button", { name: "Search in PDF", exact: true }).click();
  await dialog.getByRole("textbox", { name: "Find in PDF", exact: true }).fill("TelDrive");
  await expect(dialog.locator(".textLayer .highlight")).toHaveCount(1);
  await dialog.getByRole("button", { name: "Search in PDF", exact: true }).click();
  await expect(dialog.locator("[data-pdf-findbar]")).toHaveCount(0);
  await expect(dialog.locator(".textLayer .highlight")).toHaveCount(0);
});

test("PDF annotation controls synchronize colors, font size, ink settings, undo, and exports", async ({
  page,
}) => {
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
  await expect(
    dialog.getByRole("button", { name: "Use annotation color #000000", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await dialog.getByRole("button", { name: "Use annotation color #60a5fa", exact: true }).click();
  await slider(page, "PDF ink stroke width", 5);
  await slider(page, "PDF ink opacity", 50);
  const inkBox = (await dialog.locator(".pdfViewer .page").first().boundingBox())!;
  const sidebarBox = await dialog.locator("[data-pdf-reader] aside").first().boundingBox();
  // With the non-reflowing sidebar open, draw on the unobstructed page area.
  const startX = Math.max(
    inkBox.x + inkBox.width * 0.2,
    (sidebarBox?.x || 0) + (sidebarBox?.width || 0) + 48,
  );
  const endX = Math.min(inkBox.x + inkBox.width - 48, startX + inkBox.width * 0.3);
  await page.mouse.move(startX, inkBox.y + Math.min(inkBox.height * 0.6, 350));
  await page.mouse.down();
  await page.mouse.move(endX, inkBox.y + Math.min(inkBox.height * 0.65, 380), { steps: 12 });
  await page.mouse.up();
  const blueStrokes = () =>
    dialog
      .locator(".pdfViewer .page svg")
      .evaluateAll(
        (elements) =>
          elements.filter((el) => getComputedStyle(el).stroke === "rgb(96, 165, 250)").length,
      );
  await expect.poll(blueStrokes).toBeGreaterThan(0);
  await pdfTool(page, "Undo PDF edit");
  await expect.poll(blueStrokes).toBe(0);
  await pdfTool(page, "Redo PDF edit");
  await expect.poll(blueStrokes).toBeGreaterThan(0);

  const downloaded = page.waitForEvent("download");
  await pdfTool(
    page,
    (page.viewportSize()?.width || 0) >= 1280 ? "Save edited PDF copy" : "Save copy",
  );
  const download = await downloaded;
  const exported = readFileSync((await download.path())!).toString("latin1");
  expect(exported).toMatch(/\/Subtype\s*\/FreeText/);
  expect(exported).toMatch(/\/Subtype\s*\/Ink/);
  expect(exported).toMatch(/\/CA\s+0\.5/);
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("dialog", { name: "Unsaved PDF changes", exact: true })).toHaveCount(
    0,
  );
});

test("PDF brightness control is display-only and never dirties the document", async ({ page }) => {
  const dialog = await openPdfWorkspace(page);
  const viewer = dialog.locator(".pdfViewer.teldrive-pdf-viewer");
  await expect(viewer).toHaveAttribute("data-pdf-brightness", "100");
  const zoomOptions = dialog.getByRole("button", { name: "PDF zoom options", exact: true });
  if (await zoomOptions.isVisible()) {
    await zoomOptions.click();
  } else {
    await dialog.getByRole("button", { name: "PDF reader tools", exact: true }).click();
  }
  await slider(page, "PDF page brightness", 130);
  await expect(viewer).toHaveAttribute("data-pdf-brightness", "130");
  await expect(viewer).toHaveAttribute("style", /brightness\(1\.3\)/);
  await page.keyboard.press("Escape");
  await expect(page.locator('[data-slot="popover-dialog"]')).toHaveCount(0);
  // Display-only: changing brightness must not trigger the unsaved-changes prompt.
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("dialog", { name: "Unsaved PDF changes", exact: true })).toHaveCount(
    0,
  );
});

test("PDF outline renders a collapsible hierarchy", async ({ page }) => {
  const dialog = await openPdfWorkspace(page, "outline-sample.pdf");
  const sidebar =
    (page.viewportSize()?.width || 0) >= 1024
      ? dialog
      : page.getByRole("dialog", { name: "Document navigation" });
  if ((page.viewportSize()?.width || 0) < 1024) {
    await dialog.getByRole("button", { name: "Open PDF sidebar" }).click();
    await expect(sidebar).toBeVisible();
  }
  await sidebar.getByRole("tab", { name: "Outline" }).click();
  const chapterOne = sidebar.getByRole("button", { name: "Chapter 1", exact: true });
  const sectionOne = sidebar.getByRole("button", { name: "Section 1.1", exact: true });
  const sectionTwo = sidebar.getByRole("button", { name: "Section 1.2", exact: true });
  await expect(chapterOne).toBeVisible();
  await expect(sectionOne).toBeVisible();
  await expect(sectionTwo).toBeVisible();
  await expect(sidebar.getByRole("button", { name: "Chapter 2", exact: true })).toBeVisible();

  const pageInput = dialog.getByRole("textbox", { name: "PDF page number", exact: true });
  // Collapsing a parent hides its children without navigating.
  await sidebar.getByRole("button", { name: "Collapse Chapter 1", exact: true }).click();
  await expect(sectionOne).toBeHidden();
  await expect(sectionTwo).toBeHidden();
  await expect(pageInput).toHaveValue("1");
  await sidebar.getByRole("button", { name: "Expand Chapter 1", exact: true }).click();
  await expect(sectionOne).toBeVisible();
  await expect(sectionTwo).toBeVisible();
  // A bookmark changes the vertical destination without shifting the PDF left.
  const viewport = dialog.locator("[data-pdf-viewer-container]");
  const horizontalBefore = await viewport.evaluate((element) => element.scrollLeft);
  const pageWidthBefore = (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width;
  await sectionTwo.click();
  await expect(pageInput).toHaveValue("2");
  await expect
    .poll(() => viewport.evaluate((element) => element.scrollLeft))
    .toBe(horizontalBefore);
  await expect
    .poll(async () => (await dialog.locator(".pdfViewer .page").first().boundingBox())!.width)
    .toBeCloseTo(pageWidthBefore, 0);
});

test("oversized mobile PDFs render crisp visible detail without oversized canvases", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "mobile", "Requires the high-DPI mobile profile");
  const dialog = await openPdfWorkspace(page, "reader-sample.pdf", makeLargePdf(2, 7000));
  const pageView = dialog.locator(".pdfViewer .page").first();
  await expect
    .poll(() => pageView.locator("canvas").count(), { timeout: 20_000 })
    .toBeGreaterThan(1);
  const metrics = await pageView.locator("canvas").evaluateAll((canvases) =>
    canvases.map((element) => ({
      width: element.width,
      height: element.height,
      scale: element.width / element.getBoundingClientRect().width,
    })),
  );
  expect(metrics.every((canvas) => canvas.width * canvas.height <= 6_291_456 + 1000)).toBe(true);
  expect(metrics.some((canvas) => canvas.scale > 2)).toBe(true);
});

test("mobile PDF renders sharp HiDPI canvases within its pixel budget", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "mobile", "Requires the high-DPI mobile profile");
  const dialog = await openPdfWorkspace(page);
  const canvas = dialog.locator(".pdfViewer .page canvas").first();
  const measure = () =>
    canvas.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return {
        resolution: element.width / rect.width,
        pixels: element.width * element.height,
        deviceRatio: window.devicePixelRatio,
      };
    });
  await expect.poll(async () => (await measure()).resolution).toBeGreaterThan(2);
  const metrics = await measure();
  expect(metrics.resolution).toBeCloseTo(metrics.deviceRatio, 0);
  expect(metrics.pixels).toBeLessThanOrEqual(6_291_456);
  await pdfTool(page, "Zoom in");
  await expect.poll(async () => (await measure()).pixels).toBeGreaterThan(metrics.pixels);
  expect((await measure()).pixels).toBeLessThanOrEqual(6_291_456);
});

test("PDF keeps the visible region HiDPI on display changes and zooming", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium", "Desktop zoom and device-scale transition");
  const dialog = await openPdfWorkspace(page);
  const pageView = dialog.locator(".pdfViewer .page").first();
  const measure = () =>
    pageView.evaluate((element) => {
      const bounds = document.querySelector("[data-pdf-viewer-container]")!.getBoundingClientRect();
      const pageBounds = element.getBoundingClientRect();
      const visible = {
        left: Math.max(bounds.left, pageBounds.left),
        top: Math.max(bounds.top, pageBounds.top),
        right: Math.min(bounds.right, pageBounds.right),
        bottom: Math.min(bounds.bottom, pageBounds.bottom),
      };
      const deviceRatio = window.devicePixelRatio;
      const sharpVisibleRegion = Array.from(element.querySelectorAll("canvas")).some((canvas) => {
        const rect = canvas.getBoundingClientRect();
        return (
          canvas.width > 0 &&
          rect.width > 0 &&
          canvas.width / rect.width >= deviceRatio * 0.94 &&
          rect.left <= visible.left + 2 &&
          rect.top <= visible.top + 2 &&
          rect.right >= visible.right - 2 &&
          rect.bottom >= visible.bottom - 2
        );
      });
      return { deviceRatio, sharpVisibleRegion };
    });
  await expect.poll(async () => (await measure()).sharpVisibleRegion).toBe(true);

  const client = await page.context().newCDPSession(page);
  const viewport = page.viewportSize()!;
  try {
    await client.send("Emulation.setDeviceMetricsOverride", {
      width: viewport.width,
      height: viewport.height,
      deviceScaleFactor: 2,
      mobile: false,
    });
    await expect.poll(async () => (await measure()).deviceRatio).toBe(2);
    // Chromium's CDP emulation does not dispatch real resolution/resize events.
    // Deliver the same resize notification that a display transition produces.
    await page.evaluate(() => window.dispatchEvent(new Event("resize")));
    await expect
      .poll(async () => (await measure()).sharpVisibleRegion, { timeout: 20_000 })
      .toBe(true);

    const zoomOptions = dialog.getByRole("button", { name: "PDF zoom options", exact: true });
    await zoomOptions.click();
    await page.getByRole("button", { name: "200%", exact: true }).click();
    await expect
      .poll(async () => (await measure()).sharpVisibleRegion, { timeout: 20_000 })
      .toBe(true);
  } finally {
    await client.send("Emulation.clearDeviceMetricsOverride");
    await client.detach();
  }
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
  await page.mouse.click(
    pageBox.x + pageBox.width * 0.25,
    pageBox.y + Math.min(pageBox.height * 0.4, 240),
  );
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
      postMessage(
        message: { action?: string; callbackId?: number; sourceName?: string; targetName?: string },
        transfer: Transferable[] = [],
      ) {
        if (message.action === "SaveDocument") {
          queueMicrotask(() =>
            this.dispatchEvent(
              new MessageEvent("message", {
                data: {
                  sourceName: message.targetName,
                  targetName: message.sourceName,
                  callback: 2,
                  callbackId: message.callbackId,
                  reason: {
                    name: "UnknownErrorException",
                    message: "Simulated save failure",
                    details: "Regression test",
                  },
                },
              }),
            ),
          );
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
  await dialog
    .locator(".freeTextEditor [contenteditable=true]")
    .last()
    .fill("Do not lose this edit");
  await dialog.getByRole("button", { name: "Close PDF reader", exact: true }).click();
  const prompt = page.getByRole("dialog", { name: "Unsaved PDF changes", exact: true });
  await prompt.getByRole("button", { name: "Save copy and close", exact: true }).click();
  await expect(prompt.getByRole("alert")).toContainText("Simulated save failure");
  await expect(prompt).toBeVisible();
  await prompt.getByRole("button", { name: "Keep editing", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Simulated save failure");
  expect(errors).toEqual([]);
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
  // The test browser uses a dark color scheme, so Automatic resolves to night.
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

test.beforeEach(async ({page}) => { await page.addInitScript(() => localStorage.setItem("teldrive-language", "en")); });
