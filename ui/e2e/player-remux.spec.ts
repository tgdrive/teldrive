import { readFileSync } from "node:fs";
import { expect, type Page, test } from "@playwright/test";

const fixture = readFileSync(new URL("./fixtures/player/multi-track.mkv", import.meta.url));
const MEDIA_URL = "/fixture/multi-track.mkv";

// Serves the fixture with real Range support, like the file content endpoint does.
async function serveFixture(page: Page) {
  const requests: string[] = [];
  await page.route("**/fixture/multi-track.mkv", async (route) => {
    const range = /^bytes=(\d+)-(\d*)$/.exec(route.request().headers().range ?? "");
    requests.push(range ? `${range[1]}-${range[2]}` : "full");
    if (!range) {
      return route.fulfill({ status: 200, body: fixture, headers: { "Accept-Ranges": "bytes" } });
    }
    const start = Number(range[1]);
    const end = range[2] ? Math.min(Number(range[2]), fixture.length - 1) : fixture.length - 1;
    if (start >= fixture.length) return route.fulfill({ status: 416 });
    return route.fulfill({
      status: 206,
      body: fixture.subarray(start, end + 1),
      headers: {
        "Accept-Ranges": "bytes",
        "Content-Range": `bytes ${start}-${end}/${fixture.length}`,
      },
    });
  });
  return requests;
}

// A bare page on the app's origin, so Vite serves the modules under test.
async function openHarness(page: Page) {
  await page.route("**/__harness", (route) =>
    route.fulfill({ contentType: "text/html", body: "<!doctype html><video id=v></video>" }),
  );
  await page.goto("/__harness");
  // A click is the user gesture that lets the page play audio.
  await page.click("body");
}

// Installs a tone detector on the <video>: returns the dominant frequency of what it plays.
async function installDetector(page: Page) {
  await page.evaluate(() => {
    const video = document.getElementById("v") as HTMLVideoElement;
    const context = new AudioContext();
    const analyser = context.createAnalyser();
    analyser.fftSize = 8192;
    const silent = context.createGain();
    silent.gain.value = 0;
    context.createMediaElementSource(video).connect(analyser);
    analyser.connect(silent).connect(context.destination);
    void context.resume();
    (window as unknown as { dominantFrequency: () => number }).dominantFrequency = () => {
      const bins = new Float32Array(analyser.frequencyBinCount);
      analyser.getFloatFrequencyData(bins);
      let best = 0;
      for (let i = 1; i < bins.length; i++) if (bins[i] > bins[best]) best = i;
      // Silence reads as a flat floor; report it as 0 Hz rather than a random bin.
      return bins[best] < -90 ? 0 : (best * context.sampleRate) / analyser.fftSize;
    };
  });
}

async function startSession(page: Page, audioTrackId?: number) {
  await page.evaluate(
    async ({ url, audioTrackId }) => {
      const { RemuxSession } = await import("/src/features/player/remux-session.ts");
      const session = await RemuxSession.open(location.origin + url, {
        pickAudio: audioTrackId === undefined ? undefined : () => audioTrackId,
      });
      const video = document.getElementById("v") as HTMLVideoElement;
      (window as unknown as Record<string, unknown>).session = session;
      session.attach(video);
      video.src = session.objectUrl;
      await video.play();
    },
    { url: MEDIA_URL, audioTrackId },
  );
}

type SessionWindow = {
  session: {
    audioTracks: { id: number; codec: string; mode: string; language?: string; name?: string }[];
    activeAudio: number;
    selectAudio: (id: number) => Promise<void>;
    duration: number;
  };
  dominantFrequency: () => number;
};

const dominant = (page: Page) =>
  page.evaluate(() => (window as unknown as SessionWindow).dominantFrequency());

// The detector's resolution is 48000 / 8192 = 5.9 Hz, so allow a few bins either side.
async function expectTone(page: Page, hz: number) {
  await expect
    .poll(async () => Math.abs((await dominant(page)) - hz) < 25, {
      timeout: 30_000,
      message: `expected a ${hz} Hz tone`,
    })
    .toBe(true);
}

test.describe("audio track remuxing", () => {
  test.skip(({ isMobile }) => isMobile, "engine behaviour does not depend on the viewport");

  test("lists the tracks of a file and plays the default one", async ({ page }) => {
    await serveFixture(page);
    await openHarness(page);
    await installDetector(page);
    await startSession(page);

    const tracks = await page.evaluate(
      () => (window as unknown as SessionWindow).session.audioTracks,
    );
    expect(tracks.map((t) => [t.id, t.codec, t.mode, t.language, t.name])).toEqual([
      [2, "opus", "direct", "eng", "English"],
      [3, "opus", "direct", "jpn", "Japanese"],
      [4, "aac", "direct", "ger", "Deutsch"],
      [5, "eac3", "transcode", "fre", "Francais (E-AC-3)"],
    ]);
    expect(
      await page.evaluate(() => (window as unknown as SessionWindow).session.activeAudio),
    ).toBe(2);
    await expectTone(page, 440);
    // Video really plays: the clock advances and frames are decoded.
    await expect
      .poll(() =>
        page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).currentTime),
      )
      .toBeGreaterThan(1);
    expect(
      await page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).videoWidth),
    ).toBe(96);
  });

  test("switches audio tracks during playback, including a transcoded one", async ({ page }) => {
    await serveFixture(page);
    await openHarness(page);
    await installDetector(page);
    await startSession(page);
    await expectTone(page, 440);

    for (const [id, hz] of [
      [3, 880], // Opus -> Opus: same codec, no buffer type change
      [4, 1320], // AAC: the audio buffer changes type
      [5, 660], // E-AC-3: decoded by the WebAssembly decoder and re-encoded as Opus
      [2, 440],
    ] as const) {
      const before = await page.evaluate(
        () => (document.getElementById("v") as HTMLVideoElement).currentTime,
      );
      await page.evaluate(
        (trackId) => (window as unknown as SessionWindow).session.selectAudio(trackId),
        id,
      );
      await expectTone(page, hz);
      const after = await page.evaluate(() => {
        const video = document.getElementById("v") as HTMLVideoElement;
        return { time: video.currentTime, paused: video.paused, error: video.error?.message };
      });
      // Playback carried on from (about) where it was, with the picture untouched.
      expect(after.error).toBeUndefined();
      expect(after.paused).toBe(false);
      expect(after.time).toBeGreaterThanOrEqual(before);
    }
  });

  test("switches away from a converted track and back, in quick succession", async ({ page }) => {
    await serveFixture(page);
    await openHarness(page);
    await installDetector(page);
    await startSession(page);
    await expectTone(page, 440);
    for (const [id, hz] of [
      [5, 660],
      [4, 1320],
      [5, 660],
      [3, 880],
      [4, 1320],
    ] as const) {
      await page.evaluate(
        (trackId) => (window as unknown as SessionWindow).session.selectAudio(trackId),
        id,
      );
      await expectTone(page, hz);
    }
  });

  test("switching audio while paused stays paused, and the new track plays on resume", async ({
    page,
  }) => {
    await serveFixture(page);
    await openHarness(page);
    await installDetector(page);
    await startSession(page);
    await expectTone(page, 440);
    await page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).pause());
    const pausedAt = await page.evaluate(
      () => (document.getElementById("v") as HTMLVideoElement).currentTime,
    );

    await page.evaluate(() => (window as unknown as SessionWindow).session.selectAudio(3));
    expect(
      await page.evaluate(() => (window as unknown as SessionWindow).session.activeAudio),
    ).toBe(3);
    const state = await page.evaluate(() => {
      const video = document.getElementById("v") as HTMLVideoElement;
      return { paused: video.paused, time: video.currentTime };
    });
    expect(state.paused).toBe(true);
    expect(state.time).toBeCloseTo(pausedAt, 1);

    await page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).play());
    await expectTone(page, 880);
  });

  test("seeks forwards and backwards and keeps the chosen track", async ({ page }) => {
    await serveFixture(page);
    await openHarness(page);
    await installDetector(page);
    await startSession(page);
    await page.evaluate(() => (window as unknown as SessionWindow).session.selectAudio(3));
    await expectTone(page, 880);

    // Forward, outside what has been buffered so far.
    await page.evaluate(() => {
      (document.getElementById("v") as HTMLVideoElement).currentTime = 9;
    });
    await expect
      .poll(
        () => page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).currentTime),
        {
          timeout: 30_000,
        },
      )
      .toBeGreaterThan(9.5);
    await expectTone(page, 880);

    // And back to the start.
    await page.evaluate(() => {
      (document.getElementById("v") as HTMLVideoElement).currentTime = 0.5;
    });
    await expect
      .poll(
        () => page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).currentTime),
        {
          timeout: 30_000,
        },
      )
      .toBeGreaterThan(1.5);
    await expectTone(page, 880);
  });

  test("plays through to the end", async ({ page }) => {
    await serveFixture(page);
    await openHarness(page);
    await startSession(page, 4);
    await page.evaluate(() => {
      (document.getElementById("v") as HTMLVideoElement).currentTime = 9.5;
    });
    await expect
      .poll(() => page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).ended), {
        timeout: 20_000,
      })
      .toBe(true);
  });

  test("only reads the parts of the file it needs", async ({ page }) => {
    const requests = await serveFixture(page);
    await openHarness(page);
    await startSession(page);
    await expect
      .poll(() =>
        page.evaluate(() => (document.getElementById("v") as HTMLVideoElement).currentTime),
      )
      .toBeGreaterThan(0.5);
    // Everything arrives as range requests; nothing downloads the file as a whole.
    expect(requests.length).toBeGreaterThan(0);
    expect(requests).not.toContain("full");
  });
});
