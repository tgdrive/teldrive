// Regression coverage retained from tgdrive/teldrive main (67bf180).
import { expect, type Page, test } from "@playwright/test";
import { installPreferencesApi } from "./fixtures/preferences";

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
  await installPreferencesApi(page);
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

test("upload settings use HeroUI controls and save choices to the account", async ({ page }) => {
  const preferences = await installPreferencesApi(page);
  await page.goto("/settings/uploads");
  const encryption = page.getByRole("switch", { name: "Encrypt uploaded files" });
  await expect(encryption).not.toBeChecked();
  await encryption.press("Space");
  await page.getByRole("button", { name: /rename the new file/i }).click();
  await page.getByRole("option", { name: "Replace existing", exact: true }).click();
  await expect(encryption).toBeChecked();
  await expect(page.getByRole("button", { name: /replace existing/i })).toBeVisible();
  await expect
    .poll(() => preferences.current().uploads)
    .toMatchObject({ encryption: true, conflictPolicy: "replace" });
  expect(preferences.patches).toContainEqual({ uploads: { encryption: true } });
  expect(preferences.patches).toContainEqual({ uploads: { conflictPolicy: "replace" } });
});

test.beforeEach(async ({page}) => { await page.addInitScript(() => localStorage.setItem("teldrive-language", "en")); });
