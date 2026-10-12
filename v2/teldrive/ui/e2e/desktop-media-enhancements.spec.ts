import { expect, test } from "@playwright/test";
import { installFileApi, alphaId } from "./file-api.fixture";

test.beforeEach(async ({ page }) => installFileApi(page));

test("HTTP casting loads both Google SDK components through HTTPS", async ({ page }) => {
  const requests: string[] = [];
  await page.route("**/www.gstatic.com/**", async (route) => {
    const url = route.request().url();
    requests.push(url);
    if (url.includes("cast_framework.js")) {
      await new Promise((resolve) => setTimeout(resolve, 200));
      await route.fulfill({
        contentType: "application/javascript",
        body: `window.cast={framework:{CastContext:{getInstance:()=>({setOptions(){},getCurrentSession(){return {loadMedia:async()=>{window.didCast=true}}},endCurrentSession(){}})}}};`,
      });
    } else {
      await route.fulfill({
        contentType: "application/javascript",
        body: `window.chrome=window.chrome||{};window.chrome.cast={AutoJoinPolicy:{ORIGIN_SCOPED:"origin"},media:{DEFAULT_MEDIA_RECEIVER_APP_ID:"CC1AD845",MediaInfo:class{},GenericMediaMetadata:class{},LoadRequest:class{}}};window.__onGCastApiAvailable(true);`,
      });
    }
  });
  await page.goto(
    "/cast#" +
      new URLSearchParams({
        url: "http://192.168.1.50/playback?ticket=temporary",
        type: "video/mp4",
      }),
  );
  await page.getByRole("button", { name: "Chromecast / Google Cast", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Vídeo enviado");
  expect(requests).toHaveLength(2);
  expect(requests.every((url) => new URL(url).protocol === "https:")).toBe(true);
  expect(requests.some((url) => url.includes("cast_framework.js"))).toBe(true);
  expect(requests.some((url) => url.includes("clank/cast_sender.js"))).toBe(true);
});

test("storage follows trash and shared UI uses the application icon", async ({ page }) => {
  await page.goto("/files");
  if (test.info().project.name !== "chromium")
    await page.getByRole("button", { name: "Abrir navegación" }).click();
  const links = await page
    .locator("aside nav a")
    .evaluateAll((nodes) => nodes.map((node) => node.getAttribute("href")));
  expect(links.indexOf("/storage")).toBe(links.indexOf("/trash") + 1);
  await expect(page.locator('aside img[src="/images/teldrive.png"]').first()).toBeVisible();
});

test("custom config imports and edits multiple remotes without losing comments", async ({
  page,
}) => {
  await page.goto("/settings/rclone");
  const original =
    "# mantener comentario\n[primera]\ntype = alias\nremote = /datos\napi_key = secreto-local\n\n[segunda]\ntype = s3\nregion = original\ncustom_flag = keep\n";
  await page
    .locator('input[type="file"]')
    .setInputFiles({ name: "rclone.conf", mimeType: "text/plain", buffer: Buffer.from(original) });
  await expect(page.getByLabel("rclone api_key", { exact: true })).toHaveAttribute(
    "type",
    "password",
  );
  await page.getByLabel("Conexión del archivo rclone.conf").selectOption("segunda");
  await page.getByLabel("rclone region", { exact: true }).fill("nueva-region");
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Exportar archivo editado", exact: true }).click();
  const download = await downloadPromise;
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  const content = Buffer.concat(chunks).toString();
  expect(content).toContain("# mantener comentario");
  expect(content).toContain("[primera]");
  expect(content).toContain("api_key = secreto-local");
  expect(content).toContain("region = nueva-region");
  expect(content).toContain("custom_flag = keep");
});

test("MKV selects audio, text/bitmap subtitles and sends selected tracks to DLNA", async ({
  page,
}) => {
  const movie = {
    id: alphaId,
    name: "varias-pistas.mkv",
    kind: "file",
    status: "active",
    generation: 1,
    mimeType: "video/x-matroska",
    size: 4096,
    modTime: "2026-07-01T00:00:00Z",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    encryption: false,
  };
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/v1/files") return route.fulfill({ json: { items: [movie] } });
    if (url.pathname.endsWith("/playback"))
      return route.fulfill({
        status: 201,
        json: {
          ticket: "a".repeat(64),
          expiresAt: "2099-01-01T00:00:00Z",
          conversionAvailable: true,
        },
      });
    if (url.pathname.includes("/content/"))
      return route.fulfill({ contentType: "video/x-matroska", body: Buffer.alloc(16) });
    return route.fallback();
  });
  await page.route("**/api/v1/playback/tracks?*", (route) =>
    route.fulfill({
      json: {
        available: true,
        tracks: [
          {
            index: 1,
            kind: "audio",
            codec: "aac",
            language: "spa",
            title: "Español",
            default: true,
            bitmap: false,
          },
          {
            index: 2,
            kind: "audio",
            codec: "aac",
            language: "eng",
            title: "Inglés",
            default: false,
            bitmap: false,
          },
          {
            index: 3,
            kind: "subtitle",
            codec: "subrip",
            language: "spa",
            title: "Texto",
            default: false,
            bitmap: false,
          },
          {
            index: 4,
            kind: "subtitle",
            codec: "hdmv_pgs_subtitle",
            language: "eng",
            title: "PGS",
            default: false,
            bitmap: true,
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/playback?*", (route) =>
    route.fulfill({ contentType: "video/mp4", body: Buffer.alloc(16) }),
  );
  await page.route("**/api/v1/playback/subtitles?*", (route) =>
    route.fulfill({
      contentType: "text/vtt",
      body: "WEBVTT\n\n00:00.100 --> 00:01.000\nTexto de prueba\n",
    }),
  );
  await page.route("**/api/v1/casting/relay", (route) =>
    route.fulfill({
      status: 201,
      json: { addresses: ["http://192.168.1.50:32100"], expiresAt: "2099-01-01T00:00:00Z" },
    }),
  );
  await page.route("**/api/v1/casting/devices", (route) =>
    route.fulfill({
      json: { devices: [{ id: "tv-test", name: "Televisor de prueba", protocol: "dlna" }] },
    }),
  );
  let castBody: Record<string, unknown> | undefined;
  await page.route("**/api/v1/casting/devices/tv-test/play", (route) => {
    castBody = route.request().postDataJSON();
    return route.fulfill({ status: 204 });
  });
  await page.goto("/files");
  await page.getByRole("row", { name: /varias-pistas.mkv/ }).dblclick();
  await page.getByLabel("Pista de audio").selectOption("2");
  await expect(page.locator("video")).toHaveAttribute("src", /audio=2/);
  await page.getByLabel("Pista de subtítulos").selectOption("4");
  await expect(page.locator("video")).toHaveAttribute("src", /subtitle=4/);
  await page.getByLabel("Pista de subtítulos").selectOption("");
  await expect(page.locator("video")).not.toHaveAttribute("src", /subtitle=/);
  await page.getByLabel("Pista de subtítulos").selectOption("3");
  await page.getByRole("button", { name: "Transmitir", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Transmitir vídeo" });
  await expect(dialog.getByLabel("Dirección para transmitir")).toHaveValue(
    "http://192.168.1.50:32100",
  );
  await dialog.getByRole("button", { name: "Buscar televisores DLNA" }).click();
  await dialog.getByRole("button", { name: "Transmitir a este TV" }).click();
  await expect(dialog.getByRole("status")).toContainText("Televisor de prueba");
  expect(castBody).toMatchObject({ audio: 2, subtitle: 3, mode: "video", ticket: "a".repeat(64) });
});

test("casting page opens without a browser login and hands off to Google Cast", async ({
  page,
}) => {
  await page.route("**/api/v1/me", (route) =>
    route.fulfill({
      status: 401,
      json: { error: { code: "unauthorized", message: "Inicia sesión" } },
    }),
  );
  await page.addInitScript(() => {
    let loaded: unknown;
    const session = {
      loadMedia: async (value: unknown) => {
        loaded = value;
        Object.assign(window, { castLoaded: loaded });
      },
    };
    const context = {
      setOptions: () => {},
      getCurrentSession: () => session,
      requestSession: async () => {},
      endCurrentSession: () => {},
    };
    Object.assign(window, { cast: { framework: { CastContext: { getInstance: () => context } } } });
    Object.assign(window.chrome ?? (window.chrome = {}), {
      cast: {
        AutoJoinPolicy: { ORIGIN_SCOPED: "origin" },
        media: {
          DEFAULT_MEDIA_RECEIVER_APP_ID: "CC1AD845",
          MediaInfo: class {
            constructor(
              public contentId: string,
              public contentType: string,
            ) {}
          },
          GenericMediaMetadata: class {},
          LoadRequest: class {
            constructor(public media: unknown) {}
          },
        },
      },
    });
  });
  const url = "http://192.168.1.50:32100/playback?ticket=temporal&mode=video";
  await page.goto(
    "/cast#" + new URLSearchParams({ url, title: "Vídeo de prueba", type: "video/mp4" }),
  );
  await page.getByRole("button", { name: "Chromecast / Google Cast", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Vídeo enviado");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as Window & { castLoaded?: { media: { contentId: string } } }).castLoaded?.media
            .contentId,
      ),
    )
    .toBe(url);
});
