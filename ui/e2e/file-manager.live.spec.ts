import { expect, test } from "@playwright/test";

const baseURL = process.env.TELDRIVE_UI_BASE_URL;
const accessToken = process.env.TELDRIVE_UI_TEST_ACCESS_TOKEN;
const refreshToken = process.env.TELDRIVE_UI_TEST_REFRESH_TOKEN;

test("real backend supports the file-manager create, rename, trash, and restore lifecycle", async ({
  context,
  page,
  isMobile,
}) => {
  test.skip(!baseURL || !accessToken || !refreshToken, "requires scripts/test-ui.sh");
  test.skip(isMobile, "the live lifecycle only needs one browser project");

  await context.addCookies([
    { name: "teldrive_access", value: accessToken, url: baseURL },
    { name: "teldrive_refresh", value: refreshToken, url: baseURL },
  ]);

  const originalName = `Playwright folder ${Date.now()}`;
  const renamedName = `${originalName} renamed`;

  await page.goto("/files");
  await page.getByRole("button", { name: "New folder" }).click();
  const createDialog = page.getByRole("dialog", { name: "Create folder" });
  await createDialog.getByRole("textbox", { name: "Folder name" }).fill(originalName);
  await createDialog.getByRole("textbox", { name: "Folder name" }).press("Enter");
  await expect(page.getByText(originalName, { exact: true })).toBeVisible();

  await page.getByRole("row", { name: originalName }).click();
  await page.keyboard.press("F2");
  const renameDialog = page.getByRole("dialog", { name: "Rename item" });
  const renameInput = renameDialog.getByRole("textbox", { name: "New name" });
  await renameInput.fill(renamedName);
  await renameInput.press("Enter");
  await expect(page.getByText(renamedName, { exact: true })).toBeVisible();

  await page.getByRole("row", { name: renamedName }).click();
  const trashRequest = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/files/bulk/trash") && response.request().method() === "POST",
  );
  await page.keyboard.press("Delete");
  await expect((await trashRequest).ok()).toBe(true);
  await expect(page.getByText(renamedName, { exact: true })).toBeHidden();

  const basicTrashList = await page.request.get(
    "/api/v1/files?status=trashed&sort=name&order=asc&limit=100",
  );
  expect(basicTrashList.ok()).toBe(true);
  const basicTrashItems = (await basicTrashList.json()) as { items: Array<{ name: string }> };
  expect(basicTrashItems.items.map((item) => item.name)).toContain(renamedName);

  const trashList = await page.request.get(
    "/api/v1/files?status=trashed&sort=updatedAt&order=desc&limit=100",
  );
  expect(trashList.ok()).toBe(true);
  const trashItems = (await trashList.json()) as { items: Array<{ name: string }> };
  expect(trashItems.items.map((item) => item.name)).toContain(renamedName);

  await page.goto("/trash");
  const trashedItem = page.getByText(renamedName, { exact: true });
  await expect(trashedItem).toBeVisible();
  await trashedItem
    .locator("xpath=ancestor::div[contains(@class, 'grid')][1]")
    .getByRole("button", { name: "Restore" })
    .click();
  await expect(trashedItem).toBeHidden();

  await page.goto("/files");
  await expect(page.getByText(renamedName, { exact: true })).toBeVisible();
});
