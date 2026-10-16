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
          { id: 100, username: "manager_bot", enabled: true, createdAt: "2026-10-16T12:00:00Z" },
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
