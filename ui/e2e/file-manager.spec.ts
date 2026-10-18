import { expect, type Page, test } from "@playwright/test";
import { startDownloadServer } from "./fixtures/download-server";
import {
  installPreferencesApi,
  type PreferencesApi,
  type PreferencesSeed,
} from "./fixtures/preferences";

const now = "2026-07-22T12:00:00Z";
const rootId = "11111111-1111-4111-8111-111111111111";
const alphaId = "22222222-2222-4222-8222-222222222222";
const betaId = "33333333-3333-4333-8333-333333333333";

test("sorting hydrates before the first request and URL overrides saved preferences", async ({
  page,
}) => {
  const preferences = await installFileApi(page, { files: { sort: "size", order: "desc" } });
  const requests: URL[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname === "/api/v1/files") requests.push(url);
  });
  await page.goto("/files");
  await expect.poll(() => requests.length).toBeGreaterThan(0);
  expect(requests[0].searchParams.get("sort")).toBe("size");
  expect(requests[0].searchParams.get("order")).toBe("desc");
  const sortOrder = page.getByRole("button", { name: "Toggle sort order" });
  await expect(sortOrder).toHaveAttribute(
    "aria-label",
    "Toggle sort order: descending — switch to ascending",
  );
  await expect(sortOrder).toHaveText("");
  await expect(sortOrder.locator("svg")).toBeVisible();
  const descendingIcon = await sortOrder.locator("svg").innerHTML();
  await sortOrder.click();
  await expect(sortOrder).toHaveAttribute(
    "aria-label",
    "Toggle sort order: ascending — switch to descending",
  );
  expect(await sortOrder.locator("svg").innerHTML()).not.toBe(descendingIcon);
  await expect.poll(() => requests.at(-1)?.searchParams.get("order")).toBe("asc");
  await expect.poll(() => preferences.current().files.order).toBe("asc");
  requests.length = 0;
  await page.goto("/files?sort=updatedAt&order=asc");
  await expect.poll(() => requests.length).toBeGreaterThan(0);
  expect(requests[0].searchParams.get("sort")).toBe("updatedAt");
  expect(requests[0].searchParams.get("order")).toBe("asc");
});

type FixtureFile = {
  id: string;
  parentId?: string;
  name: string;
  kind: "file" | "folder";
  status: "active" | "trashed";
  generation: number;
  mimeType?: string;
  size?: number;
  modTime: string;
  createdAt: string;
  updatedAt: string;
  encryption: boolean;
};

function file(
  overrides: Partial<FixtureFile> & Pick<FixtureFile, "id" | "name" | "kind">,
): FixtureFile {
  return {
    status: "active",
    generation: 1,
    modTime: now,
    createdAt: now,
    updatedAt: now,
    encryption: false,
    ...overrides,
  };
}

async function installFileApi(page: Page, seed: PreferencesSeed = {}): Promise<PreferencesApi> {
  let uploadSequence = 0;
  const uploadFiles = new Map<string, ReturnType<typeof file>>();
  const files = new Map<string, FixtureFile>([
    [rootId, file({ id: rootId, name: "Destination", kind: "folder" })],
    [
      alphaId,
      file({ id: alphaId, name: "alpha.txt", kind: "file", mimeType: "text/plain", size: 10 }),
    ],
    [
      betaId,
      file({ id: betaId, name: "beta.txt", kind: "file", mimeType: "text/plain", size: 20 }),
    ],
  ]);
  const bookmarks = new Set<string>();
  const findNameConflict = (parentId: string | undefined, name: string, excludeId?: string) =>
    [...files.values()].find(
      (candidate) =>
        candidate.id !== excludeId &&
        candidate.status === "active" &&
        candidate.parentId === parentId &&
        candidate.name.toLowerCase() === name.toLowerCase(),
    );
  const nextAvailableName = (parentId: string | undefined, name: string, excludeId?: string) => {
    const dot = name.lastIndexOf(".");
    const stem = dot > 0 ? name.slice(0, dot) : name;
    const extension = dot > 0 ? name.slice(dot) : "";
    let index = 1;
    let candidate = `${stem} (${index})${extension}`;
    while (findNameConflict(parentId, candidate, excludeId)) {
      index += 1;
      candidate = `${stem} (${index})${extension}`;
    }
    return candidate;
  };

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = request.method();

    if (path === "/v1/me" && method === "GET") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture User",
          username: "fixture",
          premium: true,
          role: "owner",
          capabilities: [
            "files.read",
            "files.write",
            "files.share",
            "system.manageUsers",
            "system.manageJobs",
            "system.manageQueues",
            "system.localImport",
            "system.maintenance",
            "system.owner",
          ],
          createdAt: now,
        },
      });
    }
    if (path === "/v1/files/statistics/drive" && method === "GET") {
      return route.fulfill({
        json: {
          totalFiles: files.size,
          totalFolders: 1,
          totalBytes: 30,
          trashedFiles: 0,
          activeShares: 0,
          openUploads: 0,
        },
      });
    }
    if (path === "/v1/storage/stats" && method === "GET") {
      return route.fulfill({
        json: {
          summary: {
            logicalBytes: 0,
            activeFiles: 0,
            activeFolders: 0,
            trashedFiles: 0,
            trashBytes: 0,
          },
          growth: [],
          categories: [],
          channels: [],
          cleanup: {
            trashBytes: 0,
            staleUploadBytes: 0,
            staleUploads: 0,
            totalReclaimableBytes: 0,
          },
          activity: [],
        },
      });
    }
    if (path === "/v1/jobs" && method === "GET") {
      return route.fulfill({ json: { tasks: [], meta: {} } });
    }
    if (path === "/v1/jobs/statistics" && method === "GET") {
      return route.fulfill({
        json: {
          available: 0,
          cancelled: 0,
          completed: 0,
          discarded: 0,
          pending: 0,
          retryable: 0,
          running: 0,
          scheduled: 0,
        },
      });
    }
    if (path === "/v1/jobs/queues" && method === "GET") {
      return route.fulfill({ json: { queues: [] } });
    }
    if (path === "/v1/jobs/123" && method === "GET") {
      return route.fulfill({
        json: {
          id: "123",
          status: "completed",
          type: "fixture",
          queue: "default",
          attempt: 1,
          maxAttempts: 1,
          priority: 1,
          tags: [],
          args: {},
          errors: [],
          attemptedBy: [],
          createdAt: now,
          scheduledAt: now,
        },
      });
    }
    if (path === "/v1/files" && method === "GET") {
      const parentId = url.searchParams.get("parentId") ?? undefined;
      const search = url.searchParams.get("search");
      const searchType = url.searchParams.get("searchType");
      const bookmarked = url.searchParams.get("bookmarked") === "true";
      const items = [...files.values()].filter(
        (entry) =>
          entry.status === "active" &&
          entry.parentId === parentId &&
          (!bookmarked || bookmarks.has(entry.id)) &&
          (!search ||
            (searchType === "regex"
              ? new RegExp(search, "i").test(entry.name)
              : entry.name.toLowerCase().includes(search.toLowerCase()))),
      );
      return route.fulfill({ json: { items } });
    }
    if (path === "/v1/files/bookmarks" && (method === "POST" || method === "DELETE")) {
      const body = request.postDataJSON() as { fileIds: string[] };
      for (const id of body.fileIds) {
        if (method === "POST") bookmarks.add(id);
        else bookmarks.delete(id);
      }
      return route.fulfill({ status: 204 });
    }
    if (path === `/v1/files/${alphaId}/content/alpha.txt` && method === "GET") {
      return route.fulfill({ contentType: "text/plain", body: "alpha preview" });
    }
    if (path === "/v1/folders" && method === "POST") {
      const body = request.postDataJSON() as { name: string; parentId?: string };
      const existing = [...files.values()].find(
        (entry) =>
          entry.status === "active" &&
          entry.parentId === body.parentId &&
          entry.name.toLowerCase() === body.name.toLowerCase(),
      );
      if (existing) {
        return route.fulfill({
          status: 409,
          json: { error: { code: "name_conflict", message: "Name already exists" } },
        });
      }
      const created = file({
        id: crypto.randomUUID(),
        name: body.name,
        kind: "folder",
        parentId: body.parentId,
      });
      files.set(created.id, created);
      return route.fulfill({ status: 201, json: created });
    }
    if (path === "/v1/uploads" && method === "POST") {
      const body = request.postDataJSON() as {
        name: string;
        parentId?: string;
        size: number;
        encryption: boolean;
      };
      uploadSequence++;
      const id = `66666666-6666-4666-8666-${String(uploadSequence).padStart(12, "0")}`;
      uploadFiles.set(
        id,
        file({
          id: crypto.randomUUID(),
          name: body.name,
          kind: "file",
          parentId: body.parentId,
          size: body.size,
          mimeType: "application/octet-stream",
        }),
      );
      return route.fulfill({
        status: 201,
        json: {
          id,
          userId: 1,
          parentId: body.parentId,
          name: body.name,
          expectedSize: body.size,
          partSize: 512 * 1024 * 1024,
          state: "open",
          encryption: body.encryption,
          conflictPolicy: "rename",
          createdAt: now,
          updatedAt: now,
          expiresAt: "2026-07-23T12:00:00Z",
        },
      });
    }
    const uploadMatch = path.match(
      /^\/v1\/uploads\/([^/]+)(?:\/(parts)(?:\/(\d+))?|\/(complete))?$/,
    );
    if (uploadMatch) {
      const [, uploadId, parts, , complete] = uploadMatch;
      if (method === "GET" && parts) return route.fulfill({ json: { items: [] } });
      if (method === "PUT" && parts) return route.fulfill({ status: 204 });
      if (method === "POST" && complete) {
        const uploaded = uploadFiles.get(uploadId)!;
        files.set(uploaded.id, uploaded);
        return route.fulfill({
          json: uploaded,
        });
      }
      if (method === "DELETE") return route.fulfill({ status: 204 });
    }
    if (path === "/v1/files/bulk/trash" && method === "POST") {
      const body = request.postDataJSON() as { fileIds: string[] };
      for (const id of body.fileIds) {
        const entry = files.get(id);
        if (entry) files.set(id, { ...entry, status: "trashed" });
      }
      return route.fulfill({ json: { items: body.fileIds } });
    }

    const match = path.match(/^\/v1\/files\/([^/]+)(?:\/(copy|move))?$/);
    if (match) {
      const [, id, operation] = match;
      const entry = files.get(id);
      if (!entry)
        return route.fulfill({
          status: 404,
          json: { error: { code: "not_found", message: "Not found" } },
        });
      if (method === "PATCH") {
        const body = request.postDataJSON() as { name: string };
        const renamed = { ...entry, name: body.name, generation: entry.generation + 1 };
        files.set(id, renamed);
        return route.fulfill({ json: renamed });
      }
      if (method === "POST" && operation === "copy") {
        const body = request.postDataJSON() as {
          parentId?: string;
          name?: string;
          conflictPolicy?: "fail" | "rename" | "replace";
        };
        let name = body.name ?? entry.name;
        const conflict = findNameConflict(body.parentId, name);
        if (conflict) {
          if (body.conflictPolicy === "rename") name = nextAvailableName(body.parentId, name);
          else if (body.conflictPolicy === "replace") files.delete(conflict.id);
          else {
            return route.fulfill({
              status: 409,
              json: { error: { code: "name_conflict", message: "Name already exists" } },
            });
          }
        }
        const copied = file({
          ...entry,
          id: crypto.randomUUID(),
          parentId: body.parentId,
          name,
        });
        files.set(copied.id, copied);
        return route.fulfill({ status: 201, json: copied });
      }
      if (method === "POST" && operation === "move") {
        const body = request.postDataJSON() as {
          parentId?: string;
          conflictPolicy?: "fail" | "rename" | "replace";
        };
        let name = entry.name;
        const conflict = findNameConflict(body.parentId, name, id);
        if (conflict) {
          if (body.conflictPolicy === "rename") name = nextAvailableName(body.parentId, name, id);
          else if (body.conflictPolicy === "replace") files.delete(conflict.id);
          else {
            return route.fulfill({
              status: 409,
              json: { error: { code: "name_conflict", message: "Name already exists" } },
            });
          }
        }
        const moved = { ...entry, parentId: body.parentId, name, generation: entry.generation + 1 };
        files.set(id, moved);
        return route.fulfill({ json: moved });
      }
    }

    return route.fulfill({
      status: 404,
      json: { error: { code: "not_found", message: `${method} ${path}` } },
    });
  });
  return installPreferencesApi(page, seed);
}

test.beforeEach(async ({ page }) => installFileApi(page));

test("sidebar shows the build-time version without a health request", async ({
  page,
  isMobile,
}) => {
  let requests = 0;
  await page.route("**/api/health/live", async (route) => {
    requests += 1;
    return route.fulfill({ status: 503, json: {} });
  });
  await page.goto("/files");
  if (isMobile) await page.getByRole("button", { name: "Open navigation" }).click();
  const sidebar = page.locator("aside");
  const versionLabel = process.env.VERSION?.trim() || "dev";
  await expect(sidebar.getByText(versionLabel, { exact: true })).toBeVisible();
  await expect(sidebar.getByRole("button", { name: "Account menu" })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("sidebar-version.png"), scale: "css" });
  if (isMobile) {
    await page
      .getByRole("dialog", { name: "Navigation", exact: true })
      .getByRole("button", { name: "Close navigation" })
      .last()
      .click();
    await page.getByRole("button", { name: "Open navigation" }).click();
    await expect(sidebar.getByText(versionLabel, { exact: true })).toBeVisible();
  }
  expect(requests).toBe(0);
});

for (const theme of ["light", "dark"] as const) {
  test(`mobile navigation has an opaque full-height background in ${theme} mode`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await installPreferencesApi(page, { appearance: { theme }, updatedAt: now });
    await page.goto("/files");
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
    await page.getByRole("button", { name: "Open navigation" }).click();
    const navigation = page.getByRole("dialog", { name: "Navigation", exact: true });
    const sidebar = navigation.locator("aside");
    await expect(sidebar).toBeVisible();
    await expect
      .poll(() =>
        sidebar.evaluate((element) => {
          const reference = document.createElement("div");
          reference.style.backgroundColor = "var(--sidebar)";
          element.append(reference);
          const expected = getComputedStyle(reference).backgroundColor;
          reference.remove();
          return getComputedStyle(element).backgroundColor === expected;
        }),
      )
      .toBe(true);
    await expect
      .poll(() =>
        sidebar.evaluate((element) => {
          const bounds = element.getBoundingClientRect();
          return bounds.top === 0 && bounds.bottom === window.innerHeight;
        }),
      )
      .toBe(true);
    const screenshot = test.info().outputPath(`mobile-sidebar-${theme}.png`);
    await page.screenshot({ path: screenshot, scale: "css" });
    await test.info().attach(`mobile-sidebar-${theme}`, {
      path: screenshot,
      contentType: "image/png",
    });
    await navigation.getByRole("button", { name: "Close navigation" }).last().click();
    await expect(navigation).toBeHidden();
  });
}

test("sidebar active state follows route path and bookmark search state", async ({
  page,
  isMobile,
}) => {
  const cases = [
    ["/files?path=%2F&query=&view=list", "Files"],
    ["/files?bookmarked=true", "Bookmarks"],
    ["/shared?parentId=11111111-1111-4111-8111-111111111111&view=grid", "Shared"],
    ["/shared-with-me?view=grid", "Shared with me"],
    ["/storage", "Storage"],
    ["/tasks", "Tasks"],
    ["/tasks/123", "Tasks"],
    ["/trash", "Trash"],
  ] as const;
  await page.goto("/files");
  const mainNav = page.getByRole("navigation", { name: "Main navigation" });

  for (const [path, activeLabel] of cases) {
    await page.goto(path);
    if (isMobile) await page.getByRole("button", { name: "Open navigation" }).click();
    const activeLinks = mainNav.locator('a[class*="bg-accent/10"]');
    await expect(activeLinks).toHaveCount(1);
    await expect(activeLinks).toHaveText(activeLabel);
  }
});

test("Recent lists files modified within 30 days across the drive", async ({ page }) => {
  const requests: URL[] = [];
  await page.route("**/api/v1/files?*", async (route) => {
    const url = new URL(route.request().url());
    requests.push(url);
    return route.fulfill({
      json: {
        items: [
          file({
            id: crypto.randomUUID(),
            name: "recent.txt",
            kind: "file",
            updatedAt: new Date().toISOString(),
          }),
        ],
      },
    });
  });
  await page.goto("/files?recent=true");
  await expect(
    page
      .getByRole("grid", { name: "Files and folders", exact: true })
      .getByRole("row", { name: /recent.txt/ }),
  ).toBeVisible();
  const query = requests.find((url) => url.searchParams.get("scope") === "drive")!.searchParams;
  expect(query.get("kind")).toBe("file");
  expect(query.get("sort")).toBe("updatedAt");
  expect(query.get("order")).toBe("desc");
  expect(query.get("parentId")).toBeNull();
  const age = Date.now() - new Date(query.get("updatedAfter")!).valueOf();
  expect(age).toBeGreaterThanOrEqual(30 * 86400000);
  expect(age).toBeLessThan(30 * 86400000 + 60000);
});

test("completed upload appears in the current directory without reloading", async ({ page }) => {
  await page.goto("/files?view=list");
  const row = page.getByRole("row", { name: /fresh-upload.txt/ });
  await expect(row).toHaveCount(0);
  await page.getByRole("button", { name: "Upload", exact: true }).click();
  const chooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Upload files", exact: true }).click();
  await (await chooserPromise).setFiles({
    name: "fresh-upload.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("uploaded content"),
  });
  await expect(
    page
      .getByRole("grid", { name: "Files and folders", exact: true })
      .getByRole("row", { name: /fresh-upload.txt/ }),
  ).toBeVisible();
});

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
  await page.getByRole("button", { name: "Upload", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Upload files" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Upload folder" })).toBeVisible();

  const folderChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Upload folder" }).click();
  const folderChooser = await folderChooserPromise;
  await folderChooser.setFiles("e2e/fixtures/Destination");
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("teldrive.uploads.v3")))
    .toBeNull();

  const tree = page.getByRole("treegrid", { name: "Upload queue" });
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
  await expect(page.getByRole("progressbar", { name: "Overall upload progress" })).toHaveAttribute(
    "aria-valuenow",
    "100",
  );
  const destinationBatch = tree.getByRole("row", { name: /Destination/ });
  await tree.getByRole("button", { name: "Collapse Destination" }).click();
  await expect(destinationBatch).toHaveAttribute("aria-expanded", "false");
  await tree.getByRole("button", { name: "Expand Destination" }).click();
  await expect(destinationBatch).toHaveAttribute("aria-expanded", "true");

  const shelf = page.getByTestId("upload-shelf");
  await expect(shelf).toHaveScreenshot("upload-tree-expanded.png");
  await page.getByRole("button", { name: "Collapse uploads" }).click();
  await expect(shelf).toHaveScreenshot("upload-tree-collapsed.png");
});

test("upload queue is ephemeral across a browser reload", async ({ page }) => {
  await page.goto("/files");
  await page.getByRole("button", { name: "Upload", exact: true }).click();
  const fileChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Upload files" }).click();
  const fileChooser = await fileChooserPromise;
  await fileChooser.setFiles("e2e/fixtures/Destination/cover.jpg");

  const shelf = page.getByTestId("upload-shelf");
  await expect(shelf).toBeVisible();
  const queue = shelf.getByRole("treegrid", { name: "Upload queue" });
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
  await page.getByRole("button", { name: "Upload", exact: true }).click();
  const fileChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Upload files" }).click();
  const fileChooser = await fileChooserPromise;
  await fileChooser.setFiles([
    { name: "first.txt", mimeType: "text/plain", buffer: Buffer.from("first") },
    { name: "second.txt", mimeType: "text/plain", buffer: Buffer.from("second") },
  ]);

  const queue = page.getByTestId("upload-shelf").getByRole("treegrid", { name: "Upload queue" });
  await expect(queue.getByRole("row", { name: /first\.txt/ })).toBeVisible();
  await expect(queue.getByRole("row", { name: /second\.txt/ })).toBeVisible();
  await expect(queue.getByText("2 files", { exact: true })).toHaveCount(0);
});

test("upload settings use an encryption switch and normalize chunk size", async ({ page }) => {
  const preferences = await installPreferencesApi(page);
  await page.goto("/settings/uploads");
  const encryption = page.getByRole("switch", { name: "Encrypt uploaded files" });
  await expect(encryption).not.toBeChecked();
  await encryption.press("Space");
  await expect(encryption).toBeChecked();
  await expect.poll(() => preferences.current().uploads.encryption).toBe(true);

  const partSize = page.getByRole("textbox", { name: /Preferred part size in MiB/ });
  await expect(partSize).toHaveValue("512");
  await partSize.fill("521");
  await partSize.blur();
  await expect(partSize).toHaveValue("528");
  await expect
    .poll(() => preferences.patches.at(-1))
    .toEqual({ uploads: { preferredPartSize: 528 * 1024 * 1024 } });

  await partSize.fill("3000");
  await partSize.blur();
  await expect(partSize).toHaveValue("2,048");
  await expect
    .poll(() => preferences.patches.at(-1))
    .toEqual({ uploads: { preferredPartSize: 2048 * 1024 * 1024 } });
});

test("upload settings retain an existing valid chunk choice", async ({ page }) => {
  await installPreferencesApi(page, { uploads: { preferredPartSize: 640 * 1024 * 1024 } });
  await page.goto("/settings/uploads");
  await expect(page.getByRole("textbox", { name: /Preferred part size in MiB/ })).toHaveValue(
    "640",
  );
});

test("account encryption default applies to settings, uploads, and URL imports", async ({
  page,
}) => {
  await installPreferencesApi(page, { uploads: { encryption: true } });
  await page.goto("/settings/uploads");
  await expect(page.getByRole("switch", { name: "Encrypt uploaded files" })).toBeChecked();

  await page.goto("/files");
  await page.getByRole("button", { name: "Upload", exact: true }).click();
  const fileChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("menuitem", { name: "Upload files" }).click();
  const fileChooser = await fileChooserPromise;
  const uploadCreated = page.waitForRequest(
    (request) =>
      request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/uploads",
  );
  await fileChooser.setFiles({
    name: "secret.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("secret"),
  });
  expect((await uploadCreated).postDataJSON()).toMatchObject({ encryption: true });

  await page.getByRole("button", { name: "Upload", exact: true }).click();
  await page.getByRole("menuitem", { name: "Background upload" }).click();
  const dialog = page.getByRole("dialog", { name: "Background upload" });
  await expect(dialog.getByRole("switch", { name: "Encrypt uploaded files" })).toBeChecked();
});

test("legacy browser upload and sort settings import once into an account without preferences", async ({
  page,
}) => {
  const preferences = await installPreferencesApi(page);
  await page.goto("/login");
  await page.evaluate(() => {
    localStorage.setItem(
      "teldrive.upload-settings.v2",
      JSON.stringify({
        encryption: true,
        conflictPolicy: "error",
        concurrency: 5,
        preferredPartSize: 528 * 1024 * 1024,
      }),
    );
    localStorage.setItem("file-sort", JSON.stringify({ sort: "size", order: "desc" }));
  });

  await page.goto("/files");
  await expect
    .poll(() => preferences.patches)
    .toEqual([
      {
        uploads: {
          encryption: true,
          conflictPolicy: "fail",
          concurrency: 5,
          preferredPartSize: 528 * 1024 * 1024,
        },
        files: { sort: "size", order: "desc" },
      },
    ]);
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("teldrive.upload-settings.v2")))
    .toBeNull();
  await expect.poll(() => page.evaluate(() => localStorage.getItem("file-sort"))).toBeNull();
  expect(preferences.current()).toMatchObject({
    uploads: { encryption: true, conflictPolicy: "fail", concurrency: 5 },
    files: { sort: "size", order: "desc" },
  });

  await page.reload();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();
  expect(preferences.patches).toHaveLength(1);
});

test("another account on the same browser does not import the previous account's appearance", async ({
  page,
}) => {
  await installPreferencesApi(page, {
    appearance: { theme: "light", locale: "es" },
    updatedAt: "2026-07-22T12:00:00Z",
  });
  await page.goto("/files");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect.poll(() => page.evaluate(() => localStorage.getItem("theme"))).toBe("light");

  const fresh = await installPreferencesApi(page);
  await page.reload();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();
  // The fresh account keeps its "system" default, which the dark test color scheme resolves.
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  expect(fresh.patches).toEqual([]);
});

test("saved account appearance and file sort override conflicting browser values on load", async ({
  page,
}) => {
  const preferences = await installPreferencesApi(page, {
    appearance: { theme: "light" },
    files: { sort: "size", order: "desc" },
    updatedAt: "2026-07-22T12:00:00Z",
  });
  await page.goto("/login");
  await page.evaluate(() => {
    localStorage.setItem("theme", "dark");
    localStorage.setItem("file-sort", JSON.stringify({ sort: "name", order: "asc" }));
  });

  const listing = page.waitForRequest(
    (request) => new URL(request.url()).pathname === "/api/v1/files",
  );
  await page.goto("/files");
  const url = new URL((await listing).url());
  expect(url.searchParams.get("sort")).toBe("size");
  expect(url.searchParams.get("order")).toBe("desc");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  expect(preferences.patches).toEqual([]);
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
    page.getByRole("button", { name: "Move selected items", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Rename selected item" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Duplicate selected item" })).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Copy selected items", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Bookmark selected items", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Download selected file" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Copy selected file download link" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Cut selected items" })).toBeVisible();
  await beta.click({ modifiers: ["Shift"] });
  await expect(page.getByText("2 selected", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Rename selected item" })).toBeHidden();
  await expect(page.getByRole("button", { name: "Duplicate selected item" })).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Move selected items", exact: true }),
  ).toBeVisible();

  await page.keyboard.press("Control+KeyA");
  await expect(page.getByText("3 selected", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByText(/selected$/)).toBeHidden();
});

test("files and folders can be bookmarked from the shared file manager", async ({ page }) => {
  await page.goto("/files");
  await page.getByRole("row", { name: /alpha\.txt/ }).click();
  await page.getByRole("button", { name: "Bookmark selected items", exact: true }).click();
  await expect(page.getByText("1 item bookmarked", { exact: true })).toBeVisible();

  await page.goto("/files?bookmarked=true");
  await expect(page.getByRole("row", { name: /alpha\.txt/ })).toBeVisible();
  await expect(page.getByRole("row", { name: /beta\.txt/ })).toHaveCount(0);
  await page.getByRole("row", { name: /alpha\.txt/ }).click();
  await page
    .getByRole("button", { name: "Remove bookmarks from selected items", exact: true })
    .click();
  await expect(page.getByText("1 bookmark removed", { exact: true })).toBeVisible();
  await expect(page.getByText("No bookmarks yet", { exact: true })).toBeVisible();
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
  await page.getByRole("button", { name: "Download selected file" }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("alpha.txt");

  await page.getByRole("button", { name: "Copy selected file download link" }).click();
  await expect
    .poll(() => page.evaluate(() => (window as typeof window & { copiedText?: string }).copiedText))
    .toBe(`${new URL(page.url()).origin}/api/v1/files/${alphaId}/content/alpha.txt?download=1`);
});

for (const passwordProtected of [false, true]) {
  for (const folder of [false, true]) {
    test(`public ${folder ? "folder" : "file"} share downloads through the browser (${passwordProtected ? "password" : "no password"})`, async ({
      page,
      baseURL,
    }) => {
      const token = "tds_download-fixture";
      const password = "fixture-password";
      const base = `/api/v1/public/shares/${token}`;
      const filename = "large #1 résumé.bin";
      const entry = file({ id: alphaId, name: filename, kind: "file", size: 6 * 1024 ** 3 });
      const content = folder
        ? `${base}/files/${alphaId}/content/${encodeURIComponent(filename)}?download=1`
        : `${base}/content/${encodeURIComponent(filename)}?download=1`;
      let exchanges = 0;

      // Make whole-file buffering a hard failure rather than merely asserting
      // that a small fixture eventually produces a download.
      await page.addInitScript(() => {
        const blob = Response.prototype.blob;
        Response.prototype.blob = function () {
          if (this.url.includes("/public/shares/")) {
            throw new Error("Public share downloads must not buffer a Blob");
          }
          return blob.call(this);
        };
      });
      await page.route(`**${base}**`, async (route) => {
        const request = route.request();
        const url = new URL(request.url());
        const suppliedPassword = request.headers()["x-share-password"];
        if (url.pathname === `${base}/download-session`) {
          exchanges += 1;
          expect(request.method()).toBe("POST");
          expect(suppliedPassword).toBe(password);
          return route.fulfill({
            status: 204,
            headers: {
              "Set-Cookie": `teldrive_share_download=opaque-fixture; Path=${base}/; HttpOnly; SameSite=Strict; Max-Age=900`,
              "Cache-Control": "no-store",
            },
          });
        }
        if (passwordProtected && suppliedPassword !== password) {
          return route.fulfill({
            status: 401,
            json: { error: { code: "unauthorized", message: "Password required" } },
          });
        }
        if (url.pathname === `${base}/files`) {
          return route.fulfill({ json: { items: [entry] } });
        }
        return route.fulfill({
          json: {
            id: rootId,
            file: folder ? file({ id: rootId, name: "Shared folder", kind: "folder" }) : entry,
            passwordProtected,
            permission: "read",
          },
        });
      });
      if (!baseURL) throw new Error("Missing Playwright base URL");
      const server = await startDownloadServer(baseURL, content);
      try {
        await page.goto(`${server.origin}/share/${token}`);
        if (passwordProtected) {
          await page.locator('input[type="password"]').fill(password);
          await page.getByRole("button", { name: "Open share", exact: true }).click();
        }
        await page.getByRole("row", { name: /large #1/ }).click();
        const pending = page.waitForEvent("download");
        await page.getByRole("button", { name: "Download selected file" }).click();
        const download = await pending;
        expect(await download.failure()).toBeNull();
        expect(download.url()).toBe(`${new URL(page.url()).origin}${content}`);
        expect(download.suggestedFilename()).toBe(filename);
        expect(exchanges).toBe(passwordProtected ? 1 : 0);
        expect(server.requests).toHaveLength(1);
        expect(server.requests[0].headers["x-share-password"]).toBeUndefined();
        if (passwordProtected) {
          expect(server.requests[0].headers.cookie).toContain(
            "teldrive_share_download=opaque-fixture",
          );
        }
        expect(new URL(page.url()).pathname).toBe(`/share/${token}`);
      } finally {
        await server.close();
      }
    });
  }
}

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
  await expect(page.getByText("2 selected", { exact: true })).toBeVisible();

  await page.keyboard.press("KeyD");
  const destination = page.getByRole("row", { name: /Destination/ });
  await expect(destination).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("navigation", { name: "Current folder" })).toContainText(
    "Destination",
  );
});

test("file operation shortcuts are guarded and update visible state", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop keyboard shortcuts");
  await page.goto("/files");
  await page.getByRole("row", { name: /alpha\.txt/ }).click();

  await page.keyboard.press("F2");
  const rename = page.getByRole("dialog", { name: "Rename item" });
  await expect(rename).toBeVisible();
  await rename.getByRole("textbox", { name: "New name" }).fill("renamed.txt");
  await rename.getByRole("textbox", { name: "New name" }).press("Enter");
  await expect(page.getByText("renamed.txt", { exact: true })).toBeVisible();

  await page.getByRole("row", { name: /renamed\.txt/ }).click();
  await page.keyboard.press("Delete");
  await expect(page.getByText("renamed.txt", { exact: true })).toBeHidden();

  await page.keyboard.press("Control+Shift+KeyN");
  const createFolder = page.getByRole("dialog", { name: "Create folder" });
  await expect(createFolder).toBeVisible();
  await expect
    .poll(() => createFolder.evaluate((dialog) => dialog.contains(document.activeElement)))
    .toBe(true);
  await page.keyboard.press("Escape");
  await expect(createFolder).toBeHidden();
  await expect(page.getByRole("textbox", { name: "Search this folder" })).toHaveCount(0);
  await expect(page.getByText(/selected$/)).toBeHidden();
});

test("file shortcuts stay within the browser and do not act inside inputs or dialogs", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop keyboard shortcuts");
  await page.goto("/files");
  const alpha = page.getByRole("row", { name: /alpha\.txt/ });
  await alpha.click();
  const search = page.getByRole("textbox", { name: "Search files" });
  await search.fill("draft");
  await search.press("F2");
  await expect(page.getByRole("dialog", { name: "Rename item" })).toHaveCount(0);
  await search.press("Control+x");
  await search.press("Delete");
  await expect(alpha).toBeVisible();
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  await alpha.focus();
  await alpha.press("F2");
  const dialog = page.getByRole("dialog", { name: "Rename item" });
  const name = dialog.getByRole("textbox", { name: "New name" });
  await name.fill("draft.txt");
  await name.press("Control+a");
  await name.press("Delete");
  await expect(name).toHaveValue("");
  await expect(dialog).toBeVisible();
  await name.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(alpha).toBeVisible();
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  await alpha.focus();
  await alpha.press("Escape");
  await expect(page.getByText("1 selected", { exact: true })).toHaveCount(0);
});

test("selected files keep the destination picker alongside clipboard actions", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop move workflow");
  await page.goto("/files");
  await page.getByRole("row", { name: /alpha\.txt/ }).click();
  await expect(
    page.getByRole("button", { name: "Move selected items", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Cut selected items" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Copy selected items" })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Paste / })).toHaveCount(0);

  await page.getByRole("button", { name: "Move selected items", exact: true }).click();
  const move = page.getByRole("dialog", { name: "Move 1 item" });
  await expect(move).toBeVisible();
  await move.getByRole("row", { name: /Destination/ }).click();
  await move.getByRole("button", { name: "Move here" }).click();
  await expect(move).toBeHidden();
  await expect(page.getByText("alpha.txt", { exact: true })).toBeHidden();

  await page.getByRole("row", { name: /Destination/ }).dblclick();
  await expect(page.getByText("alpha.txt", { exact: true })).toBeVisible();
});

test("split panes cut and copy items directly between folders", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop split view");
  await page.goto("/files");
  await page.getByRole("button", { name: "Open split view" }).click();

  const primary = page.getByTestId("file-pane-primary");
  const secondary = page.getByTestId("file-pane-secondary");
  await secondary.getByRole("row", { name: /Destination/ }).dblclick();
  await expect(secondary.getByRole("navigation", { name: "Current folder" })).toContainText(
    "Destination",
  );

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Cut selected items" }).click();
  await expect(primary.getByRole("button", { name: /^Paste 1 clipboard item$/ })).toHaveCount(0);
  await expect(secondary.getByText("1 cut", { exact: true })).toBeVisible();
  await expect(secondary.getByRole("button", { name: "Cancel cut" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Clear selection" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Move selected items", exact: true })).toHaveCount(
    0,
  );
  await expect(page.getByRole("button", { name: "Move selected items to trash" })).toHaveCount(0);

  await secondary.getByRole("button", { name: "Cancel cut" }).click();
  await expect(page.getByRole("button", { name: /^Paste / })).toHaveCount(0);
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeVisible();

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Cut selected items" }).click();
  await secondary.getByRole("button", { name: /^Paste 1 clipboard item$/ }).click();
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeHidden();
  await expect(secondary.getByText("alpha.txt", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Paste / })).toHaveCount(0);

  await secondary.getByRole("row", { name: /alpha\.txt/ }).click();
  await secondary.getByRole("button", { name: "Copy selected items" }).click();
  await expect(secondary.getByRole("button", { name: /^Paste 1 clipboard item$/ })).toHaveCount(0);
  await expect(primary.getByText("1 copied", { exact: true })).toBeVisible();
  await primary.getByRole("button", { name: /^Paste 1 clipboard item$/ }).click();
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeVisible();
  await expect(secondary.getByText("alpha.txt", { exact: true })).toBeVisible();
  await primary.getByRole("button", { name: "Clear copied items" }).click();
  await expect(page.getByRole("button", { name: /^Paste / })).toHaveCount(0);
});

test("cut paste asks before resolving a name conflict", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop split view");
  await page.goto("/files");
  await page.getByRole("button", { name: "Open split view" }).click();

  const primary = page.getByTestId("file-pane-primary");
  const secondary = page.getByTestId("file-pane-secondary");
  await secondary.getByRole("row", { name: /Destination/ }).dblclick();

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Copy selected items" }).click();
  await secondary.getByRole("button", { name: /^Paste 1 clipboard item$/ }).click();
  await expect(secondary.getByText("alpha.txt", { exact: true })).toBeVisible();
  // The success toast overlaps the action bar and pauses while the pointer is over it.
  await page.getByRole("button", { name: "Close toast", exact: true }).click();
  await secondary.getByRole("button", { name: "Clear copied items" }).click();

  await primary.getByRole("row", { name: /alpha\.txt/ }).click();
  await primary.getByRole("button", { name: "Cut selected items" }).click();
  await secondary.getByRole("button", { name: /^Paste 1 clipboard item$/ }).click();

  const conflict = page.getByRole("dialog", { name: "Item already exists" });
  await expect(conflict).toBeVisible();
  await expect(conflict.getByRole("button", { name: "Replace" })).toBeVisible();
  await expect(conflict.getByRole("button", { name: "Keep both" })).toBeVisible();
  await expect(primary.getByText("alpha.txt", { exact: true })).toBeVisible();

  await conflict.getByRole("button", { name: "Keep both" }).click();
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

  await page.getByRole("button", { name: "Open split view" }).click();
  const primary = page.getByTestId("file-pane-primary");
  const secondary = page.getByTestId("file-pane-secondary");
  await expect(primary).toBeVisible();
  await expect(secondary).toBeVisible();
  await expect(secondary.getByRole("button", { name: "Close split view" })).toBeVisible();

  await expect(primary.getByRole("textbox", { name: "Search this folder" })).toHaveCount(0);
  await expect(secondary.getByRole("textbox", { name: "Search this folder" })).toHaveCount(0);
  await expect(secondary.getByRole("button", { name: "Back" })).toBeVisible();
  await expect(secondary.getByRole("button", { name: "Up one folder" })).toBeDisabled();

  const primaryFolder = primary.getByRole("navigation", { name: "Current folder" });
  const secondaryFolder = secondary.getByRole("navigation", { name: "Current folder" });
  await secondary.getByRole("row", { name: /Destination/ }).dblclick();
  await expect(secondaryFolder).toContainText("Destination");
  await expect(primaryFolder).not.toContainText("Destination");
  await expect(secondary.getByRole("button", { name: "Up one folder" })).toBeEnabled();

  await page.goBack();
  await expect(secondaryFolder).not.toContainText("Destination");
  await expect(primaryFolder).not.toContainText("Destination");
  await expect(page.getByRole("button", { name: "Close split view" })).toBeVisible();

  await page.getByRole("button", { name: "Close split view" }).click();
  await expect(page.getByTestId("file-pane-secondary")).toHaveCount(0);
});

test("selection toolbar stays within narrow viewports and its actions remain reachable", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 740 });
  await page.goto("/files");
  await page
    .getByRole("row", { name: /alpha\.txt/ })
    .locator('[data-slot="checkbox-control"]')
    .click();
  const toolbar = page.getByRole("group", { name: "Selected file actions" });
  await expect(toolbar).toBeVisible();
  await expect(
    page.getByTestId("file-browser-toolbar").getByRole("group", {
      name: "Selected file actions",
    }),
  ).toHaveCount(0);

  for (const width of [320, 390, 412, 1280]) {
    await page.setViewportSize({ width, height: 740 });
    await expect
      .poll(() =>
        toolbar.evaluate((element) => {
          const bounds = element.getBoundingClientRect();
          return bounds.left >= 0 && bounds.right <= window.innerWidth;
        }),
      )
      .toBe(true);
    for (const button of await toolbar.getByRole("button").all()) {
      await expect(button).toHaveCSS("flex-shrink", "0");
      expect((await button.boundingBox())!.width).toBeGreaterThanOrEqual(28);
    }
    const rows = await toolbar.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const buttons = [...element.querySelectorAll("button")];
      if (
        buttons.some((button) => {
          const rect = button.getBoundingClientRect();
          return (
            rect.left < bounds.left ||
            rect.right > bounds.right ||
            rect.top < bounds.top ||
            rect.bottom > bounds.bottom
          );
        })
      )
        return 0;
      return new Set(buttons.map((button) => Math.round(button.getBoundingClientRect().top))).size;
    });
    if (width === 1280) expect(rows).toBe(1);
    else expect(rows).toBeGreaterThan(1);
    await expect
      .poll(() =>
        page.getByTestId("file-browser-toolbar").evaluate((element) => {
          const header = element.getBoundingClientRect();
          return [...element.querySelectorAll("button")].every((button) => {
            const bounds = button.getBoundingClientRect();
            return (
              bounds.width === 0 || (bounds.left >= header.left && bounds.right <= header.right)
            );
          });
        }),
      )
      .toBe(true);
  }

  await page.setViewportSize({ width: 320, height: 740 });
  await expect(toolbar).toHaveCSS("flex-wrap", "wrap");
  await expect
    .poll(() => toolbar.evaluate((element) => element.scrollWidth - element.clientWidth))
    .toBeLessThanOrEqual(1);
  await toolbar.getByRole("button", { name: "Clear selection" }).click();
  await expect(toolbar).toBeHidden();
  await expect(
    page.getByTestId("file-browser-toolbar").getByRole("button", {
      name: "Sort files",
    }),
  ).toBeVisible();
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
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Move selected items", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Clear selection" }).tap();
  await page.getByRole("row", { name: /Destination/ }).tap();
  await expect(page.getByRole("navigation", { name: "Current folder" })).toContainText(
    "Destination",
  );
  await expect(page.getByRole("button", { name: "Up one folder" })).toBeEnabled();
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth))
    .toBeLessThanOrEqual(1);
});
