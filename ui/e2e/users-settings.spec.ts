import { expect, type Route, test } from "@playwright/test";
import { installPreferencesApi } from "./fixtures/preferences";

test("QR login serializes slow polls and completes two-step password verification", async ({
  page,
}) => {
  const time = new Date("2026-10-11T12:00:00Z");
  await page.clock.install({ time });
  await page.clock.pauseAt(time);
  const flowId = "11111111-1111-4111-8111-111111111111";
  let pollRequests = 0;
  let authenticated = false;
  let capturePoll!: (route: Route) => void;
  const pendingPoll = new Promise<Route>((resolve) => {
    capturePoll = resolve;
  });
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/v1/me") {
      if (!authenticated)
        return route.fulfill({
          status: 401,
          json: { error: { code: "unauthorized", message: "Sign in" } },
        });
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "QR User",
          role: "owner",
          capabilities: [],
          premium: false,
          createdAt: time.toISOString(),
        },
      });
    }
    if (path === "/api/v1/auth/telegram/qr/start") {
      return route.fulfill({
        json: {
          flowId,
          state: "pending",
          expiresAt: "2026-10-11T12:10:00Z",
          qrUrl: "tg://login?token=fixture",
          qrExpiresAt: "2026-10-11T12:05:00Z",
        },
      });
    }
    if (path === "/api/v1/auth/cookie/telegram/qr/poll") {
      expect(route.request().postDataJSON()).toEqual({ flowId });
      pollRequests++;
      if (pollRequests === 1) {
        capturePoll(route);
        return;
      }
      return route.fulfill({
        json: {
          flowId,
          state: "pending",
          expiresAt: "2026-10-11T12:10:00Z",
          qrUrl: "tg://login?token=consumed",
        },
      });
    }
    if (path === "/api/v1/auth/cookie/telegram/verify-password") {
      expect(route.request().postDataJSON()).toEqual({ flowId, password: "correct-password" });
      authenticated = true;
      return route.fulfill({ json: { authenticated: true, expiresAt: "2026-10-12T12:00:00Z" } });
    }
    if (path === "/api/v1/files") return route.fulfill({ json: { items: [] } });
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/login");
  await page.getByRole("tab", { name: "QR code", exact: true }).click();
  await expect(page.getByLabel("Telegram sign-in QR code")).toBeVisible();
  await page.clock.runFor(2500);
  const poll = await pendingPoll;
  await page.clock.runFor(7500);
  expect(pollRequests).toBe(1);
  await poll.fulfill({
    json: { flowId, state: "password_required", expiresAt: "2026-10-11T12:10:00Z" },
  });
  await expect(page.getByLabel("Two-step verification password", { exact: true })).toBeVisible();
  await page.clock.runFor(7500);
  expect(pollRequests).toBe(1);
  await page.getByLabel("Two-step verification password", { exact: true }).fill("correct-password");
  await page.getByRole("button", { name: "Verify and sign in", exact: true }).click();
  await expect(page).toHaveURL(/\/files/);
  expect(authenticated).toBe(true);
});

test("channel discovery previews all channels and syncs only checked choices", async ({ page }) => {
  let syncRequests = 0;
  let registered = false;
  const discovered = [
    { id: 9001, name: "Storage" },
    { id: 9002, name: "Personal" },
  ];
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture",
          role: "owner",
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/channels/discover") {
      return route.fulfill({ json: discovered });
    }
    if (url.pathname === "/api/v1/channels") {
      return route.fulfill({
        json: {
          items: registered
            ? [
                {
                  ...discovered[0],
                  selected: false,
                  health: "unknown",
                  createdAt: "2026-07-22T12:00:00Z",
                },
              ]
            : [],
        },
      });
    }
    if (url.pathname === "/api/v1/channels/sync") {
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers()["idempotency-key"]).toBeTruthy();
      expect(route.request().postDataJSON()).toEqual({ channelIds: [9001] });
      syncRequests++;
      if (syncRequests === 1) {
        return route.fulfill({
          status: 503,
          json: { error: { code: "unavailable", message: "Try again" } },
        });
      }
      registered = true;
      return route.fulfill({ json: [] });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/settings/channels");
  await page.getByRole("button", { name: "Discover and sync", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("checkbox", { name: "Storage", exact: true })).toBeChecked();
  await expect(dialog.getByRole("checkbox", { name: "Personal", exact: true })).toBeChecked();
  expect(syncRequests).toBe(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(syncRequests).toBe(0);
  await page.getByRole("button", { name: "Discover and sync", exact: true }).click();
  await dialog.getByText("Personal", { exact: true }).click();
  await expect(dialog.getByRole("checkbox", { name: "Personal", exact: true })).not.toBeChecked();
  await dialog.getByRole("button", { name: "Sync selected channels", exact: true }).click();
  await expect(page.getByText("Channel sync failed", { exact: true })).toBeVisible();
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("checkbox", { name: "Personal", exact: true })).not.toBeChecked();
  await page.locator('[data-sonner-toast][data-type="error"] [data-close-button]').click();
  await dialog.getByRole("button", { name: "Sync selected channels", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("Channels synchronized", { exact: true })).toBeVisible();
  expect(syncRequests).toBe(2);
});

test("bots can be manually provisioned after migration", async ({ page }) => {
  let provisionRequests = 0;
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture",
          role: "owner",
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/bots") {
      return route.fulfill({
        json: { items: [{ id: 777, enabled: true, createdAt: "2026-07-22T12:00:00Z" }] },
      });
    }
    if (url.pathname === "/api/v1/bots/777/provision") {
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers()["idempotency-key"]).toBeTruthy();
      provisionRequests++;
      return route.fulfill({ status: 202, json: { jobId: "123" } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/settings/bots");
  await expect(page.getByRole("button", { name: "Provision all bots", exact: true })).toBeEnabled();
  await expect(page.getByText("Enabled", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Provision bot 777", exact: true }).click();
  await expect(page.getByRole("alertdialog")).toBeVisible();
  expect(provisionRequests).toBe(0);
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Provision bot", exact: true })
    .click();
  await expect(page.getByText("Bot provisioning queued", { exact: true })).toBeVisible();
  expect(provisionRequests).toBe(1);
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
});

test("bulk bot provisioning keeps single actions and can retry queue errors", async ({ page }) => {
  let bulkRequests = 0;
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture",
          role: "owner",
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/bots") {
      return route.fulfill({
        json: {
          items: [
            { id: 777, enabled: true, createdAt: "2026-07-22T12:00:00Z" },
            { id: 778, enabled: false, createdAt: "2026-07-22T12:00:00Z" },
          ],
          nextCursor: "more-bots",
        },
      });
    }
    if (url.pathname === "/api/v1/bots/provision") {
      expect(route.request().method()).toBe("POST");
      expect(route.request().headers()["idempotency-key"]).toBeTruthy();
      expect(route.request().postData()).toBeNull(); // Server selects all owned bots, not just this page.
      bulkRequests++;
      if (bulkRequests === 1) {
        return route.fulfill({
          status: 503,
          json: { error: { code: "unavailable", message: "Try again" } },
        });
      }
      return route.fulfill({ status: 202, json: { jobIds: ["123", "124", "125"] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/settings/bots");
  await expect(page.getByRole("button", { name: "Provision bot 777", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Provision bot 778", exact: true })).toBeVisible();
  const configuredBots = page
    .locator('[data-slot="card"]')
    .filter({ has: page.getByText("Configured bots", { exact: true }) });
  await configuredBots
    .locator('[data-slot="card-header"]')
    .getByRole("button", { name: "Provision all bots", exact: true })
    .click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toContainText("including bots not shown on this page");
  expect(bulkRequests).toBe(0);
  await dialog.getByRole("button", { name: "Provision all bots", exact: true }).click();
  await expect(
    page.getByText("Bots could not be queued for provisioning", { exact: true }),
  ).toBeVisible();
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
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture",
          role: "owner",
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (url.pathname === "/api/v1/bots") {
      if (route.request().method() === "POST") {
        expect(route.request().postDataJSON()).toEqual({ tokens: ["777:secret", "778:secret"] });
        added = true;
        return route.fulfill({
          json: { bots, failedIndexes: [], jobId: "123", jobIds: ["123", "124"] },
        });
      }
      return route.fulfill({ json: { items: added ? bots : [] } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/settings/bots");
  const bulk = page.getByRole("button", { name: "Provision all bots", exact: true });
  await expect(bulk).toBeDisabled();
  await page
    .getByRole("textbox", { name: "Bot tokens", exact: true })
    .fill("777:secret\n778:secret");
  await page.getByRole("button", { name: "Add bots", exact: true }).click();
  await expect(page.getByText("2 bots queued for verification", { exact: true })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Bot tokens", exact: true })).toHaveValue("");
  await expect(bulk).toBeEnabled();
});

test("settings navigation separates appearance, reader and player controls", async ({
  page,
  isMobile,
}) => {
  await page.route("**/api/v1/**", (route) => {
    if (new URL(route.request().url()).pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Reader",
          role: "user",
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/settings/appearance");
  await expect(page.getByRole("button", { name: "Light", exact: true })).toBeVisible();
  await expect(page.getByRole("slider", { name: "PDF page brightness", exact: true })).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("switch", { name: "Read tracks from the file", exact: true }),
  ).toHaveCount(0);

  async function navigate(name: string) {
    if (isMobile)
      await page.getByRole("button", { name: "Open settings navigation", exact: true }).click();
    await page
      .getByRole("navigation", { name: "Settings navigation", exact: true })
      .getByRole("link", { name, exact: true })
      .click();
  }

  await navigate("Reader");
  await expect(page).toHaveURL(/\/settings\/reader$/);
  const brightness = page.getByRole("slider", { name: "PDF page brightness", exact: true });
  await expect(brightness).toBeVisible();
  await expect(
    page.getByRole("switch", { name: "Keep documents in this browser", exact: true }),
  ).toBeVisible();
  await brightness.focus();
  await brightness.press("Home");
  await page.reload();
  await expect(brightness).toHaveValue("50");
  await expect(
    page.getByRole("switch", { name: "Read tracks from the file", exact: true }),
  ).toHaveCount(0);

  await navigate("Player");
  await expect(page).toHaveURL(/\/settings\/player$/);
  await expect(
    page.getByRole("switch", { name: "Read tracks from the file", exact: true }),
  ).toBeVisible();
  await expect(brightness).toHaveCount(0);
  await navigate("Appearance");
  await expect(page).toHaveURL(/\/settings\/appearance$/);
  await expect(page.getByRole("button", { name: "Light", exact: true })).toBeVisible();
});

test("appearance applies light tokens and persists across reloads", async ({ page }) => {
  await page.route("**/api/v1/**", async (route) => {
    if (new URL(route.request().url()).pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture",
          role: "owner",
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    if (new URL(route.request().url()).pathname === "/api/v1/files/statistics/drive") {
      return route.fulfill({ json: { totalFiles: 0, totalBytes: 0, openUploads: 0 } });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/settings/appearance");
  await page.emulateMedia({ colorScheme: "dark" });
  const root = page.locator("html");
  await expect(root).toHaveAttribute("data-theme", "dark");
  const darkBackground = await root.evaluate((el) =>
    getComputedStyle(el).getPropertyValue("--background"),
  );
  await page.getByRole("button", { name: "Light", exact: true }).click();
  await expect(root).toHaveAttribute("data-theme", "light");
  await expect(root).not.toHaveClass(/dark/);
  await expect(root).toHaveCSS("color-scheme", "light");
  await expect
    .poll(() => root.evaluate((el) => getComputedStyle(el).getPropertyValue("--background")))
    .not.toBe(darkBackground);
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
  await expect
    .poll(() => errorToast.evaluate((el) => getComputedStyle(el).backgroundColor))
    .not.toBe(darkToastColors.background);
});

test("language switch translates shell and persists across reloads", async ({ page }) => {
  await page.route("**/api/v1/**", async (route) => {
    if (new URL(route.request().url()).pathname === "/api/v1/me") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture",
          role: "owner",
          capabilities: [],
          premium: false,
          createdAt: "2026-07-22T12:00:00Z",
        },
      });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  await installPreferencesApi(page);
  await page.goto("/settings/appearance");
  await expect(
    page.getByRole("heading", { name: "Appearance", exact: true, level: 1 }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Español", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Apariencia", exact: true, level: 1 }),
  ).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("lang", "es");
  await expect(page.getByPlaceholder("Buscar archivos")).toBeVisible();
  const openNav = page.getByRole("button", { name: "Abrir navegación", exact: true });
  if (await openNav.isVisible()) await openNav.click();
  await expect(page.getByRole("link", { name: "Archivos", exact: true })).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Apariencia", exact: true, level: 1 }),
  ).toBeVisible();
  await page.getByRole("button", { name: "English", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Appearance", exact: true, level: 1 }),
  ).toBeVisible();
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

  await installPreferencesApi(page);
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
