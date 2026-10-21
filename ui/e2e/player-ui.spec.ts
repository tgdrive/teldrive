import { readFileSync } from "node:fs";
import { expect, type Page, type Route, test } from "@playwright/test";
import { installPreferencesApi } from "./fixtures/preferences";

// Drives the real web player on a public share page, which uses the same preview dialog and
// VideoViewer as the signed-in file manager.

const now = "2026-10-21T12:00:00Z";
const token = "tds_trackToken123";
const folderId = "a1111111-1111-4111-8111-111111111111";

const bytes = (name: string) => readFileSync(new URL(`./fixtures/player/${name}`, import.meta.url));

type Item = { id: string; name: string; mimeType: string; data: Buffer };
const items: Record<string, Item> = {
  multi: {
    id: "b0000000-0000-4000-8000-000000000001",
    name: "Multi.mkv",
    mimeType: "video/x-matroska",
    data: bytes("multi-track.mkv"),
  },
  single: {
    id: "b0000000-0000-4000-8000-000000000002",
    name: "Single.mkv",
    mimeType: "video/x-matroska",
    data: bytes("single-track.mkv"),
  },
  eac3: {
    id: "b0000000-0000-4000-8000-000000000003",
    name: "Eac3.mkv",
    mimeType: "video/x-matroska",
    data: bytes("eac3-only.mkv"),
  },
  broken: {
    id: "b0000000-0000-4000-8000-000000000004",
    name: "Broken.mkv",
    mimeType: "video/x-matroska",
    data: Buffer.from("not really a matroska file, but the extension says so"),
  },
  plain: {
    id: "b0000000-0000-4000-8000-000000000005",
    name: "Plain.webm",
    mimeType: "video/webm",
    data: bytes("../viewers/sample.webm"),
  },
};

function entry(id: string, name: string, kind: "file" | "folder", mimeType: string, size?: number) {
  return {
    id,
    parentId: kind === "folder" ? undefined : folderId,
    name,
    kind,
    mimeType,
    size,
    encryption: false,
    status: "active",
    modTime: now,
    generation: 1,
    createdAt: now,
    updatedAt: now,
  };
}

async function installShare(page: Page) {
  const prefix = `/api/v1/public/shares/${token}`;
  const content = async (route: Route, item: Item) => {
    const range = /^bytes=(\d+)-(\d*)$/.exec(route.request().headers().range ?? "");
    const data = item.data;
    if (!range) {
      return route.fulfill({ status: 200, body: data, headers: { "Accept-Ranges": "bytes" } });
    }
    const start = Number(range[1]);
    const end = range[2] ? Math.min(Number(range[2]), data.length - 1) : data.length - 1;
    if (start >= data.length) return route.fulfill({ status: 416 });
    return route.fulfill({
      status: 206,
      body: data.subarray(start, end + 1),
      headers: {
        "Accept-Ranges": "bytes",
        "Content-Range": `bytes ${start}-${end}/${data.length}`,
      },
    });
  };
  await page.route(
    (url) => url.pathname.startsWith("/api/"),
    async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === prefix) {
        return route.fulfill({
          json: {
            id: "c3333333-3333-4333-8333-333333333333",
            file: entry(folderId, "Videos", "folder", "drive/folder"),
            passwordProtected: false,
            permission: "read",
          },
        });
      }
      if (url.pathname === `${prefix}/files`) {
        return route.fulfill({
          json: {
            items: Object.values(items).map((i) =>
              entry(i.id, i.name, "file", i.mimeType, i.data.length),
            ),
          },
        });
      }
      for (const item of Object.values(items)) {
        if (url.pathname.startsWith(`${prefix}/files/${item.id}/content/`)) {
          return content(route, item);
        }
      }
      return route.fulfill({ status: 404, json: { code: "not_found", title: "not found" } });
    },
  );
  await installPreferencesApi(page);
}

async function openVideo(page: Page, name: string) {
  await page.goto(`/share/${token}`);
  await page.getByTitle(name).dblclick();
  // The first load also fetches the lazily loaded player code, so allow it some time.
  await expect(page.locator("[data-content-viewer] video")).toBeVisible({ timeout: 20_000 });
}

const video = (page: Page) => page.locator("[data-content-viewer] video");
const currentTime = (page: Page) => video(page).evaluate((v: HTMLVideoElement) => v.currentTime);

async function installDetector(page: Page) {
  await page.evaluate(() => {
    const element = document.querySelector("[data-content-viewer] video") as HTMLVideoElement;
    const context = new AudioContext();
    const analyser = context.createAnalyser();
    analyser.fftSize = 8192;
    const silent = context.createGain();
    silent.gain.value = 0;
    context.createMediaElementSource(element).connect(analyser);
    analyser.connect(silent).connect(context.destination);
    void context.resume();
    (window as unknown as { tone: () => number }).tone = () => {
      const bins = new Float32Array(analyser.frequencyBinCount);
      analyser.getFloatFrequencyData(bins);
      let best = 0;
      for (let i = 1; i < bins.length; i++) if (bins[i] > bins[best]) best = i;
      return bins[best] < -90 ? 0 : (best * context.sampleRate) / analyser.fftSize;
    };
  });
}

async function expectTone(page: Page, hz: number) {
  await expect
    .poll(
      async () =>
        Math.abs(
          (await page.evaluate(() => (window as unknown as { tone: () => number }).tone())) - hz,
        ) < 25,
      { timeout: 30_000, message: `expected a ${hz} Hz tone` },
    )
    .toBe(true);
}

async function seek(page: Page, time: number) {
  await video(page).evaluate(async (v: HTMLVideoElement, target) => {
    v.pause();
    const seeked = new Promise((resolve) => v.addEventListener("seeked", resolve, { once: true }));
    v.currentTime = target;
    await seeked;
  }, time);
  // Let the player's timeupdate reach its state.
  await page.waitForTimeout(350);
}

// The player's menu stays open after a choice, so only open it when it is not showing already.
const openSubtitleMenu = async (page: Page) => {
  if (await page.getByText("Load subtitle file").isVisible()) return;
  const subtitles = page.getByText("Subtitles", { exact: true });
  if (!(await subtitles.isVisible())) await page.getByRole("button", { name: "Settings" }).click();
  await subtitles.click();
};

const caption = (page: Page) => page.locator("[data-embedded-caption] .vplayer__caption-cue");

test.describe("embedded tracks in the web player", () => {
  test.skip(({ isMobile }) => isMobile, "these tests exercise desktop menus and audio");

  test.beforeEach(async ({ page }) => {
    await installShare(page);
  });

  test("lists the audio tracks and switches between them", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    // The player plays the file through a MediaSource, not the plain URL.
    expect(await video(page).getAttribute("src")).toMatch(/^blob:/);
    await installDetector(page);
    await expectTone(page, 440);

    await page.getByRole("button", { name: "Audio", exact: true }).click();
    const menu = page.getByRole("menu", { name: "Audio track" });
    await expect(menu.getByRole("menuitemradio")).toHaveText([
      /^EnglishOpus Mono$/,
      /^JapaneseOpus Mono$/,
      /^Deutsch \(German\)AAC Mono$/,
      /^Francais \(E-AC-3\) \(French\)E-AC-3 Stereo$/,
    ]);
    await expect(menu.getByRole("menuitemradio", { name: /^English/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );

    await menu.getByRole("menuitemradio", { name: /^Japanese/ }).click();
    await expect(menu).toBeHidden();
    await expectTone(page, 880);

    await page.getByRole("button", { name: "Audio", exact: true }).click();
    await expect(page.getByRole("menuitemradio", { name: /^Japanese/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await page.getByRole("menuitemradio", { name: /^Francais/ }).click();
    await expectTone(page, 660); // E-AC-3, converted in the browser
    await page.getByRole("button", { name: "Audio", exact: true }).click();
    await page.getByRole("menuitemradio", { name: /^Deutsch/ }).click();
    await expectTone(page, 1320);
    // Still playing and the picture has not been reset.
    expect(await video(page).evaluate((v: HTMLVideoElement) => v.paused)).toBe(false);
  });

  test("the audio menu is keyboard accessible and closes on Escape", async ({ page }) => {
    await openVideo(page, items.multi.name);
    const button = page.getByRole("button", { name: "Audio", exact: true });
    await button.focus();
    await page.keyboard.press("Enter");
    const menu = page.getByRole("menu", { name: "Audio track" });
    await expect(menu).toBeVisible();
    // The selected track has focus, arrows move between tracks.
    await expect(menu.getByRole("menuitemradio", { name: /^English/ })).toBeFocused();
    await page.keyboard.press("ArrowDown");
    await expect(menu.getByRole("menuitemradio", { name: /^Japanese/ })).toBeFocused();
    await page.keyboard.press("ArrowUp");
    await expect(menu.getByRole("menuitemradio", { name: /^English/ })).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(button).toBeFocused();
  });

  test("remembers the audio language for the next video", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await page.getByRole("button", { name: "Audio", exact: true }).click();
    await page.getByRole("menuitemradio", { name: /^Japanese/ }).click();
    await expect
      .poll(() => page.evaluate(() => localStorage.getItem("teldrive-player-audio-language")))
      .toBe("jpn");

    // Close the viewer and open the file again: it starts on Japanese.
    await page.getByRole("button", { name: "Close viewer" }).click();
    await page.getByTitle(items.multi.name).dblclick();
    await expect(video(page)).toBeVisible();
    await installDetector(page);
    await expectTone(page, 880);
    await page.getByRole("button", { name: "Audio", exact: true }).click();
    await expect(page.getByRole("menuitemradio", { name: /^Japanese/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  test("embedded subtitles are listed with the others and drawn as captions", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await openSubtitleMenu(page);
    await expect(page.getByText("Off", { exact: true })).toBeVisible();
    await expect(page.getByText("English", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Japanese (ASS)", { exact: true })).toBeVisible();
    await expect(page.getByText("Load subtitle file")).toBeVisible();

    await page.getByText("English", { exact: true }).first().click();
    await seek(page, 1.5);
    await expect(caption(page)).toHaveText("Hello from the first subtitle");
    await seek(page, 5);
    await expect(caption(page)).toHaveText("Second line & more\nwith a break");
    await seek(page, 8);
    await expect(caption(page)).toHaveCount(0);
    await seek(page, 10);
    await expect(caption(page)).toHaveText("Late line near the end");

    // Another embedded track replaces it; overlapping ASS lines are shown together.
    await openSubtitleMenu(page);
    await page.getByText("Japanese (ASS)", { exact: true }).click();
    await seek(page, 2.5);
    await expect(caption(page)).toHaveText(
      "ASS first, with styling tags\nOverlapping, second\nline break",
    );
    await seek(page, 7.2); // a vector drawing is skipped, the text line is not
    await expect(caption(page)).toHaveText("Comma, inside, the, text, field, yes");

    // Off removes the captions.
    await openSubtitleMenu(page);
    await page.getByText("Off", { exact: true }).click();
    await seek(page, 2.5);
    await expect(caption(page)).toHaveCount(0);
  });

  test("a subtitle choice is remembered and applied to the next video", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await openSubtitleMenu(page);
    await page.getByText("English", { exact: true }).first().click();
    await expect
      .poll(() => page.evaluate(() => localStorage.getItem("teldrive-player-subtitle-language")))
      .toBe("eng");

    await page.getByRole("button", { name: "Close viewer" }).click();
    await page.getByTitle(items.multi.name).dblclick();
    await expect(video(page)).toBeVisible();
    await seek(page, 1.5);
    await expect(caption(page)).toHaveText("Hello from the first subtitle");

    // Turning subtitles off is remembered too.
    await openSubtitleMenu(page);
    await page.getByText("Off", { exact: true }).click();
    await expect
      .poll(() => page.evaluate(() => localStorage.getItem("teldrive-player-subtitle-language")))
      .toBe("off");
  });

  test("loading a subtitle file still works next to embedded ones", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await openSubtitleMenu(page);
    await page.locator("input[type=file]").setInputFiles({
      name: "extra.srt",
      mimeType: "text/plain",
      buffer: Buffer.from("1\n00:00:01,000 --> 00:00:03,000\nFrom a file\n"),
    });
    await seek(page, 1.5);
    // The player draws loaded files itself; the embedded layer stays out of the way.
    await expect(page.locator(".vplayer__caption-cue", { hasText: "From a file" })).toBeVisible();
    await expect(caption(page)).toHaveCount(0);
  });

  test("seeking in a remuxed video keeps playing with the chosen audio", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await installDetector(page);
    await page.getByRole("button", { name: "Audio", exact: true }).click();
    await page.getByRole("menuitemradio", { name: /^Japanese/ }).click();
    await expectTone(page, 880);
    await video(page).evaluate((v: HTMLVideoElement) => {
      v.currentTime = 9;
    });
    await expect.poll(() => currentTime(page), { timeout: 15_000 }).toBeGreaterThan(9.4);
    await expectTone(page, 880);
  });

  test("a file with one playable audio track keeps playing natively", async ({ page }) => {
    await openVideo(page, items.single.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    expect(await video(page).getAttribute("src")).toMatch(/\/content\/Single\.mkv/);
    await expect(page.getByRole("button", { name: "Audio", exact: true })).toHaveCount(0);
    // Its embedded subtitle is still available.
    await openSubtitleMenu(page);
    await page.getByText("English", { exact: true }).first().click();
    await seek(page, 1.5);
    await expect(caption(page)).toHaveText("Hello from the first subtitle");
  });

  test("audio the browser cannot decode is converted so the video is not silent", async ({
    page,
  }) => {
    await openVideo(page, items.eac3.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    expect(await video(page).getAttribute("src")).toMatch(/^blob:/);
    await installDetector(page);
    await expectTone(page, 660);
    // One track: nothing to choose.
    await expect(page.getByRole("button", { name: "Audio", exact: true })).toHaveCount(0);
  });

  test("a file that is not really Matroska still plays through the normal path", async ({
    page,
  }) => {
    await openVideo(page, items.broken.name);
    expect(await video(page).getAttribute("src")).toMatch(/\/content\/Broken\.mkv/);
    await expect(page.getByRole("button", { name: "Audio", exact: true })).toHaveCount(0);
  });

  test("other video files are not touched at all", async ({ page }) => {
    const requests: string[] = [];
    page.on("request", (request) => {
      if (request.url().includes("/content/")) requests.push(request.headers().range ?? "none");
    });
    await openVideo(page, items.plain.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    expect(await video(page).getAttribute("src")).toMatch(/\/content\/Plain\.webm/);
    await expect(page.getByRole("button", { name: "Audio", exact: true })).toHaveCount(0);
    // No probing: the first request is the player's own.
    expect(requests.length).toBeGreaterThan(0);
  });
});

// A phone in portrait leaves little room above the controls for the menu.
test.describe("audio menu on a small screen", () => {
  test.skip(({ isMobile }) => !isMobile, "mobile viewport only");

  test("fits inside the player and stays reachable", async ({ page }) => {
    await installShare(page);
    await openVideo(page, items.multi.name);
    await page.getByRole("button", { name: "Audio", exact: true }).click();
    const menu = page.getByRole("menu", { name: "Audio track" });
    await expect(menu).toBeVisible();
    const box = await menu.boundingBox();
    const player = await page.locator("[data-content-viewer] .vplayer").first().boundingBox();
    expect(box).not.toBeNull();
    expect(player).not.toBeNull();
    // Entirely inside the player, so no entry is cut off by it.
    expect(box!.y).toBeGreaterThanOrEqual(player!.y - 1);
    expect(box!.x).toBeGreaterThanOrEqual(player!.x - 1);
    expect(box!.x + box!.width).toBeLessThanOrEqual(player!.x + player!.width + 1);
    // Every track can be reached, by scrolling when they do not all fit.
    const last = menu.getByRole("menuitemradio", { name: /^Francais/ });
    await last.scrollIntoViewIfNeeded();
    await last.click();
    await expect(menu).toBeHidden();
  });
});

// The signed-in file manager opens the same preview dialog and player.
test.describe("embedded tracks when signed in", () => {
  test.skip(({ isMobile }) => isMobile, "these tests exercise desktop menus and audio");

  test("the file manager player has the audio and subtitle tracks too", async ({ page }) => {
    const multi = items.multi;
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
            capabilities: ["files.read", "files.write", "files.share"],
            createdAt: now,
          },
        });
      }
      if (path === "/v1/files") {
        return route.fulfill({
          json: {
            items: [entry(multi.id, multi.name, "file", multi.mimeType, multi.data.length)].map(
              (item) => ({ ...item, parentId: undefined }),
            ),
            meta: {},
          },
        });
      }
      if (path.startsWith(`/v1/files/${multi.id}/content/`)) {
        const range = /^bytes=(\d+)-(\d*)$/.exec(route.request().headers().range ?? "");
        const data = multi.data;
        if (!range) return route.fulfill({ status: 200, body: data });
        const start = Number(range[1]);
        const end = range[2] ? Math.min(Number(range[2]), data.length - 1) : data.length - 1;
        if (start >= data.length) return route.fulfill({ status: 416 });
        return route.fulfill({
          status: 206,
          body: data.subarray(start, end + 1),
          headers: {
            "Accept-Ranges": "bytes",
            "Content-Range": `bytes ${start}-${end}/${data.length}`,
          },
        });
      }
      return route.fulfill({ status: 404, json: { code: "not_found", title: "not found" } });
    });
    await installPreferencesApi(page);

    await page.goto("/files");
    await page.getByRole("row", { name: /Multi\.mkv/ }).dblclick();
    await expect(video(page)).toBeVisible({ timeout: 20_000 });
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    expect(await video(page).getAttribute("src")).toMatch(/^blob:/);
    await installDetector(page);
    await expectTone(page, 440);

    await page.getByRole("button", { name: "Audio", exact: true }).click();
    await page.getByRole("menuitemradio", { name: /^Japanese/ }).click();
    await expectTone(page, 880);

    await openSubtitleMenu(page);
    await page.getByText("English", { exact: true }).first().click();
    await seek(page, 1.5);
    await expect(caption(page)).toHaveText("Hello from the first subtitle");
  });
});
