import { expect, test } from "@playwright/test";

test("bots can be manually provisioned after migration", async ({ page }) => {
  let provisionRequests = 0;
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({ json: { userId: 1, displayName: "Fixture", role: "owner", capabilities: [], premium: false, createdAt: "2026-07-22T12:00:00Z" } });
    }
    if (url.pathname === "/api/v1/bots") {
      return route.fulfill({ json: { items: [{ id: 777, enabled: true, createdAt: "2026-07-22T12:00:00Z" }] } });
    }
    if (url.pathname === "/api/v1/bots/777/provision") {
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers()["idempotency-key"]).toBeTruthy();
      provisionRequests++;
      return route.fulfill({ status: 202, json: { jobId: "123" } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/settings/bots");
  await expect(page.getByRole("button", { name: "Provision all bots", exact: true })).toBeEnabled();
  await expect(page.getByText("Enabled", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Provision bot 777", exact: true }).click();
  await expect(page.getByRole("alertdialog")).toBeVisible();
  expect(provisionRequests).toBe(0);
  await page.getByRole("alertdialog").getByRole("button", { name: "Provision bot", exact: true }).click();
  await expect(page.getByText("Bot provisioning queued", { exact: true })).toBeVisible();
  expect(provisionRequests).toBe(1);
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
});

test("bulk bot provisioning keeps single actions and can retry queue errors", async ({ page }) => {
  let bulkRequests = 0;
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({ json: { userId: 1, displayName: "Fixture", role: "owner", capabilities: [], premium: false, createdAt: "2026-07-22T12:00:00Z" } });
    }
    if (url.pathname === "/api/v1/bots") {
      return route.fulfill({ json: { items: [
        { id: 777, enabled: true, createdAt: "2026-07-22T12:00:00Z" },
        { id: 778, enabled: false, createdAt: "2026-07-22T12:00:00Z" },
      ], nextCursor: "more-bots" } });
    }
    if (url.pathname === "/api/v1/bots/provision") {
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers()["idempotency-key"]).toBeTruthy();
      expect(route.request().postData()).toBeNull(); // Server selects all owned bots, not just this page.
      bulkRequests++;
      if (bulkRequests === 1) {
        return route.fulfill({ status: 503, json: { error: { code: "unavailable", message: "Try again" } } });
      }
      return route.fulfill({ status: 202, json: { jobIds: ["123", "124", "125"] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/settings/bots");
  await expect(page.getByRole("button", { name: "Provision bot 777", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Provision bot 778", exact: true })).toBeVisible();
  const configuredBots = page.locator('[data-slot="card"]').filter({ has: page.getByText("Configured bots", { exact: true }) });
  await configuredBots.locator('[data-slot="card-header"]').getByRole("button", { name: "Provision all bots", exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("including bots not shown on this page");
  expect(bulkRequests).toBe(0);
  await dialog.getByRole("button", { name: "Provision all bots", exact: true }).click();
  await expect(page.getByText("Bots could not be queued for provisioning", { exact: true })).toBeVisible();
  await expect(dialog).toBeVisible();
  // On mobile, dismiss the bottom toast before retrying the bottom-sheet action.
  await page.locator('[data-sonner-toast][data-type="error"] [data-close-button]').click();
  await dialog.getByRole("button", { name: "Provision all bots", exact: true }).click();
  await expect(page.getByText("3 bots queued for provisioning", { exact: true })).toBeVisible();
  expect(bulkRequests).toBe(2);
  await expect(dialog).toHaveCount(0);
});

test("adding bots supports independent provisioning task IDs", async ({ page }) => {
  let added = false;
  const bots = [777, 778].map((id) => ({ id, enabled: false, createdAt: "2026-07-22T12:00:00Z" }));
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({ json: { userId: 1, displayName: "Fixture", role: "owner", capabilities: [], premium: false, createdAt: "2026-07-22T12:00:00Z" } });
    }
    if (url.pathname === "/api/v1/bots") {
      if (route.request().method() === "POST") {
        expect(route.request().postDataJSON()).toEqual({ tokens: ["777:secret", "778:secret"] });
        added = true;
        return route.fulfill({ json: { bots, failedIndexes: [], jobId: "123", jobIds: ["123", "124"] } });
      }
      return route.fulfill({ json: { items: added ? bots : [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/settings/bots");
  const bulk = page.getByRole("button", { name: "Provision all bots", exact: true });
  await expect(bulk).toBeDisabled();
  await page.getByRole("textbox", { name: "Bot tokens", exact: true }).fill("777:secret\n778:secret");
  await page.getByRole("button", { name: "Add bots", exact: true }).click();
  await expect(page.getByText("2 bots queued for verification", { exact: true })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Bot tokens", exact: true })).toHaveValue("");
  await expect(bulk).toBeEnabled();
});

test("appearance applies light tokens and persists across reloads", async ({ page }) => {
  await page.route("**/api/v1/**", async (route) => {
    if (new URL(route.request().url()).pathname === "/api/v1/me") {
      return route.fulfill({ json: { userId: 1, displayName: "Fixture", role: "owner", capabilities: [], premium: false, createdAt: "2026-07-22T12:00:00Z" } });
    }
    if (new URL(route.request().url()).pathname === "/api/v1/files/statistics/drive") {
      return route.fulfill({ json: { totalFiles: 0, totalBytes: 0, openUploads: 0 } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await page.goto("/settings/appearance");
  await page.emulateMedia({ colorScheme: "dark" });
  const root = page.locator("html");
  await expect(root).toHaveAttribute("data-theme", "dark");
  const darkBackground = await root.evaluate((el) => getComputedStyle(el).getPropertyValue("--background"));
  await page.getByRole("button", { name: "Light", exact: true }).click();
  await expect(root).toHaveAttribute("data-theme", "light");
  await expect(root).not.toHaveClass(/dark/);
  await expect(root).toHaveCSS("color-scheme", "light");
  await expect.poll(() => root.evaluate((el) => getComputedStyle(el).getPropertyValue("--background"))).not.toBe(darkBackground);
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.getByRole("button", { name: "System", exact: true }).click();
  await expect(root).toHaveAttribute("data-theme", "dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "light");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(root).toHaveAttribute("data-theme", "dark");
  await page.goto("/settings");
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  await expect(page.getByText("Unable to log out", { exact: true })).toBeVisible();
  await expect(page.locator("[data-sonner-toaster]")).toHaveAttribute("data-sonner-theme", "dark");
  const errorToast = page.locator('[data-sonner-toast][data-type="error"]');
  const darkToastColors = await errorToast.evaluate((el) => {
    const style = getComputedStyle(el);
    return { background: style.backgroundColor, text: style.color, border: style.borderColor };
  });
  const errorText = await errorToast.evaluate((el) => {
    const probe = document.createElement("span");
    probe.style.color = "var(--error-text)";
    el.appendChild(probe);
    const color = getComputedStyle(probe).color;
    probe.remove();
    return color;
  });
  expect(darkToastColors.text).toBe(errorText);
  expect(darkToastColors.background).toBe("oklch(0.21 0.008 70 / 0.85)");
  expect(darkToastColors.border).toBe("oklch(0.95 0.02 70 / 0.1)");
  await expect(errorToast).toHaveCSS("backdrop-filter", "blur(16px)");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("[data-sonner-toaster]")).toHaveAttribute("data-sonner-theme", "light");
  await expect.poll(() => errorToast.evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe(darkToastColors.background);
});

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
