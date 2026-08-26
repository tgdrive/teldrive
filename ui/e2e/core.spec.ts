import { expect, type Page, test } from "@playwright/test";

const now = "2026-07-22T12:00:00Z";
const folderId = "11111111-1111-4111-8111-111111111111";
const fileId = "22222222-2222-4222-8222-222222222222";
const trashedId = "33333333-3333-4333-8333-333333333333";

const activeFiles = [
  {
    id: folderId,
    name: "Documents",
    kind: "folder",
    encryption: true,
    status: "active",
    modTime: now,
    generation: 1,
    createdAt: now,
    updatedAt: now,
  },
  {
    id: fileId,
    name: "fixture.txt",
    kind: "file",
    mimeType: "text/plain",
    size: 128,
    encryption: true,
    status: "active",
    modTime: now,
    generation: 1,
    createdAt: now,
    updatedAt: now,
  },
];

async function installApi(page: Page) {
  let uploaded = false;
  let jobState = "running";
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
          totalFiles: 1,
          totalFolders: 1,
          totalBytes: 128,
          trashedFiles: 1,
          activeShares: 0,
          openUploads: 0,
        },
      });
    }
    if (path === "/v1/files" && method === "GET") {
      const status = url.searchParams.get("status");
      if (status === "trashed") {
        return route.fulfill({
          json: {
            items: [
              {
                id: trashedId,
                name: "deleted.txt",
                kind: "file",
                mimeType: "text/plain",
                size: 32,
                encryption: true,
                status: "trashed",
                modTime: now,
                generation: 1,
                createdAt: now,
                updatedAt: now,
              },
            ],
          },
        });
      }
      const search = url.searchParams.get("search")?.toLowerCase();
      if (search === "load-more") {
        const cursor = url.searchParams.get("cursor");
        if (cursor === "page-2") {
          return route.fulfill({
            json: {
              items: [
                {
                  ...activeFiles[1],
                  id: "99999999-9999-4999-8999-999999999999",
                  name: "loaded-second-page.txt",
                },
              ],
            },
          });
        }
        return route.fulfill({
          json: {
            items: Array.from({ length: 100 }, (_, index) => ({
              ...activeFiles[1],
              id: `88888888-8888-4888-8${String(index).padStart(3, "0")}-888888888888`,
              name: `load-more-${String(index + 1).padStart(3, "0")}.txt`,
            })),
            nextCursor: "page-2",
          },
        });
      }
      const items = search
        ? activeFiles.filter((file) => file.name.toLowerCase().includes(search))
        : activeFiles;
      const kind = url.searchParams.get("kind");
      return route.fulfill({
        json: { items: kind === "folder" ? items.filter((file) => file.kind === "folder") : items },
      });
    }
    if (path === `/v1/files/${fileId}/content/readme.txt` && method === "GET") {
      return route.fulfill({ contentType: "text/plain", body: "fixture preview" });
    }
    if (path === "/v1/storage/stats" && method === "GET") {
      return route.fulfill({
        json: {
          summary: {
            logicalBytes: 2684354560,
            activeFiles: 184291,
            activeFolders: 1284,
            trashedFiles: 327,
            trashBytes: 927712935,
          },
          growth: Array.from({ length: 30 }, (_, index) => ({
            day: `2026-07-${String(index + 1).padStart(2, "0")}`,
            addedBytes: 10485760 + index * 524288,
            logicalBytes: 2147483648 + index * 18874368,
          })),
          categories: [
            { category: "video", totalFiles: 92000, totalSize: 1900000000 },
            { category: "archive", totalFiles: 18000, totalSize: 380000000 },
            { category: "image", totalFiles: 52000, totalSize: 190000000 },
            { category: "document", totalFiles: 17000, totalSize: 110000000 },
            { category: "audio", totalFiles: 5000, totalSize: 80000000 },
            { category: "other", totalFiles: 291, totalSize: 24354560 },
          ],
          channels: [
            {
              channelId: 1001,
              name: "Main Storage",
              selected: true,
              health: "healthy",
              lastCheckedAt: now,
              partCount: 104292,
              storedBytes: 1524713390,
            },
            {
              channelId: 1002,
              name: "Archive Channel",
              selected: false,
              health: "healthy",
              lastCheckedAt: now,
              partCount: 48219,
              storedBytes: 765041049,
            },
            {
              channelId: 1003,
              name: "Overflow Channel",
              selected: false,
              health: "unknown",
              partCount: 11804,
              storedBytes: 126164665,
            },
          ],
          cleanup: {
            trashBytes: 927712935,
            staleUploadBytes: 105226699,
            staleUploads: 3,
            totalReclaimableBytes: 1032939634,
          },
          activity: [
            {
              id: 4,
              type: "file.purged",
              resourceType: "file",
              label: "archive-2024.tar.zst",
              occurredAt: now,
            },
            {
              id: 3,
              type: "file.restored",
              resourceType: "file",
              label: "report-final.pdf",
              occurredAt: now,
            },
            {
              id: 2,
              type: "channel.created",
              resourceType: "channel",
              label: "Overflow Channel",
              occurredAt: now,
            },
            {
              id: 1,
              type: "upload.completed",
              resourceType: "upload",
              label: "nature-documentary.mkv",
              occurredAt: now,
            },
          ],
        },
      });
    }
    const job = {
      id: "42",
      status: jobState,
      type: "teldrive_upload_cleanup",
      queue: "maintenance",
      description: "Clean stale uploads",
      message: jobState === "running" ? "Scanning stale uploads" : "Task cancelled",
      progress: jobState === "running" ? 35 : 35,
      attempt: 1,
      maxAttempts: 10,
      priority: 2,
      tags: ["teldrive", "maintenance"],
      args: { batchSize: 100 },
      metadata: { source: "fixture", path: "/uploads" },
      errors: [],
      attemptedBy: ["fixture-worker"],
      createdAt: now,
      scheduledAt: now,
      startedAt: now,
      completedAt: jobState === "cancelled" ? now : undefined,
    };
    if (path === "/v1/jobs/statistics" && method === "GET") {
      return route.fulfill({
        json: {
          available: 0,
          cancelled: jobState === "cancelled" ? 1 : 0,
          completed: 0,
          discarded: 0,
          pending: 0,
          retryable: 0,
          running: jobState === "running" ? 1 : 0,
          scheduled: 0,
        },
      });
    }
    if (path === "/v1/jobs/queues" && method === "GET") {
      return route.fulfill({
        json: {
          queues: [
            {
              name: "maintenance",
              paused: false,
              available: 0,
              running: 1,
              retryable: 0,
              scheduled: 0,
            },
          ],
        },
      });
    }
    if (path === "/v1/jobs" && method === "GET") {
      return route.fulfill({ json: { tasks: [job], meta: {} } });
    }
    if (path === "/v1/jobs" && method === "POST") {
      return route.fulfill({ status: 201, json: job });
    }
    if (path === "/v1/jobs/42" && method === "GET") {
      return route.fulfill({ json: job });
    }
    if (path === "/v1/jobs/42/cancel" && method === "POST") {
      jobState = "cancelled";
      return route.fulfill({ json: { ...job, status: jobState, completedAt: now } });
    }
    if (path === "/v1/jobs/42/retry" && method === "POST") {
      jobState = "available";
      return route.fulfill({ json: { ...job, status: jobState } });
    }
    if (path === "/v1/periodic-jobs" && method === "GET") {
      return route.fulfill({
        json: {
          jobs: [
            {
              id: "teldrive-upload-cleanup",
              kind: "teldrive_upload_cleanup",
              args: { batchSize: 100 },
              queue: "maintenance",
              priority: 2,
              maxAttempts: 10,
              tags: ["teldrive", "cleanup", "uploads"],
              cronExpression: "*/5 * * * *",
              cronTimezone: "UTC",
              nextRunAt: now,
              paused: false,
              createdAt: now,
              updatedAt: now,
            },
            {
              id: "custom-periodic-job",
              kind: "teldrive_upload_cleanup",
              args: { batchSize: 25 },
              queue: "maintenance",
              priority: 2,
              maxAttempts: 10,
              tags: [],
              cronExpression: "0 * * * *",
              cronTimezone: "UTC",
              nextRunAt: now,
              paused: false,
              createdAt: now,
              updatedAt: now,
            },
          ],
        },
      });
    }
    if (path === "/v1/periodic-jobs/catalog" && method === "GET") {
      return route.fulfill({
        json: {
          templates: [
            {
              kind: "teldrive_upload_cleanup",
              label: "Upload cleanup",
              description: "Clean stale multipart uploads.",
              defaultId: "teldrive-upload-cleanup",
              defaultArgs: { batchSize: 100 },
              defaultQueue: "maintenance",
              recommendedCron: "*/5 * * * *",
            },
          ],
        },
      });
    }
    if (path === "/v1/folders" && method === "POST") {
      const body = request.postDataJSON() as { name: string };
      return route.fulfill({
        status: 201,
        json: { ...activeFiles[0], id: "44444444-4444-4444-8444-444444444444", name: body.name },
      });
    }
    if (path === `/v1/files/${fileId}/shares` && method === "POST") {
      return route.fulfill({
        status: 201,
        json: {
          id: "55555555-5555-4555-8555-555555555555",
          fileId,
          token: "fixture-token",
          publicUrl: "https://files.example.test/v1/public/shares/fixture-token",
          passwordProtected: false,
          createdAt: now,
        },
      });
    }
    if (path === "/v1/uploads" && method === "POST") {
      return route.fulfill({
        status: 201,
        json: {
          id: "66666666-6666-4666-8666-666666666666",
          name: "dropped.txt",
          expectedSize: 12,
          mimeType: "text/plain",
          modTime: now,
          encryption: false,
          conflictPolicy: "rename",
          partSize: 16,
          state: "open",
          expiresAt: now,
          createdAt: now,
        },
      });
    }
    if (path === "/v1/uploads/66666666-6666-4666-8666-666666666666/parts" && method === "GET") {
      return route.fulfill({ json: { items: [] } });
    }
    if (path === "/v1/uploads/66666666-6666-4666-8666-666666666666/parts/1" && method === "PUT") {
      return route.fulfill({
        status: 201,
        json: {
          uploadId: "66666666-6666-4666-8666-666666666666",
          partNo: 1,
          state: "stored",
          plainSize: 12,
          createdAt: now,
          updatedAt: now,
        },
      });
    }
    if (path === "/v1/uploads/66666666-6666-4666-8666-666666666666/complete" && method === "POST") {
      uploaded = true;
      return route.fulfill({
        status: 201,
        json: {
          ...activeFiles[1],
          id: "77777777-7777-4777-8777-777777777777",
          name: "dropped.txt",
          size: 12,
        },
      });
    }
    if (path === "/v1/uploads" && method === "GET") {
      return route.fulfill({
        json: {
          items: uploaded
            ? [
                {
                  id: "66666666-6666-4666-8666-666666666666",
                  name: "dropped.txt",
                  expectedSize: 12,
                  modTime: now,
                  encryption: false,
                  conflictPolicy: "rename",
                  partSize: 16,
                  state: "completed",
                  expiresAt: now,
                  createdAt: now,
                },
              ]
            : [],
        },
      });
    }
    if (
      ["/v1/channels", "/v1/bots", "/v1/sessions", "/v1/api-keys"].includes(path) &&
      method === "GET"
    ) {
      return route.fulfill({ json: { items: [] } });
    }
    if (
      path.startsWith("/v1/files/") ||
      path === "/v1/folders" ||
      path === "/v1/files/bulk/trash"
    ) {
      return route.fulfill({
        status: method === "DELETE" ? 204 : 200,
        json: method === "DELETE" ? undefined : activeFiles[1],
      });
    }
    return route.fulfill({
      status: 404,
      json: { error: { code: "not_found", message: `Unhandled fixture ${method} ${path}` } },
    });
  });
}

function collectRuntimeErrors(page: Page) {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(`pageerror: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(`console: ${message.text()}`);
  });
  return errors;
}

test.beforeEach(async ({ page }) => installApi(page));

test("files renders, selects, previews, and exposes actions", async ({ page, isMobile }) => {
  test.skip(
    isMobile,
    "desktop virtualized interaction is covered separately from mobile navigation",
  );
  const errors = collectRuntimeErrors(page);
  await page.goto("/files");
  await expect(page.getByRole("heading", { name: "Files" })).toBeVisible();
  await expect(page.getByText("fixture.txt", { exact: true })).toBeVisible();
  await page.getByText("fixture.txt", { exact: true }).click();
  await expect(page.getByRole("heading", { name: "fixture.txt" })).toBeVisible();
  await expect(page.getByText("fixture preview")).toBeVisible();
  expect(errors).toEqual([]);
});

test("files loads additional cursor pages while virtualized", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop covers the scroll-driven virtualizer sentinel");
  await page.goto("/files?path=%2F&query=load-more&view=list");
  const list = page.getByRole("grid", { name: "Files and folders" });
  await expect(list).toBeVisible();
  await expect(page.getByText("load-more-001.txt", { exact: true })).toBeVisible();
  await list.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
  });
  await expect(page.getByText("101 items", { exact: true })).toBeVisible();
  expect(await list.getByRole("row").count()).toBeLessThan(100);
});
test("tasks uses RiverPro job data", async ({ page }) => {
  await page.goto("/tasks");
  await expect(page.getByRole("heading", { name: "Tasks" })).toBeVisible();
  await expect(page.getByRole("link", { name: "teldrive_upload_cleanup #42" })).toBeVisible();
  await expect(page.getByText("maintenance queue", { exact: true })).toBeVisible();
});

test("storage dashboard shows usage, distribution, cleanup, and activity", async ({ page }) => {
  await page.goto("/storage");
  await expect(page.getByRole("heading", { name: "Storage", exact: true })).toBeVisible();
  await expect(page.getByText("Storage growth", { exact: true })).toBeVisible();
  await expect(page.getByText("Storage composition", { exact: true })).toBeVisible();
  await expect(page.getByText("Telegram channel distribution", { exact: true })).toBeVisible();
  await expect(page.getByText("Cleanup opportunities", { exact: true })).toBeVisible();
  await expect(page.getByText("Recent storage activity", { exact: true })).toBeVisible();
  await expect(page.getByText("Data health", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Upload health", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Largest items", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Maintenance jobs", { exact: true })).toHaveCount(0);
});

test("settings exposes all Teldrive areas", async ({ page, isMobile }) => {
  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Account" })).toBeVisible();
  if (isMobile) await page.getByRole("button", { name: "Open settings navigation" }).click();
  const navigation = page.locator('nav[aria-label="Settings navigation"]:visible');
  await expect(navigation).toBeVisible();
  for (const label of [
    "Overview",
    "Channels",
    "Bots",
    "Sessions",
    "API keys",
    "Uploads",
    "Periodic Jobs",
    "Appearance",
  ]) {
    await expect(navigation.getByRole("link", { name: label, exact: true }).first()).toBeVisible();
  }
});

test("task detail is the exact detail surface", async ({ page }) => {
  await page.goto("/tasks/42");
  await expect(page.getByRole("heading", { name: "Clean stale uploads" })).toBeVisible();
  await expect(page.getByText("Arguments", { exact: true })).toBeVisible();
  await expect(page.getByText("Metadata", { exact: true })).toBeVisible();
  await expect(page.getByText("Attempts", { exact: true })).toBeVisible();
});

test("periodic jobs appears in settings with the editor", async ({ page }) => {
  await page.goto("/settings/periodic-jobs");
  await expect(page.locator("h1").filter({ hasText: "Periodic Jobs" })).toBeVisible();
  await page.getByRole("button", { name: "Add periodic job" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
});

test("periodic job controls are icon buttons with row-scoped pending state", async ({ page }) => {
  let releasePause!: () => void;
  const pauseResponse = new Promise<void>((resolve) => {
    releasePause = resolve;
  });
  await page.route("**/api/v1/periodic-jobs/teldrive-upload-cleanup/pause", async (route) => {
    await pauseResponse;
    await route.fulfill({
      json: {
        id: "teldrive-upload-cleanup",
        kind: "teldrive_upload_cleanup",
        args: { batchSize: 100 },
        queue: "maintenance",
        priority: 2,
        maxAttempts: 10,
        tags: ["teldrive", "cleanup", "uploads"],
        cronExpression: "*/5 * * * *",
        cronTimezone: "UTC",
        nextRunAt: new Date().toISOString(),
        paused: true,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
    });
  });

  await page.goto("/settings/periodic-jobs");
  const firstPause = page.getByRole("button", { name: "Pause teldrive-upload-cleanup" });
  const secondPause = page.getByRole("button", { name: "Pause custom-periodic-job" });
  await expect(page.getByRole("button", { name: "Edit teldrive-upload-cleanup" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Delete teldrive-upload-cleanup" })).toBeVisible();

  await firstPause.click();
  await expect(firstPause).toBeDisabled();
  await expect(secondPause).toBeEnabled();
  releasePause();
  await expect(page.getByRole("button", { name: "Resume teldrive-upload-cleanup" })).toBeVisible();
});

test("task launcher queues a Teldrive River job", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop task launcher interaction");
  await page.goto("/tasks");
  await page.getByRole("button", { name: "New task" }).click();
  await expect(page.getByRole("heading", { name: "New task" })).toBeVisible();
  const queued = page.waitForRequest(
    (request) => request.method() === "POST" && request.url().endsWith("/api/v1/jobs"),
  );
  await page.getByRole("button", { name: "Queue clean stale uploads" }).click();
  await queued;
  await expect(page.getByRole("heading", { name: "New task" })).toBeHidden();
});

test("trash exposes restore and permanent deletion", async ({ page }) => {
  await page.goto("/trash");
  await expect(page.getByRole("heading", { name: "Trash" })).toBeVisible();
  await expect(page.getByText("deleted.txt", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Restore" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Delete forever" })).toBeVisible();
});

test("command palette searches Teldrive files", async ({ page }) => {
  await page.goto("/files");
  await page.keyboard.press("Control+KeyK");
  const dialog = page.getByRole("dialog", { name: /search/i });
  await expect(dialog).toBeVisible();
  const input = dialog.getByRole("textbox");
  await input.fill("fixture");
  await expect(dialog.getByText("fixture.txt", { exact: true })).toBeVisible();
});

test("mobile navigation opens and reaches Tasks", async ({ page, isMobile }) => {
  test.skip(!isMobile, "mobile-only navigation");
  await page.goto("/files");
  await page.getByRole("button", { name: "Open navigation" }).click();
  const dialog = page.getByRole("dialog", { name: "Navigation" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("link", { name: "Tasks", exact: true }).click();
  await expect(page).toHaveURL(/\/tasks$/);
});

test("auth guard redirects before protected UI renders", async ({ page }) => {
  await page.route("**/api/v1/me", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 350));
    await route.fulfill({
      status: 401,
      json: { error: { code: "unauthorized", message: "Authentication required" } },
    });
  });

  const navigation = page.goto("/");
  await page.waitForTimeout(100);
  await expect(page.getByText("Cloud drive", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Files" })).toHaveCount(0);
  await navigation;
  await expect(page).toHaveURL(/\/login\?redirect=/);
  await expect(page.getByRole("heading", { name: "Sign in with Telegram" })).toBeVisible();
});

test("Telegram login surface renders independently", async ({ page }) => {
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Sign in with Telegram" })).toBeVisible();
  await expect(page.getByText("Phone", { exact: true })).toBeVisible();
  await expect(page.getByText("QR code", { exact: true })).toBeVisible();
});

test("React Aria drop zone and file trigger complete an upload", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop upload interaction");
  await page.goto("/files");
  const dropZone = page.getByTestId("file-drop-zone");
  await expect(dropZone).toHaveAttribute("data-rac", "");

  const completed = page.waitForRequest((request) =>
    request.url().endsWith("/api/v1/uploads/66666666-6666-4666-8666-666666666666/complete"),
  );
  await page
    .locator('input[type="file"]')
    .first()
    .setInputFiles({
      name: "dropped.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("hello world!"),
    });
  await completed;
  await expect(page.getByText("dropped.txt", { exact: true })).toBeVisible();
  await expect(page.getByText("completed", { exact: true })).toBeVisible();
});

test("files create-folder dialog uses  design controls", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop file toolbar");
  await page.goto("/files");
  await page.getByRole("button", { name: "New folder" }).click();
  const dialog = page.getByRole("dialog", { name: "Create folder" });
  await dialog.getByRole("textbox", { name: "Folder name" }).fill("Projects");
  await dialog.getByRole("button", { name: "Create folder" }).click();
  await expect(dialog).toBeHidden();
});

test("upload settings use HeroUI controls and persist choices", async ({ page }) => {
  await page.goto("/settings/uploads");
  const encryption = page.getByRole("switch", { name: "Encrypt uploaded files" });
  await expect(encryption).not.toBeChecked();
  await encryption.press("Space");
  await page.getByRole("button", { name: /rename new file/i }).click();
  await page.getByRole("option", { name: "Replace existing", exact: true }).click();
  await expect(encryption).toBeChecked();
  await expect(page.getByRole("button", { name: /replace existing/i })).toBeVisible();
});

test("running RiverPro job can be cancelled", async ({ page }) => {
  await page.goto("/tasks");
  const cancelled = page.waitForRequest(
    (request) => request.method() === "POST" && request.url().endsWith("/api/v1/jobs/42/cancel"),
  );
  await page.getByRole("button", { name: /^Cancel teldrive_upload_cleanup #42$/ }).click();
  await cancelled;
  await expect(page.getByRole("link", { name: "teldrive_upload_cleanup #42" })).toBeHidden();
});

test("all active routes render without runtime errors", async ({ page }) => {
  const errors = collectRuntimeErrors(page);
  for (const route of [
    "/files",
    "/storage",
    "/tasks",
    "/trash",
    "/settings",
    "/settings/channels",
    "/settings/bots",
    "/settings/sessions",
    "/settings/api-keys",
    "/settings/uploads",
    "/settings/periodic-jobs",
    "/tasks/42",
    "/settings/appearance",
  ]) {
    await page.goto(route);
    await expect(page.locator("main").first()).toBeVisible();
  }
  expect(errors).toEqual([]);
});

test("visual regression for every active surface", async ({ page }, testInfo) => {
  const surfaces = [
    ["files", "/files"],
    ["storage", "/storage"],
    ["tasks", "/tasks"],
    ["trash", "/trash"],
    ["settings-overview", "/settings"],
    ["settings-channels", "/settings/channels"],
    ["settings-bots", "/settings/bots"],
    ["settings-sessions", "/settings/sessions"],
    ["settings-api-keys", "/settings/api-keys"],
    ["settings-uploads", "/settings/uploads"],
    ["settings-periodic-jobs", "/settings/periodic-jobs"],
    ["settings-appearance", "/settings/appearance"],
    ["task-detail", "/tasks/42"],
    ["login", "/login"],
  ] as const;

  for (const [name, route] of surfaces) {
    await page.goto(route);
    await page.evaluate(() => document.fonts.ready);
    await expect(page).toHaveScreenshot(`${testInfo.project.name}-${name}.png`, {
      fullPage: true,
      caret: "hide",
    });
  }
});
