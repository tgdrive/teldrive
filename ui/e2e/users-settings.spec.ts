import { expect, test } from "@playwright/test";

test("users search is debounced and stays usable while results load", async ({ page }) => {
  const users = [
    { userId: 1, displayName: "Instance Owner", role: "owner", disabled: false },
    { userId: 2, displayName: "Alice", username: "alice", role: "user", disabled: false },
  ];
  const searches: (string | null)[] = [];
  let releaseSearch!: () => void;
  const searchResponse = new Promise<void>((resolve) => {
    releaseSearch = resolve;
  });

  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          ...users[0],
          displayName: "Fixture Admin",
          capabilities: ["system.manageUsers", "system.owner"],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/admin/users") {
      const search = url.searchParams.get("search");
      searches.push(search);
      if (search) await searchResponse;
      return route.fulfill({
        json: search ? users.filter((user) => user.displayName.includes(search)) : users,
      });
    }
    return route.fulfill({ status: 404, json: {} });
  });

  await page.goto("/settings/users");
  await expect(page.getByText("Instance Owner", { exact: true })).toBeVisible();
  expect(searches).toEqual([null]);
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1_000));

  const input = page.getByRole("textbox", { name: "Search users" });
  await input.fill("A");
  await page.clock.runFor(200);
  await input.fill("Al");
  await page.clock.runFor(200);
  await input.fill(" Alice ");
  await page.clock.runFor(299);
  expect(searches).toEqual([null]);
  await expect(input).toHaveValue(" Alice ");

  await page.clock.runFor(1);
  await expect.poll(() => searches).toEqual([null, "Alice"]);
  await expect(input).toBeVisible();
  await expect(input).toBeFocused();
  await expect(page.getByText("Instance Owner", { exact: true })).toBeVisible();
  const response = page.waitForResponse((response) =>
    response.url().includes("/api/v1/admin/users?search=Alice"),
  );
  releaseSearch();
  await (await response).finished();
  await page.clock.runFor(1);
  await expect(page.getByText("Instance Owner", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Alice", { exact: true })).toBeVisible();

  // Whitespace-only changes should not issue another search.
  await input.fill("Alice");
  await page.clock.runFor(300);
  expect(searches).toEqual([null, "Alice"]);

  // Clearing restores the cached unfiltered list without an unnecessary request.
  await input.fill("");
  await page.clock.runFor(300);
  await expect(page.getByText("Instance Owner", { exact: true })).toBeVisible();
  expect(searches).toEqual([null, "Alice"]);
});
