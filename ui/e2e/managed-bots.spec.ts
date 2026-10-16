import { expect, test } from "@playwright/test";
import { installFileApi } from "./file-api.fixture";

test.beforeEach(async ({ page }) => {
  await installFileApi(page);
  await page.route("**/api/v1/bots**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/manager"))
      return route.fulfill({ json: { id: 100, username: "manager_bot", canManageBots: true } });
    if (path.endsWith("/check-username")) return route.fulfill({ json: { available: true } });
    if (path.endsWith("/managed"))
      return route.fulfill({
        json: {
          bot: {
            id: 200,
            username: "storage_bot",
            enabled: false,
            createdAt: "2026-10-16T12:00:00Z",
          },
          recovered: false,
          jobId: "123",
        },
      });
    return route.fulfill({
      json: {
        items: [
          {
            id: 100,
            name: "Gestor principal",
            username: "manager_bot",
            enabled: true,
            createdAt: "2026-10-16T12:00:00Z",
          },
        ],
      },
    });
  });
});

test("creates a bot using an imported manager without exposing tokens", async ({ page }) => {
  let body: unknown;
  await page.route("**/api/v1/bots/managed", async (route) => {
    body = route.request().postDataJSON();
    return route.fulfill({
      json: {
        bot: {
          id: 200,
          username: "storage_bot",
          enabled: false,
          createdAt: "2026-10-16T12:00:00Z",
        },
        recovered: false,
      },
    });
  });
  await page.goto("/settings/bots");
  await expect(
    page.getByRole("option", { name: "Gestor principal · @manager_bot · ID: 100", exact: true }),
  ).toHaveAttribute("value", "100");
  await expect(page.getByText("Gestor principal", { exact: true })).toBeVisible();
  await expect(page.getByText("@manager_bot", { exact: true })).toBeVisible();
  await expect(page.getByText("ID: 100", { exact: true })).toBeVisible();
  const create = page.getByRole("button", { name: "Crear y añadir bot", exact: true });
  await expect(create).toBeDisabled();
  await page.getByLabel("Bot gestor", { exact: true }).selectOption("100");
  await page.getByRole("button", { name: "Verificar gestor", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Gestor verificado: @manager_bot");
  await page.getByLabel("Nombre del nuevo bot", { exact: true }).fill("Storage");
  await page.getByLabel("Usuario del nuevo bot", { exact: true }).fill("storage_bot");
  await page.getByRole("button", { name: "Comprobar usuario", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("El usuario está disponible.");
  await create.click();
  await expect(page.getByRole("status")).toHaveText("Bot creado y añadido: @storage_bot");
  expect(body).toEqual({ managerBotId: 100, name: "Storage", username: "storage_bot" });
  await expect(page.getByRole("button", { name: "Añadir bots", exact: true })).toBeVisible();
});

test("shows manager permissions, occupied names, and interrupted creation recovery", async ({
  page,
}) => {
  await page.route("**/api/v1/bots/100/manager", (route) =>
    route.fulfill({
      status: 422,
      json: { error: { code: "bot_manager_permission", message: "Enable management" } },
    }),
  );
  await page.goto("/settings/bots");
  await page.getByLabel("Bot gestor", { exact: true }).selectOption("100");
  await page.getByRole("button", { name: "Verificar gestor", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Activa Bot Management Mode");
  await expect(
    page.getByRole("button", { name: "Crear y añadir bot", exact: true }),
  ).toBeDisabled();
  await page.unroute("**/api/v1/bots/100/manager");
  await page.getByRole("button", { name: "Verificar gestor", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Gestor verificado: @manager_bot");
  await page.getByLabel("Nombre del nuevo bot", { exact: true }).fill("Storage");
  await page.getByLabel("Usuario del nuevo bot", { exact: true }).fill("storage_bot");
  await page.route("**/api/v1/bots/managed/check-username", (route) =>
    route.fulfill({
      status: 409,
      json: { error: { code: "bot_username_occupied", message: "Occupied" } },
    }),
  );
  await page.getByRole("button", { name: "Comprobar usuario", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("El usuario ya está ocupado");
  await page.route("**/api/v1/bots/managed", (route) =>
    route.fulfill({
      json: {
        bot: { id: 200, username: "storage_bot", enabled: true, createdAt: "2026-10-16T12:00:00Z" },
        recovered: true,
      },
    }),
  );
  await page.getByRole("button", { name: "Crear y añadir bot", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Bot recuperado y añadido: @storage_bot");
});

test("identifies bots by name, alias and ID in sorted lists and deletion", async ({
  page,
}, testInfo) => {
  await page.route("**/api/v1/bots?**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: 302,
            name: "Almacén principal",
            username: "storage_two_bot",
            enabled: false,
            createdAt: "2026-10-16T12:00:00Z",
          },
          {
            id: 100,
            name: "Gestor principal",
            username: "manager_bot",
            enabled: true,
            createdAt: "2026-10-16T12:00:00Z",
          },
          {
            id: 301,
            name: "Almacén principal",
            username: "storage_one_bot",
            enabled: true,
            createdAt: "2026-10-16T12:00:00Z",
          },
        ],
      },
    }),
  );
  await page.goto("/settings/bots");
  await expect(page.getByLabel("Bot gestor", { exact: true }).locator("option")).toHaveText([
    "Selecciona un bot importado",
    "Almacén principal · @storage_one_bot · ID: 301",
    "Almacén principal · @storage_two_bot · ID: 302",
    "Gestor principal · @manager_bot · ID: 100",
  ]);
  await expect(page.getByText("@storage_one_bot", { exact: true })).toBeVisible();
  await expect(page.getByText("@storage_two_bot", { exact: true })).toBeVisible();
  await expect(page.getByText("ID: 301", { exact: true })).toBeVisible();
  await expect(page.getByText("ID: 302", { exact: true })).toBeVisible();
  await page.getByText("Bots configurados", { exact: true }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: testInfo.outputPath("bot-identities.png"), fullPage: true });
  await page
    .getByRole("button", {
      name: "Eliminar bot Almacén principal · @storage_two_bot · ID: 302",
      exact: true,
    })
    .click();
  await expect(page.getByRole("alertdialog")).toContainText(
    "Almacén principal · @storage_two_bot · ID: 302",
  );
});
