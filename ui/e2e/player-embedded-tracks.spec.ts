import { readFileSync } from "node:fs";
import { expect, type Page, type Route, test } from "@playwright/test";
import { installPreferencesApi } from "./fixtures/preferences";

// The web player (vplayer) can read the audio tracks and subtitles stored in .mkv/.webm files, but
// only when that is turned on in Settings → Player: "Read tracks from the file" for the signed-in
// file manager and "Also on shared links" for a public share, whose visitors have no settings of their
// own. Both are saved on the account. These tests drive the real player on a public share page, which opens
// the same preview dialog and VideoViewer as the signed-in file manager.

const now = "2026-10-21T12:00:00Z";
const token = "tds_trackToken123";
const folderId = "a1111111-1111-4111-8111-111111111111";
const LEGACY_SETTING = "teldrive-player-embedded-tracks";

const bytes = (name: string) => readFileSync(new URL(`./fixtures/player/${name}`, import.meta.url));

type Item = { id: string; name: string; mimeType: string; data: Buffer };
const items = {
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
} satisfies Record<string, Item>;

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

/** Byte ranges the browser asked for, per file, to tell whether the player looked inside a file. */
type Requests = Map<string, string[]>;

/** `embeddedTracks` is the share owner's choice, as the public share API reports it. */
async function installShare(
  page: Page,
  options: { embeddedTracks?: boolean } = {},
): Promise<Requests> {
  const requests: Requests = new Map();
  const prefix = `/api/v1/public/shares/${token}`;
  const content = async (route: Route, item: Item) => {
    const header = route.request().headers().range ?? "";
    requests.set(item.name, [...(requests.get(item.name) ?? []), header]);
    const range = /^bytes=(\d+)-(\d*)$/.exec(header);
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
            embeddedTracks: options.embeddedTracks ?? false,
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
  return requests;
}

async function openVideo(page: Page, name: string) {
  await page.goto(`/share/${token}`);
  await page.getByTitle(name).dblclick();
  // The first load also fetches the lazily loaded player code, so allow it some time.
  await expect(page.locator("[data-content-viewer] video")).toBeVisible({ timeout: 20_000 });
}

const video = (page: Page) => page.locator("[data-content-viewer] video");
const currentTime = (page: Page) => video(page).evaluate((v: HTMLVideoElement) => v.currentTime);
const openSettingsMenu = (page: Page) => page.getByRole("button", { name: "Settings" }).click();

// Pauses at a time in the clip, so the menu is used while the video is still there.
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

/** The signed-in account the settings pages need. */
async function installAccount(page: Page) {
  await page.route(
    (url) => url.pathname === "/api/v1/me",
    (route) =>
      route.fulfill({
        json: {
          userId: 1,
          displayName: "Settings User",
          username: "settingsuser",
          premium: false,
          role: "owner",
          capabilities: [],
          createdAt: "2026-04-20T08:00:00Z",
        },
      }),
  );
}

test.describe("embedded tracks setting", () => {
  test.skip(({ isMobile }) => isMobile, "these tests exercise desktop menus and audio");

  test("is off by default: the player does not look inside the file", async ({ page }) => {
    const requests = await installShare(page);
    // Every script the page loads, to prove the optional code stays out of it.
    const scripts: string[] = [];
    page.on("request", (request) => {
      if (request.resourceType() === "script") scripts.push(request.url());
    });

    await openVideo(page, items.multi.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.2);
    // Plain URL, no MediaSource.
    expect(await video(page).getAttribute("src")).not.toMatch(/^blob:/);

    await openSettingsMenu(page);
    await expect(page.getByText("Speed", { exact: true })).toBeVisible();
    await expect(page.getByText("Audio", { exact: true })).toHaveCount(0);
    await page.getByText("Subtitles", { exact: true }).click();
    await expect(page.getByText("Load subtitle file")).toBeVisible();
    await expect(page.getByText("English", { exact: true })).toHaveCount(0);

    // Nothing from the track reader was fetched.
    expect(scripts.filter((url) => /embedded-tracks|mediabunny|ac3/i.test(url))).toEqual([]);
    expect(requests.get(items.multi.name)?.length).toBeGreaterThan(0);
  });

  test("the account setting alone does not turn it on for a share", async ({ page }) => {
    // A share page shows what its owner allows on shared links, even to the owner when signed in.
    await installShare(page, { embeddedTracks: false });
    await installPreferencesApi(page, { appearance: { embeddedTracks: true } });

    await openVideo(page, items.multi.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.2);
    expect(await video(page).getAttribute("src")).not.toMatch(/^blob:/);
    await openSettingsMenu(page);
    await expect(page.getByText("Audio", { exact: true })).toHaveCount(0);
  });

  test("can be turned on in Settings → Player, saved on the account", async ({ page }) => {
    await installShare(page);
    await installAccount(page);
    const preferences = await installPreferencesApi(page);
    await page.goto("/settings/player");
    const toggle = page.getByRole("switch", { name: "Read tracks from the file" });
    // The input itself is visually hidden; people click the switch and its label.
    const label = page.getByText("Read tracks from the file", { exact: true });
    await expect(toggle).toBeVisible();
    await expect(toggle).not.toBeChecked();
    expect(preferences.patches).toEqual([]);

    await label.click();
    await expect(toggle).toBeChecked();
    expect(preferences.patches.at(-1)).toEqual({ appearance: { embeddedTracks: true } });

    // On the account, so it is still on after a reload (and in any other browser).
    await page.reload();
    await expect(page.getByRole("switch", { name: "Read tracks from the file" })).toBeChecked();

    await page.getByText("Read tracks from the file", { exact: true }).click();
    await expect(page.getByRole("switch", { name: "Read tracks from the file" })).not.toBeChecked();
    expect(preferences.patches.at(-1)).toEqual({ appearance: { embeddedTracks: false } });
  });

  test("old browser embedded-track settings do not repopulate account preferences", async ({
    page,
  }) => {
    await installShare(page);
    await installAccount(page);
    const preferences = await installPreferencesApi(page);
    // Older builds kept the setting in this browser only; set it before the app first loads.
    await page.addInitScript((key) => {
      if (!window.sessionStorage.getItem("seeded")) {
        window.localStorage.setItem(key, "true");
        window.sessionStorage.setItem("seeded", "1");
      }
    }, LEGACY_SETTING);
    await page.goto("/settings/player");
    await expect(page.getByRole("switch", { name: "Read tracks from the file" })).not.toBeChecked();
    await page.reload();
    await expect(page.getByRole("switch", { name: "Read tracks from the file" })).not.toBeChecked();
    expect(preferences.patches).toEqual([]);
  });

  test("'Also on shared links' needs the first option and is saved on the account", async ({
    page,
  }) => {
    await installShare(page);
    await installAccount(page);
    // Registered last, so it wins over the one installShare added and records what is saved.
    const preferences = await installPreferencesApi(page);
    await page.goto("/settings/player");
    const tracks = page.getByRole("switch", { name: "Read tracks from the file" });
    const shares = page.getByRole("switch", { name: "Also on shared links" });
    await expect(shares).toBeVisible();
    await expect(shares).toBeDisabled();
    await expect(shares).not.toBeChecked();

    await page.getByText("Read tracks from the file", { exact: true }).click();
    await expect(tracks).toBeChecked();
    await expect(shares).toBeEnabled();

    await page.locator('[data-slot="label"]', { hasText: "Also on shared links" }).click();
    await expect(shares).toBeChecked();
    expect(preferences.patches.at(-1)).toEqual({ appearance: { embeddedTracksOnShares: true } });
    expect(preferences.current().appearance.embeddedTracksOnShares).toBe(true);

    // Saved on the account, so it is still there after a reload.
    await page.reload();
    await expect(shares).toBeChecked();

    // Turning the first option off turns this one off with it (the server enforces that).
    await page.getByText("Read tracks from the file", { exact: true }).click();
    await expect(tracks).not.toBeChecked();
    await expect(shares).not.toBeChecked();
    await expect(shares).toBeDisabled();
    expect(preferences.patches.at(-1)).toEqual({ appearance: { embeddedTracks: false } });
    expect(preferences.current().appearance.embeddedTracksOnShares).toBe(false);
  });
});

test.describe("embedded tracks in the web player (the owner allows it on the share)", () => {
  test.skip(({ isMobile }) => isMobile, "these tests exercise desktop menus and audio");

  test.beforeEach(async ({ page }) => {
    // The visitor has no setting at all: the share itself says the owner allows it.
    await installShare(page, { embeddedTracks: true });
  });

  test("lists the audio tracks and switches between them", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    // With several audio tracks the file plays through a MediaSource.
    expect(await video(page).getAttribute("src")).toMatch(/^blob:/);
    await installDetector(page);
    await expectTone(page, 440);

    await openSettingsMenu(page);
    await page.getByText("Audio", { exact: true }).click();
    await expect(page.getByText("Japanese", { exact: true })).toBeVisible();
    await expect(page.getByText("Deutsch (German)", { exact: true })).toBeVisible();

    await page.getByText("Japanese", { exact: true }).click();
    await expectTone(page, 880);

    await openSettingsMenu(page);
    await page.getByText("Audio", { exact: true }).click();
    await page.getByText("Deutsch (German)", { exact: true }).click();
    await expectTone(page, 1320);
  });

  test("plays an E-AC-3 track by converting it", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    await installDetector(page);

    await openSettingsMenu(page);
    await page.getByText("Audio", { exact: true }).click();
    await page.getByText(/Francais/).click();
    await expectTone(page, 660);
  });

  test("shows embedded subtitles, SRT and ASS, as the video plays", async ({ page }) => {
    await openVideo(page, items.multi.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);

    await seek(page, 1.8);

    await openSettingsMenu(page);
    await page.getByText("Subtitles", { exact: true }).click();
    await page.getByText("English", { exact: true }).click();
    await expect(page.getByText("Hello from the first subtitle")).toBeVisible({ timeout: 15_000 });

    // Choosing a subtitle leaves the menu open on its list.
    await expect(page.getByText("Load subtitle file")).toBeVisible();
    await page.getByText(/Japanese \(ASS\)/).click();
    await expect(page.getByText("ASS first, with styling tags").first()).toBeVisible({
      timeout: 15_000,
    });
  });

  test("a file with one playable track plays directly and still lists its subtitles", async ({
    page,
  }) => {
    await openVideo(page, items.single.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    expect(await video(page).getAttribute("src")).not.toMatch(/^blob:/);

    await openSettingsMenu(page);
    await expect(page.getByText("Audio", { exact: true })).toHaveCount(0);
    await page.getByText("Subtitles", { exact: true }).click();
    await expect(page.getByText("English", { exact: true })).toBeVisible();
  });

  test("a single E-AC-3 track is converted so it is audible", async ({ page }) => {
    await openVideo(page, items.eac3.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.3);
    await installDetector(page);
    await expectTone(page, 660);
  });

  test("a file that is not really Matroska still plays", async ({ page }) => {
    await openVideo(page, items.broken.name);
    // Nothing to play in this file, but the player must stay usable and not throw the page away.
    await expect(page.locator("[data-content-viewer]")).toBeVisible();
    await expect(page.getByRole("button", { name: "Settings" })).toBeVisible();
  });

  test("other formats are untouched", async ({ page }) => {
    await openVideo(page, items.plain.name);
    await expect.poll(() => currentTime(page)).toBeGreaterThan(0.1);
    expect(await video(page).getAttribute("src")).not.toMatch(/^blob:/);
    await openSettingsMenu(page);
    await expect(page.getByText("Audio", { exact: true })).toHaveCount(0);
  });
});
