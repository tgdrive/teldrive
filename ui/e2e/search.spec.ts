import { expect, type Page, test } from "@playwright/test";

const resultFile = {
  id: "11111111-1111-4111-8111-111111111111",
  name: "alpha.txt",
  kind: "file",
  status: "active",
  generation: 3,
  mimeType: "text/plain",
  size: 12,
  parentId: "22222222-2222-4222-8222-222222222222",
  parentPath: "/Documents",
  modTime: "2026-07-22T12:00:00Z",
  createdAt: "2026-07-22T12:00:00Z",
  updatedAt: "2026-07-22T12:00:00Z",
  encryption: false,
};

async function installResults(page: Page) {
  const state = { items: [resultFile], requests: [] as URL[] };
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read", "files.write", "files.share"],
          premium: false,
          createdAt: resultFile.createdAt,
        },
      });
    if (url.pathname === "/api/v1/files") {
      state.requests.push(url);
      return route.fulfill({
        json: { items: url.searchParams.get("kind") === "folder" ? [] : state.items },
      });
    }
    if (url.pathname.includes("/content/"))
      return route.fulfill({ contentType: "text/plain", body: "Preview from search" });
    return route.fulfill({ status: 404, json: {} });
  });
  return state;
}

test("header search submits and the routed query fetches drive results", async ({ page }) => {
  const requests: URL[] = [];
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          username: "search",
          premium: false,
          role: "owner",
          capabilities: ["files.read", "files.write"],
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/files") {
      requests.push(url);
      return route.fulfill({
        json: {
          items: [
            {
              id: "11111111-1111-4111-8111-111111111111",
              name: "alpha.txt",
              kind: "file",
              status: "active",
              generation: 1,
              mimeType: "text/plain",
              size: 12,
              modTime: "2026-07-22T12:00:00Z",
              createdAt: "2026-07-22T12:00:00Z",
              updatedAt: "2026-07-22T12:00:00Z",
              encryption: false,
            },
          ],
        },
      });
    }
    return route.fulfill({ status: 404, json: {} });
  });

  await page.goto("/files");
  const search = page.getByRole("textbox", { name: "Search files" });
  await search.fill("alpha");
  await search.press("Enter");
  await expect(page).toHaveURL(/\/search\?q=alpha/);
  await expect(page.getByText("alpha.txt", { exact: true })).toBeVisible();
  expect(requests.at(-1)?.searchParams.get("search")).toBe("alpha");
  expect(requests.at(-1)?.searchParams.get("scope")).toBe("drive");

  await page.keyboard.press("Control+k");
  await expect(search).toBeFocused();
});

test("header submission carries the active folder context without sending a path parameter", async ({
  page,
}) => {
  let searchRequest: URL | undefined;
  const folderId = "33333333-3333-4333-8333-333333333333";
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files") {
      if (url.searchParams.has("search")) searchRequest = url;
      return route.fulfill({ json: { items: [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto(`/files?path=%2FProjects%2FDesign&parentId=${folderId}`);
  await page.getByRole("textbox", { name: "Search files" }).fill("brief");
  await page.getByRole("textbox", { name: "Search files" }).press("Enter");
  await expect(page).toHaveURL(/folderPath=%2FProjects%2FDesign/);
  await expect(page).toHaveURL(new RegExp(`parentId=${folderId}`));
  await expect.poll(() => searchRequest?.searchParams.get("search")).toBe("brief");
  expect(searchRequest?.searchParams.has("parentId")).toBe(false);
  expect(searchRequest?.searchParams.has("path")).toBe(false);
});

test("search filters-only navigation serializes scope without leaking path", async ({ page }) => {
  let request: URL | undefined;
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files") {
      request = url;
      return route.fulfill({ json: { items: [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto(
    "/search?scope=recursive&parentId=11111111-1111-4111-8111-111111111111&kind=file",
  );
  await expect(page.getByText("0 loaded")).toBeVisible();
  expect(request?.searchParams.get("scope")).toBe("recursive");
  expect(request?.searchParams.get("parentId")).toBe("11111111-1111-4111-8111-111111111111");
  expect(request?.searchParams.has("path")).toBe(false);
});

test("search filters apply as a real form and keep canonical dates in URL and API", async ({
  page,
}) => {
  const requests: URL[] = [];
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files") {
      requests.push(url);
      return route.fulfill({ json: { items: [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/search?q=report");
  await page.getByRole("button", { name: "Filters" }).click();
  await page.getByRole("button", { name: /Type/ }).click();
  await page.getByRole("option", { name: "Files", exact: true }).click();
  await page.getByLabel("Modified after").fill("2026-05-10");
  await page.getByRole("button", { name: "Apply filters" }).click();
  await expect(page).toHaveURL(/kind=file/);
  await expect
    .poll(() => requests.at(-1)?.searchParams.get("updatedAfter"))
    .toBe("2026-05-10T00:00:00.000Z");
  await page.reload();
  await expect(page.getByRole("button", { name: /Files/ })).toBeVisible();
});

test("recursive filter can choose a folder and sends only its id", async ({ page }) => {
  let request: URL | undefined;
  const folderId = "22222222-2222-4222-8222-222222222222";
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files") {
      if (url.searchParams.get("kind") === "folder")
        return route.fulfill({
          json: {
            items: [{ id: folderId, name: "Projects", kind: "folder", status: "active" }],
          },
        });
      request = url;
      return route.fulfill({ json: { items: [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/search?q=notes");
  await page.getByRole("button", { name: "Filters" }).click();
  await page.getByRole("button", { name: /Search in/ }).click();
  await expect(page.getByRole("option", { name: "Folder and subfolders" })).toBeEnabled();
  await page.getByRole("option", { name: "Folder and subfolders" }).click();
  await page.getByRole("row", { name: "Projects" }).click();
  await page.getByRole("button", { name: "Use this folder" }).click();
  await page.getByRole("button", { name: "Apply filters" }).click();
  await expect(page).toHaveURL(/scope=recursive/);
  await expect.poll(() => request?.searchParams.get("parentId")).toBe(folderId);
  expect(request?.searchParams.has("path")).toBe(false);
});

test("a recursive URL without its folder is recoverable and never requests or spins", async ({
  page,
}) => {
  let listCalls = 0;
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files") {
      listCalls++;
      return route.fulfill({ json: { items: [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/search?q=report&scope=recursive");
  await expect(page.getByText("Choose a folder to search recursively")).toBeVisible();
  await expect(page.getByRole("button", { name: "Choose folder" })).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveCount(0);
  expect(listCalls).toBe(0);
});

test("live search debounces requests and browser history restores the input", async ({ page }) => {
  const searches: string[] = [];
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files") {
      if (url.searchParams.has("search")) searches.push(url.searchParams.get("search")!);
      return route.fulfill({ json: { items: [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/files");
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1_000));
  const input = page.getByRole("textbox", { name: "Search files" });
  await input.fill("first");
  await input.press("Enter");
  await expect(page).toHaveURL(/q=first/);
  await input.fill("second");
  await expect(page).toHaveURL(/q=first/);
  await page.clock.runFor(299);
  await expect(page).toHaveURL(/q=first/);
  await page.clock.runFor(1);
  await expect(page).toHaveURL(/q=second/);
  await page.goBack();
  await expect(page).toHaveURL(/\/files/);
  await page.goForward();
  await expect(page).toHaveURL(/q=second/);
  await expect(input).toHaveValue("second");
  await input.fill("pending edit");
  await page.goBack();
  await page.clock.runFor(301);
  await expect(page).toHaveURL(/\/files/);
  expect(searches).toContain("second");
});

test("mobile search results expose a working root location button", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const fileId = "11111111-1111-4111-8111-111111111111";
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files" && route.request().method() === "GET")
      return route.fulfill({
        json: {
          items: [
            {
              id: fileId,
              name: "root.txt",
              kind: "file",
              status: "active",
              parentPath: "/",
              generation: 1,
              mimeType: "text/plain",
              size: 1,
              modTime: "2026-07-22T12:00:00Z",
              createdAt: "2026-07-22T12:00:00Z",
              updatedAt: "2026-07-22T12:00:00Z",
              encryption: false,
            },
          ],
        },
      });
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/search?q=root");
  const location = page.getByRole("button", { name: "Open containing folder /" });
  await expect(location).toBeVisible();
  await location.click();
  await expect(page).toHaveURL(/\/files\?path=%2F/);
});

test("search selection exposes bounded actions and renames the selected result", async ({
  page,
}) => {
  const fileId = "11111111-1111-4111-8111-111111111111";
  let renamed: { id: string; body: unknown } | undefined;
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname === "/api/v1/me")
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Search User",
          role: "owner",
          capabilities: ["files.read", "files.write"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    if (url.pathname === "/api/v1/files" && request.method() === "GET")
      return route.fulfill({
        json: {
          items: [
            {
              id: fileId,
              name: "alpha.txt",
              kind: "file",
              status: "active",
              generation: 3,
              mimeType: "text/plain",
              size: 12,
              modTime: "2026-07-22T12:00:00Z",
              createdAt: "2026-07-22T12:00:00Z",
              updatedAt: "2026-07-22T12:00:00Z",
              encryption: false,
            },
          ],
        },
      });
    if (url.pathname === `/api/v1/files/${fileId}` && request.method() === "PATCH") {
      renamed = { id: fileId, body: request.postDataJSON() };
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/search?q=alpha");
  const row = page.getByRole("row", { name: /alpha\.txt/ });
  await row.click();
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Cut selected items" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Duplicate selected item" })).toHaveCount(0);
  await page.getByRole("button", { name: "Rename selected item" }).click();
  await page.getByRole("textbox", { name: "New name" }).fill("renamed.txt");
  await page.getByRole("button", { name: "Rename", exact: true }).click();
  await expect.poll(() => renamed?.id).toBe(fileId);
  expect(renamed?.body).toEqual({ name: "renamed.txt" });
});

test("failed copies retain retry context and pending confirmation cannot submit twice", async ({
  page,
}) => {
  const state = await installResults(page);
  let attempts = 0;
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/files/*/copy", async (route) => {
    attempts += 1;
    expect(route.request().postDataJSON()).toEqual({ conflictPolicy: "rename" });
    if (attempts === 1)
      return route.fulfill({
        status: 409,
        json: { error: { code: "conflict", message: "Copy failed" } },
      });
    await pending;
    return route.fulfill({ json: resultFile });
  });
  await page.goto("/search?q=alpha");
  await page
    .getByRole("row", { name: /alpha\.txt/ })
    .locator('[data-slot="checkbox-control"]')
    .click();
  await page.getByRole("button", { name: "Copy selected items", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Copy 1 item" });
  const confirm = dialog.getByRole("button", { name: "Copy here" });
  await confirm.click();
  await expect(dialog.getByRole("alert")).toContainText("Items could not be copied");
  await expect(dialog).toBeVisible();
  await expect(page.getByText("1 selected", { exact: true })).toHaveCount(1);
  await confirm.click();
  await expect(confirm).toBeDisabled();
  await dialog.press("Escape");
  await expect(dialog).toBeVisible();
  await confirm.press("Enter");
  expect(attempts).toBe(2);
  const searchRequests = () =>
    state.requests.filter((url) => url.searchParams.get("scope") === "drive").length;
  const before = searchRequests();
  release();
  await expect(dialog).toHaveCount(0);
  await expect.poll(searchRequests).toBeGreaterThan(before);
  await expect(page.getByText("1 selected", { exact: true })).toHaveCount(0);
});

test("pagination preserves filters and selection; changing criteria starts a fresh page", async ({
  page,
}) => {
  await installResults(page);
  const requests: URL[] = [];
  const warnings: string[] = [];
  page.on("console", (message) => {
    if (/controlled to uncontrolled|uncontrolled to controlled/.test(message.text()))
      warnings.push(message.text());
  });
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/files?**", async (route) => {
    const url = new URL(route.request().url());
    requests.push(url);
    if (url.searchParams.has("cursor")) {
      await pending;
      return route.fulfill({
        json: {
          items: [
            { ...resultFile, id: "33333333-3333-4333-8333-333333333333", name: "alpha-second.txt" },
          ],
        },
      });
    }
    return route.fulfill({
      json: {
        items: [resultFile],
        nextCursor: url.searchParams.has("kind") ? undefined : "next-page",
      },
    });
  });
  const params = new URLSearchParams({
    q: "alpha",
    category: JSON.stringify(["document", "image"]),
    updatedAfter: "2026-05-10",
    sort: "size",
    order: "desc",
  });
  await page.goto(`/search?${params}`);
  await expect(page.getByRole("row", { name: /alpha\.txt/ })).toBeVisible();
  await expect.poll(() => requests.some((url) => url.searchParams.has("cursor"))).toBe(true);
  await page
    .getByRole("row", { name: /alpha\.txt/ })
    .locator('[data-slot="checkbox-control"]')
    .click();
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  const continuation = requests.find((url) => url.searchParams.has("cursor"))!;
  await page.getByRole("grid", { name: "Files and folders" }).focus();
  await page.keyboard.press("Control+a");
  expect(continuation.searchParams.get("search")).toBe("alpha");
  expect(continuation.searchParams.getAll("category")).toEqual(["document", "image"]);
  expect(continuation.searchParams.get("updatedAfter")).toBe("2026-05-10T00:00:00.000Z");
  expect(continuation.searchParams.get("sort")).toBe("size");
  expect(continuation.searchParams.get("order")).toBe("desc");
  release();
  await expect(page.getByRole("row", { name: /alpha-second\.txt/ })).toBeVisible();
  await expect(page.getByRole("row", { name: /alpha-second\.txt/ })).toHaveAttribute(
    "aria-selected",
    "false",
  );
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Grid view" }).click();
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Filters", exact: true }).click();
  await page.getByRole("button", { name: /Type/ }).click();
  await page.getByRole("option", { name: "Files", exact: true }).click();
  await page.getByRole("button", { name: "Apply filters" }).click();
  await expect.poll(() => requests.at(-1)?.searchParams.get("kind")).toBe("file");
  expect(requests.at(-1)?.searchParams.has("cursor")).toBe(false);
  await expect(page.getByRole("row", { name: /alpha-second\.txt/ })).toHaveCount(0);
  expect(warnings).toEqual([]);
});

test("search previews preserve the route and nested location links open the containing folder", async ({
  page,
}) => {
  await installResults(page);
  await page.goto("/search?q=alpha");
  const row = page.getByRole("row", { name: /alpha\.txt/ });
  await row.focus();
  await page.keyboard.press("Enter");
  const preview = page.getByRole("dialog", { name: "alpha.txt" });
  await expect(preview).toBeVisible();
  await expect(page).toHaveURL(/\/search\?q=alpha/);
  await preview.getByRole("button", { name: "Close viewer" }).click();
  await page
    .getByRole("button", { name: "Open containing folder /Documents" })
    .filter({ visible: true })
    .click();
  await expect(page).toHaveURL(/\/files\?path=%2FDocuments/);
  await expect(page).toHaveURL(new RegExp(`parentId=${resultFile.parentId}`));
});

test("applying unrelated filters preserves precise dates and invalid saved ranges are recoverable", async ({
  page,
}) => {
  const state = await installResults(page);
  await page.goto("/search?q=alpha&updatedAfter=2026-05-10T12%3A34%3A56Z");
  await page.getByRole("button", { name: "Filters", exact: true }).click();
  await page.getByRole("button", { name: /Type/ }).click();
  await page.getByRole("option", { name: "Files", exact: true }).click();
  await page.getByRole("button", { name: "Apply filters" }).click();
  await expect.poll(() => state.requests.at(-1)?.searchParams.get("kind")).toBe("file");
  expect(state.requests.at(-1)?.searchParams.get("updatedAfter")).toBe("2026-05-10T12:34:56.000Z");

  state.requests.length = 0;
  await page.goto("/search?q=alpha&updatedAfter=2026-05-10&updatedBefore=2026-05-10");
  await expect(page.getByText("Check the modified-date range", { exact: true })).toBeVisible();
  expect(state.requests).toHaveLength(0);
  await page.getByRole("button", { name: "Edit filters" }).click();
  await page.getByLabel("Modified before").fill("2026-05-12");
  await page.getByRole("button", { name: "Apply filters" }).click();
  await expect
    .poll(() => state.requests.at(-1)?.searchParams.get("updatedBefore"))
    .toBe("2026-05-12T00:00:00.000Z");
});

test("search error and empty states never claim that a folder is empty", async ({ page }) => {
  await installResults(page);
  let fail = true;
  await page.route("**/api/v1/files?**", (route) =>
    fail
      ? route.fulfill({
          status: 422,
          json: { error: { code: "invalid_request", message: "Search unavailable" } },
        })
      : route.fulfill({ json: { items: [] } }),
  );
  await page.goto("/search?q=missing");
  await expect(page.getByRole("alert")).toContainText("Search could not be completed");
  await expect(page.getByText("This folder is empty", { exact: true })).toHaveCount(0);
  fail = false;
  await page.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(page.getByText("No matching files", { exact: true })).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
});

test("search waits for IME composition to finish before changing the URL", async ({ page }) => {
  await installResults(page);
  await page.goto("/search?q=alpha");
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1_000));
  const input = page.getByRole("textbox", { name: "Search files" });
  await input.evaluate((element) =>
    element.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true })),
  );
  await input.fill("報告");
  await input.press("Enter");
  await page.clock.runFor(500);
  await expect(page).toHaveURL(/q=alpha/);
  await input.evaluate((element) =>
    element.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "報告" })),
  );
  await page.clock.runFor(300);
  await expect.poll(() => new URL(page.url()).searchParams.get("q")).toBe("報告");
});
