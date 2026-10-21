// Plays a video file through Media Source Extensions so the viewer can pick the audio track.
//
// A <video> element plays the first audio track of a file and nothing else, and cannot decode some
// codecs (E-AC-3 / AC-3 on most Linux builds). This session reads the file with Mediabunny,
// repackages the video and one chosen audio track as fragmented MP4, and appends it to a
// MediaSource. The browser then plays it as usual (seeking, speed, picture-in-picture, volume),
// and switching audio only replaces the audio buffer: the video is not touched.
//
//   video  : packets are copied unchanged (no re-encode).
//   audio  : copied when the browser can play the codec; otherwise decoded and re-encoded as Opus.
//
// Only the part of the file around the playhead is read and buffered.

import {
  ALL_FORMATS,
  AppendOnlyStreamTarget,
  AudioSampleSink,
  AudioSampleSource,
  canDecodeAudio,
  canEncodeAudio,
  EncodedAudioPacketSource,
  EncodedPacketSink,
  EncodedVideoPacketSource,
  Input,
  type InputAudioTrack,
  type InputVideoTrack,
  Mp4OutputFormat,
  Output,
  UrlSource,
} from "mediabunny";

export type AudioMode = "direct" | "transcode" | "unsupported";

export type AudioTrackInfo = {
  /** Container track id (the Matroska track number for .mkv files). */
  id: number;
  codec: string;
  language?: string;
  name?: string;
  channels: number;
  sampleRate: number;
  isDefault: boolean;
  mode: AudioMode;
};

export type RemuxSessionOptions = {
  /**
   * Picks the audio track to start with from the file's tracks. Without it, or when it returns
   * nothing playable, the default track is used, else the first one that can be played.
   */
  pickAudio?: (tracks: AudioTrackInfo[]) => number | undefined;
  signal?: AbortSignal;
};

/** How far ahead of the playhead to keep data buffered. */
const AHEAD_MAX = 45;
/** Data further behind the playhead than this is dropped. */
const BEHIND_KEEP = 30;
const SEEK_DEBOUNCE_MS = 120;
const READ_AHEAD_BYTES = 16 * 1024 * 1024;

class StaleError extends Error {
  constructor() {
    super("stale");
  }
}

// The AC-3 / E-AC-3 decoder is a sizeable WebAssembly bundle, so it is only fetched when a track
// of that kind is actually decoded.
let ac3Registered: Promise<void> | undefined;
function ensureAc3Decoder(): Promise<void> {
  ac3Registered ??= import("@mediabunny/ac3").then(({ registerAc3Decoder }) =>
    registerAc3Decoder(),
  );
  return ac3Registered;
}

function supportsMse(): boolean {
  return typeof MediaSource !== "undefined" && typeof MediaSource.isTypeSupported === "function";
}

function mimeFor(kind: "video" | "audio", codec: string) {
  return `${kind}/mp4; codecs="${codec}"`;
}

async function audioModeFor(
  codec: string | null,
  config: AudioDecoderConfig | null,
): Promise<AudioMode> {
  if (!codec || !config) return "unsupported";
  const output = new Mp4OutputFormat({ fastStart: "fragmented" });
  const copyable = output.getSupportedAudioCodecs().includes(codec as never);
  if (copyable && supportsMse() && MediaSource.isTypeSupported(mimeFor("audio", config.codec))) {
    return "direct";
  }
  // AC-3 and E-AC-3 are decoded by the bundled WebAssembly decoder; other codecs need WebCodecs.
  const decodable = codec === "ac3" || codec === "eac3" || (await canDecodeAudio(codec as never));
  if (decodable && (await canEncodeAudio("opus"))) return "transcode";
  return "unsupported";
}

/** Splits an MP4 byte stream into whole top-level boxes and drops the ones MSE does not need. */
class BoxSplitter {
  private buffer: Uint8Array = new Uint8Array(0);

  /** Returns groups of whole boxes, each ending at a media or init boundary. */
  push(chunk: Uint8Array): Uint8Array[] {
    const joined = new Uint8Array(this.buffer.length + chunk.length);
    joined.set(this.buffer);
    joined.set(chunk, this.buffer.length);
    this.buffer = joined;

    const groups: Uint8Array[] = [];
    let pending: Uint8Array[] = [];
    let at = 0;
    while (this.buffer.length - at >= 8) {
      const view = new DataView(this.buffer.buffer, this.buffer.byteOffset + at);
      let size = view.getUint32(0);
      const type = String.fromCharCode(
        this.buffer[at + 4],
        this.buffer[at + 5],
        this.buffer[at + 6],
        this.buffer[at + 7],
      );
      if (size === 1) {
        if (this.buffer.length - at < 16) break;
        size = Number(view.getBigUint64(8));
      } else if (size === 0) {
        break; // box extends to the end of the stream: wait for the rest
      }
      if (this.buffer.length - at < size) break;
      const box = this.buffer.subarray(at, at + size);
      at += size;
      if (type === "mfra" || type === "free" || type === "skip") continue;
      pending.push(box);
      if (type === "moov" || type === "mdat") {
        groups.push(concat(pending));
        pending = [];
      }
    }
    // Boxes before an incomplete one are returned with the next group.
    this.buffer = this.buffer.slice(at);
    if (pending.length) this.buffer = concat([...pending, this.buffer]);
    return groups;
  }
}

function concat(parts: Uint8Array[]): Uint8Array {
  let length = 0;
  for (const part of parts) length += part.length;
  const out = new Uint8Array(length);
  let at = 0;
  for (const part of parts) {
    out.set(part, at);
    at += part.length;
  }
  return out;
}

/** Runs source buffer operations one at a time, in order. */
class BufferQueue {
  private tail: Promise<unknown> = Promise.resolve();

  constructor(readonly buffer: SourceBuffer) {}

  run<T>(operation: (buffer: SourceBuffer) => T | Promise<T>): Promise<T> {
    const next = this.tail.then(async () => {
      if (this.buffer.updating) await once(this.buffer, "updateend");
      return operation(this.buffer);
    });
    this.tail = next.catch(() => undefined);
    return next;
  }

  /** Appends and waits for the browser to finish processing. */
  append(data: Uint8Array): Promise<void> {
    return this.run(async (buffer) => {
      buffer.appendBuffer(data as unknown as BufferSource);
      await once(buffer, "updateend");
    });
  }

  remove(start: number, end: number): Promise<void> {
    return this.run(async (buffer) => {
      if (end <= start) return;
      buffer.remove(start, end);
      await once(buffer, "updateend");
    });
  }
}

function once(target: EventTarget, name: string): Promise<void> {
  return new Promise((resolve) => target.addEventListener(name, () => resolve(), { once: true }));
}

type Feed = { done: Promise<void>; generation: number };

export class RemuxSession {
  readonly objectUrl: string;
  readonly audioTracks: AudioTrackInfo[];
  readonly duration: number;

  private readonly mediaSource = new MediaSource();
  private video?: HTMLVideoElement;
  private videoQueue?: BufferQueue;
  private audioQueue?: BufferQueue;
  private ready: Promise<void>;
  private disposed = false;
  private activeAudioId: number;
  private audioMime = "";
  private generation = { video: 0, audio: 0 };
  private feeds: { video?: Feed; audio?: Feed } = {};
  private finished = { video: false, audio: false };
  private seekTimer?: ReturnType<typeof setTimeout>;
  private wake?: () => void;
  /** Where the current feeds started; data from there on is on its way. */
  private feedFrom = 0;
  private lastEvict = 0;
  private errorListeners = new Set<(error: unknown) => void>();
  private audioListeners = new Set<(id: number) => void>();

  private constructor(
    private readonly input: Input,
    private readonly videoTrack: InputVideoTrack,
    private readonly videoConfig: VideoDecoderConfig,
    private readonly audioInputs: Map<number, InputAudioTrack>,
    audioTracks: AudioTrackInfo[],
    duration: number,
    activeAudioId: number,
  ) {
    this.audioTracks = audioTracks;
    this.duration = duration;
    this.activeAudioId = activeAudioId;
    this.objectUrl = URL.createObjectURL(this.mediaSource);
    this.ready = new Promise((resolve, reject) => {
      this.mediaSource.addEventListener(
        "sourceopen",
        () => {
          this.setup().then(resolve, reject);
        },
        { once: true },
      );
    });
    // Failures before anyone awaits `ready` are reported through onError instead.
    this.ready.catch((error) => this.fail(error));
  }

  /** True when this browser can play through Media Source Extensions at all. */
  static isSupported(): boolean {
    return supportsMse();
  }

  /** Opens the file and works out which audio tracks can be played. Throws if it cannot. */
  static async open(url: string, options: RemuxSessionOptions = {}): Promise<RemuxSession> {
    if (!supportsMse()) throw new Error("Media Source Extensions are not available");
    const input = new Input({
      source: new UrlSource(url, {
        // Retry transient failures, but give up on errors that will not go away.
        getRetryDelay: (attempt) => (attempt < 4 ? 0.5 * 2 ** attempt : null),
        // The source reads ahead up to this much. Every open and every seek starts a new read,
        // and the server streams from a remote store, so keep the speculative part modest.
        maxCacheSize: READ_AHEAD_BYTES,
      }),
      formats: ALL_FORMATS,
    });
    try {
      options.signal?.throwIfAborted();
      const videoTrack = await input.getPrimaryVideoTrack();
      if (!videoTrack) throw new Error("No video track");
      const videoConfig = await videoTrack.getDecoderConfig();
      if (!videoConfig) throw new Error("Unsupported video track");
      if (!MediaSource.isTypeSupported(mimeFor("video", videoConfig.codec))) {
        throw new Error(`This browser cannot play ${videoConfig.codec} through Media Source`);
      }

      const audioInputs = new Map<number, InputAudioTrack>();
      const infos: AudioTrackInfo[] = [];
      for (const track of await input.getAudioTracks()) {
        const codec = await track.getCodec();
        const config = await track.getDecoderConfig();
        const disposition = await track.getDisposition();
        const language = await track.getLanguageCode();
        audioInputs.set(track.id, track);
        infos.push({
          id: track.id,
          codec: codec ?? "unknown",
          language: language && language !== "und" ? language : undefined,
          name: (await track.getName()) ?? undefined,
          channels: await track.getNumberOfChannels(),
          sampleRate: await track.getSampleRate(),
          isDefault: disposition.default,
          mode: await audioModeFor(codec, config),
        });
      }
      const playable = infos.filter((info) => info.mode !== "unsupported");
      const wanted = options.pickAudio?.(infos);
      const preferred =
        playable.find((info) => info.id === wanted) ??
        playable.find((info) => info.isDefault) ??
        playable[0];
      const duration = (await input.getDurationFromMetadata()) ?? (await input.computeDuration());
      return new RemuxSession(
        input,
        videoTrack,
        videoConfig,
        audioInputs,
        infos,
        duration,
        preferred?.id ?? -1,
      );
    } catch (error) {
      input.dispose();
      throw error;
    }
  }

  get activeAudio(): number {
    return this.activeAudioId;
  }

  onError(listener: (error: unknown) => void): () => void {
    this.errorListeners.add(listener);
    return () => this.errorListeners.delete(listener);
  }

  onAudioChange(listener: (id: number) => void): () => void {
    this.audioListeners.add(listener);
    return () => this.audioListeners.delete(listener);
  }

  /** Connects the <video> element that plays this session. Call once it exists. */
  attach(video: HTMLVideoElement): void {
    if (this.video === video) return;
    this.video = video;
    video.addEventListener("seeking", this.onSeeking);
    video.addEventListener("timeupdate", this.onTimeUpdate);
    video.addEventListener("waiting", this.onWaiting);
  }

  private setup = async () => {
    const mediaSource = this.mediaSource;
    if (Number.isFinite(this.duration) && this.duration > 0) mediaSource.duration = this.duration;
    this.videoQueue = new BufferQueue(
      mediaSource.addSourceBuffer(mimeFor("video", this.videoConfig.codec)),
    );
    const active = this.audioTracks.find((track) => track.id === this.activeAudioId);
    if (active) {
      const config = await this.audioConfigFor(active);
      this.audioMime = mimeFor("audio", config);
      this.audioQueue = new BufferQueue(mediaSource.addSourceBuffer(this.audioMime));
    }
    this.startFeeds(this.video?.currentTime ?? 0);
  };

  private async audioConfigFor(track: AudioTrackInfo): Promise<string> {
    if (track.mode === "transcode") return "opus";
    const input = this.audioInputs.get(track.id);
    const config = await input?.getDecoderConfig();
    if (!config) throw new Error("Unsupported audio track");
    return config.codec;
  }

  // ── Feeding ───────────────────────────────────────────────

  private startFeeds(time: number) {
    this.feedFrom = time;
    this.finished = { video: false, audio: !this.audioQueue };
    this.feeds.video = this.startFeed("video", time);
    if (this.audioQueue) this.feeds.audio = this.startFeed("audio", time);
  }

  private startFeed(kind: "video" | "audio", time: number): Feed {
    const generation = ++this.generation[kind];
    const done = (
      kind === "video" ? this.feedVideo(time, generation) : this.feedAudio(time, generation)
    )
      .then(() => {
        if (generation !== this.generation[kind]) return;
        this.finished[kind] = true;
        this.maybeEndStream();
      })
      .catch((error) => {
        if (error instanceof StaleError || generation !== this.generation[kind]) return;
        this.fail(error);
      });
    return { done, generation };
  }

  private maybeEndStream() {
    if (this.disposed || !this.finished.video || !this.finished.audio) return;
    if (this.mediaSource.readyState !== "open") return;
    const queues = [this.videoQueue, this.audioQueue].filter(
      (queue): queue is BufferQueue => !!queue,
    );
    void Promise.all(queues.map((queue) => queue.run(() => undefined))).then(() => {
      if (!this.disposed && this.mediaSource.readyState === "open") this.mediaSource.endOfStream();
    });
  }

  private async feedVideo(time: number, generation: number) {
    const queue = this.videoQueue;
    if (!queue) return;
    const sink = new EncodedPacketSink(this.videoTrack);
    const first = (await sink.getKeyPacket(time)) ?? (await sink.getFirstPacket());
    this.assertCurrent("video", generation);
    if (!first) return;
    const codec = await this.videoTrack.getCodec();
    if (!codec) throw new Error("Unsupported video codec");
    const source = new EncodedVideoPacketSource(codec);
    const output = this.createOutput(queue, "video", generation);
    output.addVideoTrack(source);
    await output.start();
    try {
      let sendConfig = true;
      for await (const packet of sink.packets(first)) {
        this.assertCurrent("video", generation);
        await source.add(packet, sendConfig ? { decoderConfig: this.videoConfig } : undefined);
        sendConfig = false;
      }
      this.assertCurrent("video", generation);
      await output.finalize();
    } catch (error) {
      await output.cancel().catch(() => undefined);
      throw error;
    }
  }

  private async feedAudio(time: number, generation: number) {
    const queue = this.audioQueue;
    const info = this.audioTracks.find((track) => track.id === this.activeAudioId);
    const track = this.audioInputs.get(this.activeAudioId);
    if (!queue || !info || !track) return;
    const output = this.createOutput(queue, "audio", generation);
    const decoderConfig = await track.getDecoderConfig();
    this.assertCurrent("audio", generation);
    try {
      if (info.mode === "direct") {
        const codec = await track.getCodec();
        if (!codec || !decoderConfig) throw new Error("Unsupported audio codec");
        const source = new EncodedAudioPacketSource(codec);
        output.addAudioTrack(source);
        await output.start();
        const sink = new EncodedPacketSink(track);
        const first = (await sink.getPacket(time)) ?? (await sink.getFirstPacket());
        this.assertCurrent("audio", generation);
        if (first) {
          let sendConfig = true;
          for await (const packet of sink.packets(first)) {
            this.assertCurrent("audio", generation);
            await source.add(packet, sendConfig ? { decoderConfig } : undefined);
            sendConfig = false;
          }
        }
      } else {
        // Decode (E-AC-3 and friends have no native decoder) and encode as stereo Opus.
        const source = new AudioSampleSource({
          codec: "opus",
          bitrate: 160_000,
          transform: { numberOfChannels: Math.min(info.channels, 2), sampleRate: 48_000 },
        });
        output.addAudioTrack(source);
        if (info.codec === "ac3" || info.codec === "eac3") await ensureAc3Decoder();
        await output.start();
        const sink = new AudioSampleSink(track);
        for await (const sample of sink.samples(time)) {
          try {
            this.assertCurrent("audio", generation);
            await source.add(sample);
          } finally {
            sample.close();
          }
        }
      }
      this.assertCurrent("audio", generation);
      await output.finalize();
    } catch (error) {
      await output.cancel().catch(() => undefined);
      throw error;
    }
  }

  private createOutput(queue: BufferQueue, kind: "video" | "audio", generation: number): Output {
    const splitter = new BoxSplitter();
    const writable = new WritableStream<Uint8Array>({
      write: async (chunk) => {
        for (const group of splitter.push(chunk)) {
          await this.waitForRoom(queue, kind, generation);
          await queue.append(group).catch((error) => {
            if (error?.name === "QuotaExceededError") throw new Error("Out of buffer space");
            throw error;
          });
          this.assertCurrent(kind, generation);
        }
      },
    });
    return new Output({
      format: new Mp4OutputFormat({ fastStart: "fragmented", minimumFragmentDuration: 1 }),
      target: new AppendOnlyStreamTarget(writable),
    });
  }

  private assertCurrent(kind: "video" | "audio", generation: number) {
    if (this.disposed || generation !== this.generation[kind]) throw new StaleError();
  }

  /** Pauses feeding while enough is buffered ahead of the playhead. */
  private async waitForRoom(queue: BufferQueue, kind: "video" | "audio", generation: number) {
    for (;;) {
      this.assertCurrent(kind, generation);
      const video = this.video;
      if (!video) return;
      const ahead = this.bufferedAhead(queue.buffer, video.currentTime);
      // Keep going while the playhead is outside the buffer (playback is starved) or the buffer
      // ahead of it is not full yet.
      if (ahead === undefined || ahead < AHEAD_MAX) return;
      await new Promise<void>((resolve) => {
        const timer = setTimeout(resolve, 1000);
        this.wake = () => {
          clearTimeout(timer);
          resolve();
        };
      });
    }
  }

  /** Seconds buffered after `time` in the range containing it; undefined when `time` is not buffered. */
  private bufferedAhead(buffer: SourceBuffer, time: number): number | undefined {
    const ranges = buffer.buffered;
    for (let i = 0; i < ranges.length; i++) {
      if (time >= ranges.start(i) - 0.1 && time <= ranges.end(i)) return ranges.end(i) - time;
    }
    return undefined;
  }

  // ── Events ────────────────────────────────────────────────

  private onTimeUpdate = () => {
    this.wake?.();
    const video = this.video;
    if (!video || video.seeking) return;
    // Drop data well behind the playhead so memory stays bounded.
    const now = performance.now();
    if (now - this.lastEvict < 5000) return;
    this.lastEvict = now;
    for (const queue of [this.videoQueue, this.audioQueue]) {
      if (!queue || queue.buffer.buffered.length === 0) continue;
      const start = queue.buffer.buffered.start(0);
      const cutoff = video.currentTime - BEHIND_KEEP;
      if (cutoff - start > 10) void queue.remove(start, cutoff).catch(() => undefined);
    }
  };

  private onSeeking = () => {
    clearTimeout(this.seekTimer);
    this.seekTimer = setTimeout(() => void this.handleSeek(), SEEK_DEBOUNCE_MS);
  };

  private onWaiting = () => {
    const video = this.video;
    if (!video || video.seeking || this.seekTimer) return;
    // At the start, and right after a refill, the player waits for data that is on its way.
    // Only when nothing is coming for the playhead (the feeds ended, or began after it) refill.
    const feeding = !this.finished.video || !this.finished.audio;
    if (feeding && video.currentTime >= this.feedFrom - 1) return;
    if (!this.isBuffered(video.currentTime)) void this.restart(video.currentTime);
  };

  private async handleSeek() {
    this.seekTimer = undefined;
    const video = this.video;
    if (!video || this.disposed) return;
    const time = video.currentTime;
    if (this.isBuffered(time)) {
      this.wake?.();
      return;
    }
    await this.restart(time);
  }

  private isBuffered(time: number): boolean {
    const queues = [this.videoQueue, this.audioQueue].filter(
      (queue): queue is BufferQueue => !!queue,
    );
    if (queues.length === 0) return false;
    return queues.every((queue) => (this.bufferedAhead(queue.buffer, time) ?? 0) > 0.25);
  }

  /** Throws away everything buffered and refills from `time`. */
  private async restart(time: number) {
    if (this.disposed || this.mediaSource.readyState === "closed") return;
    // Bump the generations first so running feeds stop at their next step.
    const stopping = [this.feeds.video, this.feeds.audio];
    this.generation.video++;
    this.generation.audio++;
    this.wake?.();
    await Promise.all(stopping.map((feed) => feed?.done));
    if (this.disposed) return;
    await this.clearBuffers(!!this.audioQueue);
    this.startFeeds(time);
  }

  private async clearBuffers(includeAudio: boolean) {
    const queues = [this.videoQueue, includeAudio ? this.audioQueue : undefined];
    for (const queue of queues) {
      if (!queue) continue;
      await queue
        .run((buffer) => {
          if (this.mediaSource.readyState === "open" && buffer.updating) buffer.abort();
        })
        .catch(() => undefined);
      await queue.remove(0, Number.POSITIVE_INFINITY).catch(() => undefined);
    }
  }

  // ── Audio selection ───────────────────────────────────────

  async selectAudio(id: number): Promise<void> {
    await this.ready;
    if (this.disposed || id === this.activeAudioId) return;
    const info = this.audioTracks.find((track) => track.id === id);
    if (!info || info.mode === "unsupported") throw new Error("This audio track cannot be played");
    const video = this.video;
    const time = video?.currentTime ?? 0;
    const previous = this.feeds.audio;
    this.generation.audio++;
    this.wake?.();
    await previous?.done;
    if (this.disposed) return;
    const mime = mimeFor("audio", await this.audioConfigFor(info));
    this.activeAudioId = id;
    if (!this.audioQueue) {
      this.audioQueue = new BufferQueue(this.mediaSource.addSourceBuffer(mime));
      this.audioMime = mime;
    } else {
      await this.audioQueue.run((buffer) => {
        if (this.mediaSource.readyState === "open" && buffer.updating) buffer.abort();
      });
      await this.audioQueue.remove(0, Number.POSITIVE_INFINITY);
      if (mime !== this.audioMime) {
        await this.audioQueue.run((buffer) => buffer.changeType(mime));
        this.audioMime = mime;
      }
    }
    this.finished.audio = false;
    this.feeds.audio = this.startFeed("audio", time);
    for (const listener of this.audioListeners) listener(id);
  }

  // ── Lifecycle ─────────────────────────────────────────────

  private fail(error: unknown) {
    if (this.disposed) return;
    for (const listener of this.errorListeners) listener(error);
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    clearTimeout(this.seekTimer);
    this.generation.video++;
    this.generation.audio++;
    this.wake?.();
    const video = this.video;
    video?.removeEventListener("seeking", this.onSeeking);
    video?.removeEventListener("timeupdate", this.onTimeUpdate);
    video?.removeEventListener("waiting", this.onWaiting);
    this.input.dispose();
    URL.revokeObjectURL(this.objectUrl);
  }
}
