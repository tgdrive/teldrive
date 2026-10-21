import { expect, test } from "@playwright/test";
import { EmbeddedTracks } from "../src/features/player/embedded-tracks";
import { fixture, round, serve } from "./fixtures/player/helpers";

// The track controller against real files served over HTTP range requests, without a browser.

test.describe("embedded subtitles controller", () => {
  const settle = async (tracks: EmbeddedTracks, status: "ready" | "error" = "ready") => {
    await expect.poll(() => tracks.getSnapshot().subtitleStatus, { timeout: 10_000 }).toBe(status);
  };

  test("lists subtitles and plays the file directly when audio is playable as is", async () => {
    const server = await serve(fixture("single-track.mkv"));
    try {
      const tracks = new EmbeddedTracks(server.url, "en", (n) => `Track ${n}`);
      const source = await tracks.load(new AbortController().signal);
      // One Opus track: nothing to choose, no remuxing.
      expect(source).toEqual({ src: server.url });
      expect(tracks.getSnapshot().audio).toEqual([]);
      expect(tracks.getSnapshot().subtitles).toMatchObject([
        { id: "embedded:3", number: 3, label: "English", language: "eng", supported: true },
      ]);
      tracks.dispose();
    } finally {
      await server.close();
    }
  });

  test("falls back to the plain file for data it cannot read", async () => {
    const server = await serve(Buffer.from("garbage"));
    try {
      const tracks = new EmbeddedTracks(server.url, "en", (n) => `Track ${n}`);
      expect(await tracks.load(new AbortController().signal)).toEqual({ src: server.url });
      tracks.dispose();
    } finally {
      await server.close();
    }
  });

  test("reads only the part of the subtitles ahead of the playhead", async () => {
    const data = fixture("long-subs.mkv");
    const server = await serve(data);
    try {
      const tracks = new EmbeddedTracks(server.url, "en", (n) => `Track ${n}`);
      await tracks.load(new AbortController().signal);
      const headerBytes = server.bytesRequested();

      tracks.selectSubtitle(2);
      tracks.setPlayhead(0);
      await settle(tracks);
      const first = tracks.getSnapshot().cues.map((cue) => cue.text);
      // Cues for the next minute or so are there, the far end of the file is not.
      expect(first).toContain("Cue 0");
      expect(first).toContain("Cue 5");
      expect(first).not.toContain("Cue 50");
      expect(server.bytesRequested() - headerBytes).toBeLessThan(data.length);

      // A seek far ahead reads there and shows those cues.
      tracks.setPlayhead(400);
      await expect
        .poll(() => tracks.getSnapshot().cues.some((cue) => cue.text === "Cue 40"), {
          timeout: 10_000,
        })
        .toBe(true);
      await settle(tracks);

      // Going back to a region that has been read costs no new requests.
      const requestsBefore = server.requests.length;
      tracks.setPlayhead(2);
      await settle(tracks);
      await new Promise((resolve) => setTimeout(resolve, 400));
      expect(server.requests.length).toBe(requestsBefore);
      expect(tracks.getSnapshot().cues.map((cue) => cue.text)).toContain("Cue 0");

      // Deselecting stops reading and clears the cues.
      tracks.selectSubtitle(undefined);
      expect(tracks.getSnapshot().cues).toEqual([]);
      tracks.dispose();
    } finally {
      await server.close();
    }
  });

  test("falls back to the plain file and resumes where playback was", async () => {
    const server = await serve(fixture("single-track.mkv"));
    try {
      const tracks = new EmbeddedTracks(server.url, "en", (n) => `Track ${n}`);
      await tracks.load(new AbortController().signal);
      // Stand in for a remuxed session that has been playing for 42 s and then breaks.
      let disposed = false;
      const internals = tracks as unknown as Record<string, unknown>;
      internals.session = { dispose: () => (disposed = true), attach: () => undefined };
      tracks.attachVideo({ currentTime: 42 } as HTMLVideoElement);
      const fallbacks: unknown[] = [];
      tracks.onFallback((source) => fallbacks.push(source));

      (internals.fallBack as () => void).call(tracks);

      expect(fallbacks).toEqual([{ src: server.url }]);
      expect(disposed).toBe(true);
      expect(tracks.getSnapshot().audio).toEqual([]);
      expect(tracks.takeResumeTime()).toBe(42);
      expect(tracks.takeResumeTime()).toBeUndefined(); // only once
      tracks.dispose();
    } finally {
      await server.close();
    }
  });

  test("subtitles from the last file are the same cues for a second selection", async () => {
    const server = await serve(fixture("multi-track.mkv"));
    try {
      const tracks = new EmbeddedTracks(server.url, "en", (n) => `Track ${n}`);
      await tracks.load(new AbortController().signal);
      expect(tracks.getSnapshot().subtitles.map((s) => [s.number, s.label, s.supported])).toEqual([
        [6, "English", true],
        [7, "Japanese (ASS)", true],
      ]);
      tracks.selectSubtitle(7);
      tracks.setPlayhead(0);
      await settle(tracks);
      expect(round(tracks.getSnapshot().cues).map((cue) => cue.text)).toEqual([
        "ASS first, with styling tags",
        "ASS first, with styling tags\nOverlapping, second\nline break",
        "Overlapping, second\nline break",
        "Comma, inside, the, text, field, yes",
      ]);
      // Switching tracks gives that track's cues.
      tracks.selectSubtitle(6);
      await settle(tracks);
      expect(tracks.getSnapshot().cues.map((cue) => cue.text)).toContain(
        "Hello from the first subtitle",
      );
      tracks.dispose();
    } finally {
      await server.close();
    }
  });
});
