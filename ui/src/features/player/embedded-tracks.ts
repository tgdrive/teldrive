// Coordinates the audio and subtitle tracks stored inside a video file for one player instance.
//
// What is done depends on the file:
//   - a Matroska/WebM file with several audio tracks, or one whose audio the browser cannot
//     decode, is played through a RemuxSession so the track can be chosen (see remux-session.ts);
//   - every other file keeps playing natively, exactly as before;
//   - embedded text subtitles are read lazily, only for the track the viewer selects, and only
//     ahead of the playhead.

import type { FileEntry } from "@/api/types";
import { MatroskaFile, type MkvTrack, RangeReader } from "./matroska";
import type { AudioMode, AudioTrackInfo, RemuxSession } from "./remux-session";
import { blockToCue, mergeOverlapping, subtitleFormat, type TextCue } from "./subtitle-text";
import { audioDetail, languageKey, trackTitle } from "./track-labels";

export type AudioChoice = {
  id: number;
  label: string;
  detail: string;
  language?: string;
  mode: AudioMode;
};

export type SubtitleChoice = {
  /** The id given to the player's subtitle list. */
  id: string;
  number: number;
  label: string;
  language?: string;
  /** False for bitmap formats (PGS, VobSub), which are listed but cannot be shown. */
  supported: boolean;
  isDefault: boolean;
};

export type TracksSnapshot = {
  audio: AudioChoice[];
  activeAudio?: number;
  audioBusy: boolean;
  subtitles: SubtitleChoice[];
  /** Cues of the selected embedded subtitle track that have been read so far. */
  cues: TextCue[];
  activeSubtitle?: number;
  subtitleStatus: "idle" | "loading" | "ready" | "error";
};

export type PlaybackSource = {
  src: string;
  /** Set when the player should not guess the type from the URL. */
  type?: string;
};

const EMBEDDED_PREFIX = "embedded:";
/** How far ahead of the playhead subtitles are read, in seconds. */
const LOOKAHEAD = 120;
/** A playhead jump bigger than this is a seek, so reading in the old place is abandoned. */
const SEEK_JUMP = 10;
const RETRY_AFTER_ERROR_MS = 10_000;

const AUDIO_STORAGE_KEY = "teldrive-player-audio-language";
const SUBTITLE_STORAGE_KEY = "teldrive-player-subtitle-language";

export const subtitleId = (number: number) => `${EMBEDDED_PREFIX}${number}`;
export function embeddedSubtitleNumber(id: string | undefined): number | undefined {
  if (!id?.startsWith(EMBEDDED_PREFIX)) return undefined;
  const number = Number(id.slice(EMBEDDED_PREFIX.length));
  return Number.isInteger(number) ? number : undefined;
}

function readPreference(key: string): string | undefined {
  try {
    return window.localStorage.getItem(key) ?? undefined;
  } catch {
    return undefined;
  }
}

function writePreference(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Private mode and similar: the choice just is not remembered.
  }
}

/** Matroska and WebM files are where multi-track videos live. */
export function isMatroskaFile(file: Pick<FileEntry, "name" | "mimeType">): boolean {
  const name = file.name.toLowerCase();
  return (
    name.endsWith(".mkv") ||
    name.endsWith(".webm") ||
    name.endsWith(".mka") ||
    file.mimeType === "video/x-matroska" ||
    file.mimeType === "video/webm"
  );
}

// Audio codecs a <video> plays natively when it demuxes Matroska itself.
const NATIVE_AUDIO = new Set(["A_AAC", "A_OPUS", "A_VORBIS", "A_FLAC", "A_MPEG/L3", "A_MPEG/L2"]);

function playsNatively(codecId: string): boolean {
  if (NATIVE_AUDIO.has(codecId) || codecId.startsWith("A_PCM/")) return true;
  // Dolby codecs depend on the platform: ask the browser instead of assuming.
  const dolby = codecId.startsWith("A_EAC3") ? "ec-3" : codecId.startsWith("A_AC3") ? "ac-3" : "";
  return (
    !!dolby &&
    typeof MediaSource !== "undefined" &&
    MediaSource.isTypeSupported(`audio/mp4; codecs="${dolby}"`)
  );
}

type TrackCues = {
  raw: TextCue[];
  seen: Set<string>;
  /** Time ranges that have been read completely, sorted and non-overlapping. */
  covered: [number, number][];
};

function addCovered(ranges: [number, number][], from: number, to: number) {
  ranges.push([from, to]);
  ranges.sort((a, b) => a[0] - b[0]);
  const merged: [number, number][] = [];
  for (const range of ranges) {
    const last = merged[merged.length - 1];
    if (last && range[0] <= last[1]) last[1] = Math.max(last[1], range[1]);
    else merged.push([range[0], range[1]]);
  }
  ranges.length = 0;
  ranges.push(...merged);
}

/** The first part of [from, to] that is not covered, if any. */
function firstGap(ranges: [number, number][], from: number, to: number): number | undefined {
  let at = from;
  for (const [start, end] of ranges) {
    if (end <= at) continue;
    if (start > at) break;
    at = end;
  }
  return at < to ? at : undefined;
}

export class EmbeddedTracks {
  private snapshot: TracksSnapshot = {
    audio: [],
    audioBusy: false,
    subtitles: [],
    cues: [],
    subtitleStatus: "idle",
  };
  private listeners = new Set<() => void>();
  private disposed = false;

  private mkv?: MatroskaFile;
  private session?: RemuxSession;
  private fallbackListeners = new Set<(source: PlaybackSource) => void>();
  private subtitleTracks = new Map<number, MkvTrack>();
  private cues = new Map<number, TrackCues>();
  private playhead = 0;
  private fillAbort?: AbortController;
  private pumping = false;
  private publishTimer?: ReturnType<typeof setTimeout>;
  private retryAt = 0;
  private video?: HTMLVideoElement;
  private resumeAt?: number;

  constructor(
    private readonly url: string,
    private readonly locale: string,
    private readonly trackName: (number: number) => string,
  ) {}

  // ── State ─────────────────────────────────────────────────

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = () => this.snapshot;

  private update(patch: Partial<TracksSnapshot>) {
    if (this.disposed) return;
    this.snapshot = { ...this.snapshot, ...patch };
    for (const listener of this.listeners) listener();
  }

  /** Called when remuxed playback fails and the player should use the file directly. */
  onFallback(listener: (source: PlaybackSource) => void): () => void {
    this.fallbackListeners.add(listener);
    return () => this.fallbackListeners.delete(listener);
  }

  // ── Loading ───────────────────────────────────────────────

  /**
   * Looks inside the file and decides how to play it. Never rejects: a file that cannot be
   * inspected is simply played as it always was.
   */
  async load(signal: AbortSignal): Promise<PlaybackSource> {
    const direct: PlaybackSource = { src: this.url };
    let mkv: MatroskaFile;
    try {
      mkv = await MatroskaFile.open(new RangeReader(this.url, { signal }));
    } catch {
      return direct;
    }
    if (signal.aborted || this.disposed) return direct;
    this.mkv = mkv;

    const subtitles: SubtitleChoice[] = [];
    let index = 0;
    for (const track of mkv.info.tracks) {
      if (track.type !== "subtitle") continue;
      const supported = subtitleFormat(track) !== undefined;
      this.subtitleTracks.set(track.number, track);
      subtitles.push({
        id: subtitleId(track.number),
        number: track.number,
        label: trackTitle(track, index++, this.locale, this.trackName),
        language: track.language,
        supported,
        isDefault: false,
      });
    }
    // Turn subtitles on again if the viewer last chose this language.
    const wantedSubtitle = readPreference(SUBTITLE_STORAGE_KEY);
    if (wantedSubtitle && wantedSubtitle !== "off") {
      const match = subtitles.find(
        (track) => track.supported && languageKey(track.language) === wantedSubtitle,
      );
      if (match) match.isDefault = true;
    }
    this.update({ subtitles });

    const audio = mkv.info.tracks.filter((track) => track.type === "audio");
    const needsRemux =
      audio.length >= 2 || (audio.length === 1 && !playsNatively(audio[0].codecId));
    if (!needsRemux) return direct;
    return (await this.startRemux(signal)) ?? direct;
  }

  private async startRemux(signal: AbortSignal): Promise<PlaybackSource | undefined> {
    try {
      const { RemuxSession } = await import("./remux-session");
      if (!RemuxSession.isSupported()) return undefined;
      const preferred = readPreference(AUDIO_STORAGE_KEY);
      const session = await RemuxSession.open(this.url, {
        signal,
        // Start on the language the viewer chose last time, if this file has it.
        pickAudio: (tracks) =>
          preferred
            ? tracks.find(
                (track) =>
                  track.mode !== "unsupported" && languageKey(track.language) === preferred,
              )?.id
            : undefined,
      });
      if (signal.aborted || this.disposed) {
        session.dispose();
        return undefined;
      }
      const playable = session.audioTracks.filter((track) => track.mode !== "unsupported");
      if (playable.length === 0) {
        session.dispose();
        return undefined;
      }
      this.session = session;
      session.onError((error) => {
        console.warn("Falling back to direct playback:", error);
        this.fallBack();
      });
      session.onAudioChange((id) => this.update({ activeAudio: id }));
      const choices = session.audioTracks.map((track, index) => this.audioChoice(track, index));
      this.update({ audio: choices, activeAudio: session.activeAudio });
      return { src: session.objectUrl, type: "video/mp4" };
    } catch (error) {
      console.warn("Audio track selection is unavailable for this file:", error);
      return undefined;
    }
  }

  private audioChoice(track: AudioTrackInfo, index: number): AudioChoice {
    return {
      id: track.id,
      label: trackTitle(track, index, this.locale, this.trackName),
      detail: audioDetail(track.codec, track.channels),
      language: track.language,
      mode: track.mode,
    };
  }

  private fallBack() {
    if (this.disposed || !this.session) return;
    this.resumeAt = this.video?.currentTime || undefined;
    this.session.dispose();
    this.session = undefined;
    this.update({ audio: [], activeAudio: undefined, audioBusy: false });
    for (const listener of this.fallbackListeners) listener({ src: this.url });
  }

  /** Connects the player's <video> element to the remuxed playback, if there is one. */
  attachVideo(video: HTMLVideoElement | null) {
    this.video = video ?? undefined;
    if (video) this.session?.attach(video);
  }

  /** Where playback should continue after falling back to the plain file; once only. */
  takeResumeTime(): number | undefined {
    const time = this.resumeAt;
    this.resumeAt = undefined;
    return time;
  }

  // ── Audio ─────────────────────────────────────────────────

  async selectAudio(id: number): Promise<boolean> {
    const session = this.session;
    if (!session || this.snapshot.audioBusy || id === this.snapshot.activeAudio) return true;
    const track = session.audioTracks.find((candidate) => candidate.id === id);
    if (!track || track.mode === "unsupported") return false;
    this.update({ audioBusy: true });
    try {
      await session.selectAudio(id);
      const language = languageKey(track.language);
      if (language) writePreference(AUDIO_STORAGE_KEY, language);
      this.update({ activeAudio: id });
      return true;
    } catch (error) {
      console.warn("Could not switch audio track:", error);
      return false;
    } finally {
      this.update({ audioBusy: false });
    }
  }

  // ── Subtitles ─────────────────────────────────────────────

  /** Starts (or stops, with undefined) reading an embedded subtitle track. */
  selectSubtitle(number: number | undefined) {
    if (number === this.snapshot.activeSubtitle) return;
    this.fillAbort?.abort();
    const track = number === undefined ? undefined : this.subtitleTracks.get(number);
    const supported = track && subtitleFormat(track);
    if (number !== undefined && !supported) {
      this.update({ activeSubtitle: undefined, cues: [], subtitleStatus: "idle" });
      return;
    }
    this.update({
      activeSubtitle: number,
      cues: number === undefined ? [] : this.merged(number),
      subtitleStatus: number === undefined ? "idle" : "loading",
    });
    if (number !== undefined) {
      this.retryAt = 0;
      void this.pump();
    }
  }

  /** Remembers the viewer's choice so the next video starts the same way. */
  rememberSubtitle(number: number | undefined) {
    if (number === undefined) {
      writePreference(SUBTITLE_STORAGE_KEY, "off");
      return;
    }
    const language = languageKey(this.subtitleTracks.get(number)?.language);
    if (language) writePreference(SUBTITLE_STORAGE_KEY, language);
  }

  /** The player reports its position; reading continues ahead of it. */
  setPlayhead(time: number) {
    const jumped = Math.abs(time - this.playhead) > SEEK_JUMP;
    this.playhead = time;
    if (this.snapshot.activeSubtitle === undefined) return;
    if (jumped) this.fillAbort?.abort();
    if (!this.pumping && Date.now() >= this.retryAt) void this.pump();
  }

  private merged(number: number): TextCue[] {
    return mergeOverlapping(this.cues.get(number)?.raw ?? []);
  }

  private publish() {
    if (this.publishTimer) return;
    this.publishTimer = setTimeout(() => {
      this.publishTimer = undefined;
      const number = this.snapshot.activeSubtitle;
      if (number !== undefined) this.update({ cues: this.merged(number) });
    }, 250);
  }

  private async pump() {
    if (this.pumping || !this.mkv) return;
    this.pumping = true;
    try {
      for (;;) {
        const number = this.snapshot.activeSubtitle;
        if (number === undefined || this.disposed) break;
        const stored = this.cues.get(number) ?? { raw: [], seen: new Set(), covered: [] };
        this.cues.set(number, stored);
        const from = Math.max(0, this.playhead - 5);
        const gap = firstGap(stored.covered, from, this.playhead + LOOKAHEAD / 2);
        if (gap === undefined) break;
        const finished = await this.fill(number, stored, gap);
        if (!finished) continue; // abandoned for a seek or a new selection: look again
      }
      if (this.snapshot.activeSubtitle !== undefined && this.snapshot.subtitleStatus !== "error") {
        this.update({ subtitleStatus: "ready", cues: this.merged(this.snapshot.activeSubtitle) });
      }
    } catch (error) {
      if (!this.disposed) {
        console.warn("Could not read subtitles:", error);
        this.retryAt = Date.now() + RETRY_AFTER_ERROR_MS;
        this.update({ subtitleStatus: "error" });
      }
    } finally {
      this.pumping = false;
    }
  }

  /** Reads clusters from about `from` until the lookahead is filled. False if abandoned. */
  private async fill(number: number, stored: TrackCues, from: number): Promise<boolean> {
    const mkv = this.mkv;
    const track = this.subtitleTracks.get(number);
    const format = track && subtitleFormat(track);
    if (!mkv || !track || !format) return true;
    const controller = new AbortController();
    this.fillAbort = controller;
    const offset = await mkv.clusterOffsetFor(Math.max(0, from - 10));
    if (offset === undefined) return true;
    let firstCluster: number | undefined;
    let lastCluster = from;
    try {
      const result = await mkv.scan(offset, new Set([number]), {
        signal: controller.signal,
        onCluster: (time) => {
          firstCluster ??= time;
          lastCluster = time;
          return time <= this.playhead + LOOKAHEAD;
        },
        onBlock: (block) => {
          const cue = blockToCue(block, format);
          if (!cue) return;
          const key = `${cue.start}|${cue.end}|${cue.text}`;
          if (stored.seen.has(key)) return;
          stored.seen.add(key);
          stored.raw.push(cue);
          this.publish();
        },
      });
      // Whatever was read counts as covered, so the same part is never read again. No clusters
      // at all means there is nothing at or after this point.
      if (firstCluster === undefined) addCovered(stored.covered, from, Number.POSITIVE_INFINITY);
      else
        addCovered(
          stored.covered,
          firstCluster,
          result.reachedEnd ? Number.POSITIVE_INFINITY : lastCluster,
        );
      return true;
    } catch (error) {
      if (controller.signal.aborted) return false;
      throw error;
    }
  }

  // ── Lifecycle ─────────────────────────────────────────────

  dispose() {
    this.disposed = true;
    this.fillAbort?.abort();
    clearTimeout(this.publishTimer);
    this.session?.dispose();
    this.session = undefined;
    this.listeners.clear();
    this.fallbackListeners.clear();
  }
}
