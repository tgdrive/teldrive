import { expect, test, type Page } from "@playwright/test";
import { installFileApi } from "./file-api.fixture";

async function nativeBridge(page: Page, platform: "android" | "ios", error?: string) {
  await page.addInitScript(
    ({ platform, error }) => {
      const state = window as typeof window & {
        nativeRequests: Record<string, unknown>[];
        teldriveMobile?: {
          postMessage: (message: string) => void;
          onmessage?: (event: { data: string }) => void;
        };
        webkit?: {
          messageHandlers: {
            teldriveMobile: { postMessage: (request: Record<string, unknown>) => Promise<void> };
          };
        };
      };
      state.nativeRequests = [];
      if (platform === "android") {
        state.teldriveMobile = {
          postMessage(message) {
            const request = JSON.parse(message) as Record<string, unknown>;
            state.nativeRequests.push(request);
            queueMicrotask(() =>
              state.teldriveMobile?.onmessage?.({
                data: JSON.stringify({ id: request.id, error }),
              }),
            );
          },
        };
      } else {
        state.webkit = {
          messageHandlers: {
            teldriveMobile: {
              async postMessage(request) {
                state.nativeRequests.push(request);
                if (error) throw new Error(error);
              },
            },
          },
        };
      }
    },
    { platform, error },
  );
}

for (const platform of ["android", "ios"] as const) {
  test(`${platform} configuration save uses the native file dialog`, async ({ page }) => {
    await installFileApi(page);
    await nativeBridge(page, platform);
    const browserDownloads: string[] = [];
    page.on("download", (download) => browserDownloads.push(download.suggestedFilename()));
    await page.goto("/settings/rclone");
    await page.getByLabel("Clave de API de rclone", { exact: true }).fill("private-test-key");
    await page.getByRole("button", { name: "Descargar rclone.conf", exact: true }).click();
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            (window as typeof window & { nativeRequests: Record<string, unknown>[] }).nativeRequests
              .length,
        ),
      )
      .toBe(1);
    const requests = await page.evaluate(
      () =>
        (window as typeof window & { nativeRequests: Record<string, unknown>[] }).nativeRequests,
    );
    expect(requests[0]).toMatchObject({
      method: "save",
      filename: "rclone.conf",
      mime: "text/plain",
    });
    expect(requests[0].contents).toContain("api_key = private-test-key");
    expect(browserDownloads).toEqual([]);
  });

  test(`${platform} native save cancellation reports the error without a browser download`, async ({
    page,
  }) => {
    await installFileApi(page);
    await nativeBridge(page, platform, "Guardado cancelado");
    const browserDownloads: string[] = [];
    page.on("download", (download) => browserDownloads.push(download.suggestedFilename()));
    await page.goto("/settings/rclone");
    await page.getByLabel("Clave de API de rclone", { exact: true }).fill("private-test-key");
    await page.getByRole("button", { name: "Descargar rclone.conf", exact: true }).click();
    await expect(page.getByText("Guardado cancelado", { exact: true })).toBeVisible();
    expect(browserDownloads).toEqual([]);
  });
}
