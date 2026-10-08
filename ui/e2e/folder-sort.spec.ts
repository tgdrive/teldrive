import { expect, type Page, test } from "@playwright/test";

const photosId = "11111111-1111-4111-8111-111111111111";

type Entry = {
  id: string;
  parentId?: string;
  name: string;
  kind: "file" | "folder";
  size?: number;
  updatedAt: string;
};

const entries: Entry[] = [
  { id: photosId, name: "Photos", kind: "folder", updatedAt: "2026-07-01T00:00:00Z" },
  {
    id: "22222222-2222-4222-8222-222222222222",
    name: "alpha.txt",
    kind: "file",
    size: 30,
    updatedAt: "2026-07-02T00:00:00Z",
  },
  {
    id: "33333333-3333-4333-8333-333333333333",
    name: "beta.txt",
    kind: "file",
    size: 10,
    updatedAt: "2026-07-20T00:00:00Z",
  },
  {
    id: "44444444-4444-4444-8444-444444444444",
    name: "gamma.txt",
    kind: "file",
    size: 20,
    updatedAt: "2026-07-10T00:00:00Z",
  },
  {
    id: "55555555-5555-4555-8555-555555555555",
    parentId: photosId,
    name: "old.jpg",
    kind: "file",
    size: 5,
    updatedAt: "2026-06-01T00:00:00Z",
  },
  {
    id: "66666666-6666-4666-8666-666666666666",
    parentId: photosId,
    name: "new.jpg",
    kind: "file",
    size: 7,
    updatedAt: "2026-07-21T00:00:00Z",
  },
];

// A server that lists a folder in the order it is asked for, and remembers what it was asked.
async function installFileApi(page: Page) {
  const asked: { parentId: string | null; sort: string | null; order: string | null }[] = [];
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^\/api/, "");
    if (path === "/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture User",
          username: "fixture",
          premium: true,
          role: "owner",
          capabilities: ["files.read", "files.write"],
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (path === "/v1/files" && route.request().method() === "GET") {
      const parentId = url.searchParams.get("parentId");
      const folderPath = url.searchParams.get("path");
      const parent = parentId ?? (folderPath === "/Photos" ? photosId : undefined);
      const sort = url.searchParams.get("sort");
      const order = url.searchParams.get("order");
      asked.push({ parentId, sort, order });
      const key = (entry: Entry) =>
        sort === "size" ? (entry.size ?? 0) : sort === "updatedAt" ? entry.updatedAt : entry.name;
      const items = entries
        .filter((entry) => entry.parentId === parent)
        .sort((a, b) => {
          const left = key(a);
          const right = key(b);
          const compared = left < right ? -1 : left > right ? 1 : 0;
          return order === "desc" ? -compared : compared;
        })
        .map((entry) => ({
          status: "active",
          generation: 1,
          modTime: entry.updatedAt,
          createdAt: entry.updatedAt,
          encryption: false,
          ...entry,
        }));
      return route.fulfill({ json: { items } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  return asked;
}

function rowNames(page: Page, pane = "file-pane-primary") {
  return page
    .getByTestId(pane)
    .getByRole("row")
    .evaluateAll((rows) =>
      rows.map((row) => row.textContent?.match(/[\w-]+\.(?:txt|jpg)|Photos/)?.[0] ?? ""),
    );
}

async function chooseSort(page: Page, label: string, pane = "file-pane-primary") {
  await page.getByTestId(pane).getByRole("button", { name: "Sort" }).click();
  await page.getByRole("menuitem", { name: label }).click();
}

test("a folder is listed by name until another order is chosen, and the address keeps the choice", async ({
  page,
}) => {
  const asked = await installFileApi(page);
  await page.goto("/files");
  await expect.poll(() => rowNames(page)).toEqual(["Photos", "alpha.txt", "beta.txt", "gamma.txt"]);
  expect(asked.at(-1)).toMatchObject({ sort: "name", order: "asc" });
  // The defaults are not written into the address.
  expect(new URL(page.url()).searchParams.has("sort")).toBe(false);

  await chooseSort(page, "Size");
  await expect.poll(() => rowNames(page)).toEqual(["Photos", "beta.txt", "gamma.txt", "alpha.txt"]);
  expect(asked.at(-1)).toMatchObject({ sort: "size", order: "asc" });
  expect(new URL(page.url()).searchParams.get("sort")).toBe("size");

  await chooseSort(page, "Descending");
  await expect.poll(() => rowNames(page)).toEqual(["alpha.txt", "gamma.txt", "beta.txt", "Photos"]);
  expect(asked.at(-1)).toMatchObject({ sort: "size", order: "desc" });

  await chooseSort(page, "Modified date");
  await expect.poll(() => rowNames(page)).toEqual(["beta.txt", "gamma.txt", "alpha.txt", "Photos"]);
  expect(asked.at(-1)).toMatchObject({ sort: "updatedAt", order: "desc" });

  // A reload, or a bookmark, lists it the same way.
  await page.reload();
  await expect.poll(() => rowNames(page)).toEqual(["beta.txt", "gamma.txt", "alpha.txt", "Photos"]);
  const address = new URL(page.url()).searchParams;
  expect([address.get("sort"), address.get("order")]).toEqual(["updatedAt", "desc"]);
});

test("the order is kept on entering a folder, and choosing one adds no step to go back through", async ({
  page,
}) => {
  const asked = await installFileApi(page);
  await page.goto("/files");
  await expect.poll(() => rowNames(page)).toContain("Photos");
  await chooseSort(page, "Modified date");
  await chooseSort(page, "Descending");
  await expect.poll(() => rowNames(page)).toEqual(["beta.txt", "gamma.txt", "alpha.txt", "Photos"]);

  await page.getByRole("row", { name: /Photos/ }).dblclick();
  await expect.poll(() => rowNames(page)).toEqual(["new.jpg", "old.jpg"]);
  expect(asked.at(-1)).toMatchObject({ sort: "updatedAt", order: "desc" });

  // One step back is the folder before, with its order: the two choices were not steps.
  await page.goBack();
  await expect.poll(() => rowNames(page)).toEqual(["beta.txt", "gamma.txt", "alpha.txt", "Photos"]);
});

test("an address with an order it does not know is listed by name", async ({ page }) => {
  const asked = await installFileApi(page);
  await page.goto("/files?sort=colour&order=sideways");
  await expect.poll(() => rowNames(page)).toEqual(["Photos", "alpha.txt", "beta.txt", "gamma.txt"]);
  expect(asked.at(-1)).toMatchObject({ sort: "name", order: "asc" });
});

test("each pane of the split view has its own order", async ({ page, isMobile }) => {
  test.skip(isMobile, "desktop split view");
  await installFileApi(page);
  await page.goto("/files?sort=size");
  await expect.poll(() => rowNames(page)).toEqual(["Photos", "beta.txt", "gamma.txt", "alpha.txt"]);

  // The second pane opens as the first is.
  await page.getByRole("button", { name: "Open split view" }).click();
  const secondary = "file-pane-secondary";
  await expect
    .poll(() => rowNames(page, secondary))
    .toEqual(["Photos", "beta.txt", "gamma.txt", "alpha.txt"]);

  await chooseSort(page, "Name", secondary);
  await expect
    .poll(() => rowNames(page, secondary))
    .toEqual(["Photos", "alpha.txt", "beta.txt", "gamma.txt"]);
  await expect.poll(() => rowNames(page)).toEqual(["Photos", "beta.txt", "gamma.txt", "alpha.txt"]);

  // Closing it leaves the first pane's order.
  await page.getByRole("button", { name: "Close split view" }).click();
  await expect.poll(() => rowNames(page)).toEqual(["Photos", "beta.txt", "gamma.txt", "alpha.txt"]);
  expect(new URL(page.url()).searchParams.get("sort")).toBe("size");
});
