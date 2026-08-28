import { expect, type Page, test } from "@playwright/test";

const now = "2026-07-22T12:00:00Z";
const rootId = "11111111-1111-4111-8111-111111111111";
const alphaId = "22222222-2222-4222-8222-222222222222";
const betaId = "33333333-3333-4333-8333-333333333333";

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

async function installFileApi(page: Page) {
  let uploadSequence = 0;
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
    if (path === "/v1/files" && method === "GET") {
      const parentId = url.searchParams.get("parentId") ?? undefined;
      const search = url.searchParams.get("search");
      const searchType = url.searchParams.get("searchType");
      const items = [...files.values()].filter(
        (entry) =>
          entry.status === "active" &&
          entry.parentId === parentId &&
          (!search ||
            (searchType === "regex"
              ? new RegExp(search, "i").test(entry.name)
              : entry.name.toLowerCase().includes(search.toLowerCase()))),
      );
      return route.fulfill({ json: { items } });
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
      return route.fulfill({
        status: 201,
        json: {
          id: `66666666-6666-4666-8666-${String(uploadSequence).padStart(12, "0")}`,
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
        return route.fulfill({
          json: file({
            id: crypto.randomUUID(),
            name: uploadId,
            kind: "file",
            size: 1,
            mimeType: "application/octet-stream",
          }),
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
        const body = request.postDataJSON() as { parentId?: string; name?: string };
        const copied = file({
          ...entry,
          id: crypto.randomUUID(),
          parentId: body.parentId,
          name: body.name ?? entry.name,
        });
        files.set(copied.id, copied);
        return route.fulfill({ status: 201, json: copied });
      }
      if (method === "POST" && operation === "move") {
        const body = request.postDataJSON() as { parentId?: string };
        const moved = { ...entry, parentId: body.parentId, generation: entry.generation + 1 };
        files.set(id, moved);
        return route.fulfill({ json: moved });
      }
    }

    return route.fulfill({
      status: 404,
      json: { error: { code: "not_found", message: `${method} ${path}` } },
    });
  });
}

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
  await page.goto("/settings/uploads");
  const encryption = page.getByRole("switch", { name: "Encrypt uploaded files" });
  await expect(encryption).not.toBeChecked();
  await encryption.press("Space");
  await expect(encryption).toBeChecked();

  const partSize = page.getByRole("textbox", { name: /Preferred part size in MiB/ });
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
  await expect(partSize).toHaveValue("2,048");
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
  await expect(page.getByRole("textbox", { name: /Preferred part size in MiB/ })).toHaveValue(
    "640",
  );
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
  await expect(page.getByRole("button", { name: "Duplicate selected item" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Download selected file" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Copy selected file download link" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Cut selected items" })).toHaveCount(0);
  await beta.click({ modifiers: ["Shift"] });
  await expect(page.getByText("2 selected", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Rename selected item" })).toBeHidden();
  await expect(page.getByRole("button", { name: "Duplicate selected item" })).toBeHidden();
  await expect(
    page.getByRole("button", { name: "Move selected items", exact: true }),
  ).toBeVisible();

  await page.keyboard.press("Control+KeyA");
  await expect(page.getByText("3 selected", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByText(/selected$/)).toBeHidden();
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
  await page.keyboard.press("Escape");
  await expect(createFolder).toBeHidden();
  await page.keyboard.press("Control+KeyF");
  const search = page.getByRole("textbox", { name: "Search this folder" });
  await expect(search).toBeFocused();
  await search.fill("alpha");
  await page.keyboard.press("Control+KeyA");
  await expect(search).toHaveValue("alpha");
  await expect(page.getByText(/selected$/)).toBeHidden();
});

test("selected files move through the destination picker without clipboard state", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "desktop move workflow");
  await page.goto("/files");
  await page.getByRole("row", { name: /alpha\.txt/ }).click();
  await expect(
    page.getByRole("button", { name: "Move selected items", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Cut selected items" })).toHaveCount(0);
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

  const primaryFolder = primary.getByRole("navigation", { name: "Current folder" });
  const secondaryFolder = secondary.getByRole("navigation", { name: "Current folder" });
  await secondary.getByRole("row", { name: /Destination/ }).dblclick();
  await expect(secondaryFolder).toContainText("Destination");
  await expect(primaryFolder).not.toContainText("Destination");

  await page.goBack();
  await expect(secondaryFolder).not.toContainText("Destination");
  await expect(primaryFolder).not.toContainText("Destination");
  await expect(page.getByRole("button", { name: "Close split view" })).toBeVisible();

  await page.getByRole("button", { name: "Close split view" }).click();
  await expect(page.getByTestId("file-pane-secondary")).toHaveCount(0);
});

test("touch opens items and exposes an explicit multi-selection control", async ({
  page,
  isMobile,
}) => {
  test.skip(!isMobile, "touch-only file interaction");
  await page.goto("/files");
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
});
