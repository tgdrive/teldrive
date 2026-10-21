// Minimal Matroska/WebM reader for the web player.
//
// Browsers do not expose the subtitle tracks stored inside a Matroska file, and Mediabunny reads
// audio and video only. This module reads just enough of the container over HTTP range requests
// to list the tracks and to pull the blocks of chosen (subtitle) tracks out of the clusters:
//
//   - header / track metadata: a few small reads at the start of the file;
//   - cue points (when present) to jump to a time without scanning from the beginning;
//   - clusters, walked block by block, keeping only the blocks of the requested tracks.
//
// Nothing here decodes media. It is deliberately small and strict about bounds, because the
// input is an arbitrary user file.

const ID = {
  EBML: 0x1a45dfa3,
  Segment: 0x18538067,
  SeekHead: 0x114d9b74,
  Seek: 0x4dbb,
  SeekID: 0x53ab,
  SeekPosition: 0x53ac,
  Info: 0x1549a966,
  TimestampScale: 0x2ad7b1,
  Duration: 0x4489,
  Tracks: 0x1654ae6b,
  TrackEntry: 0xae,
  TrackNumber: 0xd7,
  TrackType: 0x83,
  FlagDefault: 0x88,
  FlagForced: 0x55aa,
  CodecID: 0x86,
  CodecPrivate: 0x63a2,
  Language: 0x22b59c,
  LanguageBcp47: 0x22b59d,
  Name: 0x536e,
  Cluster: 0x1f43b675,
  Timestamp: 0xe7,
  SimpleBlock: 0xa3,
  BlockGroup: 0xa0,
  Block: 0xa1,
  BlockDuration: 0x9b,
  Cues: 0x1c53bb6b,
  CuePoint: 0xbb,
  CueTime: 0xb3,
  CueTrackPositions: 0xb7,
  CueClusterPosition: 0xf1,
  Tags: 0x1254c367,
  Attachments: 0x1941a469,
  Chapters: 0x1043a770,
} as const;

// IDs that can only appear at the top level of a Segment. They end a cluster of unknown size.
const TOP_LEVEL_IDS = new Set<number>([
  ID.Cluster,
  ID.Cues,
  ID.Tags,
  ID.Attachments,
  ID.Chapters,
  ID.SeekHead,
  ID.Info,
  ID.Tracks,
]);

const UNKNOWN_SIZE = -1;
const MAX_HEADER = 12;
const MAX_MASTER_BYTES = 32 * 1024 * 1024;
/** Text subtitle events are tiny; a block claiming more than this is corrupt, not a subtitle. */
const MAX_TEXT_BLOCK_BYTES = 1024 * 1024;
/** Integers in EBML are at most eight bytes. */
const MAX_UINT_BYTES = 8;

export type MkvTrackType = "video" | "audio" | "subtitle" | "other";

export type MkvTrack = {
  number: number;
  type: MkvTrackType;
  codecId: string;
  language?: string;
  name?: string;
  isDefault: boolean;
  isForced: boolean;
  codecPrivate?: Uint8Array;
};

export type MkvInfo = {
  size: number;
  /** Absolute offset of the first byte after the Segment header. */
  segmentOffset: number;
  /** Seconds per timestamp tick: TimestampScale / 1e9. */
  secondsPerTick: number;
  durationSeconds?: number;
  tracks: MkvTrack[];
  /** Absolute offset of the first cluster, if any was found. */
  firstClusterOffset?: number;
  /** Absolute offset of the Cues element, if the SeekHead pointed at one. */
  cuesOffset?: number;
};

export type MkvCuePoint = { time: number; clusterOffset: number };

export type MkvBlock = {
  track: number;
  /** Presentation time in seconds. */
  time: number;
  /** Duration in seconds, when the container stores one (BlockGroup + BlockDuration). */
  duration?: number;
  data: Uint8Array;
};

// ── Reading ─────────────────────────────────────────────────

export type RangeReaderOptions = {
  fetchFn?: typeof fetch;
  signal?: AbortSignal;
  blockSize?: number;
  maxBlocks?: number;
};

/**
 * Random-access reads over HTTP range requests with a small block cache. Requests are for whole
 * blocks and the next block is fetched ahead, so a sequential walk through a file costs one
 * request per block rather than one per read.
 */
export class RangeReader {
  size: number | undefined;
  private blocks = new Map<number, Promise<Uint8Array>>();
  private readonly blockSize: number;
  private readonly maxBlocks: number;
  private readonly fetchFn: typeof fetch;
  private readonly signal?: AbortSignal;

  constructor(
    private readonly url: string,
    options: RangeReaderOptions = {},
  ) {
    this.blockSize = options.blockSize ?? 512 * 1024;
    this.maxBlocks = options.maxBlocks ?? 8;
    this.fetchFn = options.fetchFn ?? ((...args) => fetch(...args));
    this.signal = options.signal;
  }

  private fetchBlock(index: number): Promise<Uint8Array> {
    const cached = this.blocks.get(index);
    if (cached) {
      // Refresh recency.
      this.blocks.delete(index);
      this.blocks.set(index, cached);
      return cached;
    }
    const start = index * this.blockSize;
    const promise = (async () => {
      const response = await this.fetchFn(this.url, {
        headers: { Range: `bytes=${start}-${start + this.blockSize - 1}` },
        signal: this.signal,
      });
      if (response.status === 416) return new Uint8Array(0);
      if (response.status !== 206) {
        throw new Error(`Range request failed (${response.status})`);
      }
      const total = /\/(\d+)\s*$/.exec(response.headers.get("Content-Range") ?? "")?.[1];
      if (total) this.size = Number(total);
      return new Uint8Array(await response.arrayBuffer());
    })();
    // A failed block must not stay cached.
    promise.catch(() => {
      if (this.blocks.get(index) === promise) this.blocks.delete(index);
    });
    this.blocks.set(index, promise);
    while (this.blocks.size > this.maxBlocks) {
      const oldest = this.blocks.keys().next().value;
      if (oldest === undefined) break;
      this.blocks.delete(oldest);
    }
    return promise;
  }

  /** Reads up to `length` bytes at `offset`; shorter only at the end of the file. */
  async read(offset: number, length: number): Promise<Uint8Array> {
    if (length <= 0 || offset < 0) return new Uint8Array(0);
    const first = Math.floor(offset / this.blockSize);
    const last = Math.floor((offset + length - 1) / this.blockSize);
    const pending: Promise<Uint8Array>[] = [];
    for (let index = first; index <= last; index++) pending.push(this.fetchBlock(index));
    // Look ahead one block for sequential scans, unless the file is known to end before it.
    const ahead = last + 1;
    if (this.size === undefined || ahead * this.blockSize < this.size) {
      this.fetchBlock(ahead).catch(() => undefined);
    }
    const chunks = await Promise.all(pending);
    const out = new Uint8Array(length);
    let written = 0;
    for (let i = 0; i < chunks.length; i++) {
      const chunk = chunks[i];
      const chunkStart = (first + i) * this.blockSize;
      const from = Math.max(offset, chunkStart) - chunkStart;
      const to = Math.min(offset + length, chunkStart + chunk.length) - chunkStart;
      if (to <= from) break;
      out.set(chunk.subarray(from, to), written);
      written += to - from;
      if (chunk.length < this.blockSize) break; // short block: end of file
    }
    return written === length ? out : out.subarray(0, written);
  }
}

// ── EBML primitives ─────────────────────────────────────────

type Header = { id: number; size: number; headerLength: number };

function readId(bytes: Uint8Array, at: number): { id: number; length: number } | undefined {
  const first = bytes[at];
  if (first === undefined || first === 0) return undefined;
  const length = Math.clz32(first) - 23; // leading zero bits within the byte, plus one
  if (length < 1 || length > 4 || at + length > bytes.length) return undefined;
  let id = 0;
  for (let i = 0; i < length; i++) id = id * 256 + bytes[at + i];
  return { id, length };
}

function readSize(bytes: Uint8Array, at: number): { size: number; length: number } | undefined {
  const first = bytes[at];
  if (first === undefined || first === 0) return undefined;
  const length = Math.clz32(first) - 23;
  if (length < 1 || length > 8 || at + length > bytes.length) return undefined;
  let value = first & (0xff >> length);
  let allOnes = value === 0xff >> length;
  for (let i = 1; i < length; i++) {
    const byte = bytes[at + i];
    if (byte !== 0xff) allOnes = false;
    value = value * 256 + byte;
  }
  if (allOnes) return { size: UNKNOWN_SIZE, length };
  if (!Number.isSafeInteger(value)) return undefined;
  return { size: value, length };
}

function parseHeader(bytes: Uint8Array, at = 0): Header | undefined {
  const id = readId(bytes, at);
  if (!id) return undefined;
  const size = readSize(bytes, at + id.length);
  if (!size) return undefined;
  return { id: id.id, size: size.size, headerLength: id.length + size.length };
}

function toUint(data: Uint8Array): number {
  let value = 0;
  for (const byte of data) value = value * 256 + byte;
  return value;
}

function toFloat(data: Uint8Array): number {
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  if (data.length === 4) return view.getFloat32(0);
  if (data.length === 8) return view.getFloat64(0);
  return Number.NaN;
}

function toText(data: Uint8Array): string {
  let end = data.length;
  while (end > 0 && data[end - 1] === 0) end--; // strings may be NUL padded
  return new TextDecoder().decode(data.subarray(0, end));
}

type Child = { id: number; data: Uint8Array };

/** Splits the payload of a small master element into its direct children. */
function children(payload: Uint8Array): Child[] {
  const out: Child[] = [];
  let at = 0;
  while (at < payload.length) {
    const header = parseHeader(payload, at);
    if (!header || header.size === UNKNOWN_SIZE) break;
    const start = at + header.headerLength;
    const end = start + header.size;
    if (end > payload.length) break;
    out.push({ id: header.id, data: payload.subarray(start, end) });
    at = end;
  }
  return out;
}

function trackType(value: number): MkvTrackType {
  if (value === 1) return "video";
  if (value === 2) return "audio";
  if (value === 17) return "subtitle";
  return "other";
}

function parseTrackEntry(payload: Uint8Array): MkvTrack | undefined {
  const track: MkvTrack = {
    number: 0,
    type: "other",
    codecId: "",
    isDefault: true, // FlagDefault defaults to 1
    isForced: false,
  };
  let language: string | undefined;
  let bcp47: string | undefined;
  for (const child of children(payload)) {
    switch (child.id) {
      case ID.TrackNumber:
        track.number = toUint(child.data);
        break;
      case ID.TrackType:
        track.type = trackType(toUint(child.data));
        break;
      case ID.FlagDefault:
        track.isDefault = toUint(child.data) !== 0;
        break;
      case ID.FlagForced:
        track.isForced = toUint(child.data) !== 0;
        break;
      case ID.CodecID:
        track.codecId = toText(child.data);
        break;
      case ID.CodecPrivate:
        track.codecPrivate = child.data;
        break;
      case ID.Language:
        language = toText(child.data);
        break;
      case ID.LanguageBcp47:
        bcp47 = toText(child.data);
        break;
      case ID.Name:
        track.name = toText(child.data) || undefined;
        break;
    }
  }
  if (track.number <= 0) return undefined;
  const chosen = bcp47 || language;
  if (chosen && chosen !== "und") track.language = chosen;
  return track;
}

// ── The file ────────────────────────────────────────────────

export class MatroskaFile {
  private cues?: Promise<MkvCuePoint[]>;

  private constructor(
    private readonly reader: RangeReader,
    readonly info: MkvInfo,
  ) {}

  /** Reads the headers and track list. Throws when the data is not Matroska or WebM. */
  static async open(reader: RangeReader): Promise<MatroskaFile> {
    const head = await reader.read(0, MAX_HEADER);
    const ebml = parseHeader(head);
    if (!ebml || ebml.id !== ID.EBML || ebml.size === UNKNOWN_SIZE) {
      throw new Error("Not a Matroska file");
    }
    let at = ebml.headerLength + ebml.size;
    const segmentHeader = parseHeader(await reader.read(at, MAX_HEADER));
    if (!segmentHeader || segmentHeader.id !== ID.Segment) throw new Error("Missing Segment");
    const segmentOffset = at + segmentHeader.headerLength;
    const segmentEnd =
      segmentHeader.size === UNKNOWN_SIZE
        ? Number.POSITIVE_INFINITY
        : segmentOffset + segmentHeader.size;

    const info: MkvInfo = {
      size: reader.size ?? 0,
      segmentOffset,
      secondsPerTick: 1e-3, // default TimestampScale of 1,000,000 ns
      tracks: [],
    };
    let seekCues: number | undefined;
    at = segmentOffset;
    // Walk the top-level elements up to the first cluster. Big elements we do not need
    // (attachments, tags) are skipped by offset without being read.
    for (let guard = 0; guard < 1000 && at < segmentEnd; guard++) {
      const header = parseHeader(await reader.read(at, MAX_HEADER));
      if (!header) break;
      const payloadAt = at + header.headerLength;
      if (header.id === ID.Cluster) {
        info.firstClusterOffset = at;
        break;
      }
      if (header.size === UNKNOWN_SIZE) break;
      if (header.id === ID.SeekHead || header.id === ID.Info || header.id === ID.Tracks) {
        if (header.size > MAX_MASTER_BYTES) throw new Error("Header element too large");
        const payload = await reader.read(payloadAt, header.size);
        if (payload.length < header.size) throw new Error("Truncated header element");
        if (header.id === ID.SeekHead) {
          for (const seek of children(payload).filter((c) => c.id === ID.Seek)) {
            const entries = children(seek.data);
            const target = entries.find((c) => c.id === ID.SeekID);
            const position = entries.find((c) => c.id === ID.SeekPosition);
            if (target && position && toUint(target.data) === ID.Cues) {
              seekCues = segmentOffset + toUint(position.data);
            }
          }
        } else if (header.id === ID.Info) {
          let scale = 1_000_000;
          let duration: number | undefined;
          for (const child of children(payload)) {
            if (child.id === ID.TimestampScale) scale = toUint(child.data) || scale;
            if (child.id === ID.Duration) duration = toFloat(child.data);
          }
          info.secondsPerTick = scale / 1e9;
          if (duration !== undefined && Number.isFinite(duration)) {
            info.durationSeconds = duration * info.secondsPerTick;
          }
        } else {
          for (const entry of children(payload).filter((c) => c.id === ID.TrackEntry)) {
            const track = parseTrackEntry(entry.data);
            if (track) info.tracks.push(track);
          }
        }
      } else if (header.id === ID.Cues) {
        seekCues ??= at;
      }
      at = payloadAt + header.size;
    }
    info.cuesOffset = seekCues;
    info.size = reader.size ?? info.size;
    return new MatroskaFile(reader, info);
  }

  /** Cue points sorted by time. Empty when the file has none. */
  loadCues(): Promise<MkvCuePoint[]> {
    this.cues ??= this.readCues().catch(() => []);
    return this.cues;
  }

  private async readCues(): Promise<MkvCuePoint[]> {
    const offset = this.info.cuesOffset;
    if (offset === undefined) return [];
    const header = parseHeader(await this.reader.read(offset, MAX_HEADER));
    if (!header || header.id !== ID.Cues || header.size === UNKNOWN_SIZE) return [];
    if (header.size > MAX_MASTER_BYTES) return [];
    const payload = await this.reader.read(offset + header.headerLength, header.size);
    if (payload.length < header.size) return [];
    const points: MkvCuePoint[] = [];
    for (const point of children(payload).filter((c) => c.id === ID.CuePoint)) {
      let time: number | undefined;
      let clusterPosition: number | undefined;
      for (const child of children(point.data)) {
        if (child.id === ID.CueTime) time = toUint(child.data);
        if (child.id === ID.CueTrackPositions && clusterPosition === undefined) {
          const position = children(child.data).find((c) => c.id === ID.CueClusterPosition);
          if (position) clusterPosition = toUint(position.data);
        }
      }
      if (time !== undefined && clusterPosition !== undefined) {
        points.push({
          time: time * this.info.secondsPerTick,
          clusterOffset: this.info.segmentOffset + clusterPosition,
        });
      }
    }
    return points.sort((a, b) => a.time - b.time);
  }

  /**
   * Offset of the last cluster that starts at or before `time`, from the cue points. Falls back
   * to the first cluster, so a missing index only costs speed, never correctness.
   */
  async clusterOffsetFor(time: number): Promise<number | undefined> {
    const cues = await this.loadCues();
    let best: number | undefined;
    for (const point of cues) {
      if (point.time > time) break;
      best = point.clusterOffset;
    }
    return best ?? this.info.firstClusterOffset;
  }

  /**
   * Walks clusters from `offset`, reporting the blocks that belong to `wanted` tracks. Stops
   * after `onCluster` returns false, at the end of the file, or when `signal` aborts.
   *
   * `onCluster` receives each cluster's start time before its blocks are read, so the caller can
   * stop once it has seen a time beyond what it needs.
   */
  async scan(
    offset: number,
    wanted: ReadonlySet<number>,
    handlers: {
      onBlock: (block: MkvBlock) => void;
      onCluster?: (time: number) => boolean | Promise<boolean>;
      signal?: AbortSignal;
    },
  ): Promise<{ endOffset: number; reachedEnd: boolean }> {
    const { onBlock, onCluster, signal } = handlers;
    const fileEnd = this.reader.size ?? Number.POSITIVE_INFINITY;
    let at = offset;
    while (at < fileEnd) {
      signal?.throwIfAborted();
      const header = parseHeader(await this.reader.read(at, MAX_HEADER));
      if (!header) return { endOffset: at, reachedEnd: true };
      const payloadAt = at + header.headerLength;
      if (header.id !== ID.Cluster) {
        if (header.size === UNKNOWN_SIZE) return { endOffset: at, reachedEnd: true };
        at = payloadAt + header.size;
        continue;
      }
      const clusterEnd =
        header.size === UNKNOWN_SIZE ? fileEnd : Math.min(fileEnd, payloadAt + header.size);
      const next = await this.scanCluster(
        payloadAt,
        clusterEnd,
        wanted,
        onBlock,
        onCluster,
        signal,
      );
      if (next === undefined) return { endOffset: at, reachedEnd: false };
      at = next;
    }
    return { endOffset: at, reachedEnd: true };
  }

  /** Returns the offset after the cluster, or undefined when `onCluster` asked to stop. */
  private async scanCluster(
    start: number,
    end: number,
    wanted: ReadonlySet<number>,
    onBlock: (block: MkvBlock) => void,
    onCluster: ((time: number) => boolean | Promise<boolean>) | undefined,
    signal: AbortSignal | undefined,
  ): Promise<number | undefined> {
    let at = start;
    let clusterTicks = 0;
    let announced = false;
    const announce = async () => {
      announced = true;
      if (onCluster) return onCluster(clusterTicks * this.info.secondsPerTick);
      return true;
    };
    while (at < end) {
      signal?.throwIfAborted();
      const header = parseHeader(await this.reader.read(at, MAX_HEADER));
      if (!header) return end;
      // A cluster of unknown size ends at the next top-level element.
      if (TOP_LEVEL_IDS.has(header.id)) {
        if (!announced && !(await announce())) return undefined;
        return at;
      }
      if (header.size === UNKNOWN_SIZE) return end;
      const payloadAt = at + header.headerLength;
      const next = payloadAt + header.size;
      if (header.id === ID.Timestamp) {
        if (header.size <= MAX_UINT_BYTES) {
          clusterTicks = toUint(await this.reader.read(payloadAt, header.size));
        }
      } else if (header.id === ID.SimpleBlock || header.id === ID.BlockGroup) {
        // The cluster timestamp precedes its blocks (other children, like a CRC-32 or a Void
        // element, may come first), so announce the cluster when the first block turns up.
        if (!announced && !(await announce())) return undefined;
        if (header.id === ID.SimpleBlock) {
          await this.readBlock(payloadAt, header.size, clusterTicks, undefined, wanted, onBlock);
        } else {
          await this.readBlockGroup(payloadAt, next, clusterTicks, wanted, onBlock);
        }
      }
      at = next;
    }
    if (!announced && !(await announce())) return undefined;
    return end;
  }

  private async readBlockGroup(
    start: number,
    end: number,
    clusterTicks: number,
    wanted: ReadonlySet<number>,
    onBlock: (block: MkvBlock) => void,
  ) {
    let at = start;
    let blockAt = -1;
    let blockSize = 0;
    let durationTicks: number | undefined;
    while (at < end) {
      const header = parseHeader(await this.reader.read(at, MAX_HEADER));
      if (!header || header.size === UNKNOWN_SIZE) return;
      const payloadAt = at + header.headerLength;
      if (header.id === ID.Block) {
        blockAt = payloadAt;
        blockSize = header.size;
      } else if (header.id === ID.BlockDuration && header.size <= MAX_UINT_BYTES) {
        durationTicks = toUint(await this.reader.read(payloadAt, header.size));
      }
      at = payloadAt + header.size;
    }
    if (blockAt >= 0) {
      await this.readBlock(blockAt, blockSize, clusterTicks, durationTicks, wanted, onBlock);
    }
  }

  private async readBlock(
    at: number,
    size: number,
    clusterTicks: number,
    durationTicks: number | undefined,
    wanted: ReadonlySet<number>,
    onBlock: (block: MkvBlock) => void,
  ) {
    const head = await this.reader.read(at, 4 + 8);
    const track = readSize(head, 0);
    if (!track || track.size < 0 || !wanted.has(track.size)) return;
    const prefix = track.length + 3; // track number, int16 timecode, flags
    if (size < prefix || head.length < prefix || size > MAX_TEXT_BLOCK_BYTES) return;
    const flags = head[track.length + 2];
    if ((flags >> 1) & 3) return; // laced blocks do not occur for the text tracks we read
    const relative = new DataView(head.buffer, head.byteOffset + track.length, 2).getInt16(0);
    const data = await this.reader.read(at + prefix, size - prefix);
    onBlock({
      track: track.size,
      time: (clusterTicks + relative) * this.info.secondsPerTick,
      duration: durationTicks === undefined ? undefined : durationTicks * this.info.secondsPerTick,
      data,
    });
  }
}
