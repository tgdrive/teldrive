// Turns the subtitle blocks stored in a container into the plain timed text the player renders.
//
// The player shows one caption at a time as plain text, styled by the viewer's caption settings,
// so styling and positioning stored in the file (ASS overrides, SRT tags) is removed and only the
// words are kept. Bitmap subtitle formats are not text and are reported as unsupported.

import type { MkvBlock, MkvTrack } from "./matroska";

export type TextCue = { start: number; end: number; text: string };

export type SubtitleFormat = "srt" | "ass" | "webvtt";

/** Codec IDs of text subtitle tracks we can show, mapped to how their blocks are laid out. */
const TEXT_CODECS: Record<string, SubtitleFormat> = {
  "S_TEXT/UTF8": "srt",
  "S_TEXT/ASCII": "srt",
  "S_TEXT/SSA": "ass",
  "S_TEXT/ASS": "ass",
  S_SSA: "ass",
  S_ASS: "ass",
  "S_TEXT/WEBVTT": "webvtt",
};

export function subtitleFormat(track: Pick<MkvTrack, "codecId">): SubtitleFormat | undefined {
  return TEXT_CODECS[track.codecId];
}

/** Cues shown when a block carries no explicit duration (rare, but legal). */
const DEFAULT_DURATION = 4;

const ENTITIES: Record<string, string> = {
  "&amp;": "&",
  "&lt;": "<",
  "&gt;": ">",
  "&quot;": '"',
  "&apos;": "'",
  "&nbsp;": " ",
  "&lrm;": "‎",
  "&rlm;": "‏",
};

function decodeEntities(value: string): string {
  return value.replace(
    /&(?:amp|lt|gt|quot|apos|nbsp|lrm|rlm);/g,
    (entity) => ENTITIES[entity] ?? entity,
  );
}

/** Removes `<i>`, `<font ...>` and similar markup used by SRT and WebVTT. */
function stripMarkup(value: string): string {
  return decodeEntities(value.replace(/<\/?[a-zA-Z][^>]*>/g, "")).replace(/\{\\[^}]*\}/g, "");
}

function tidy(value: string): string {
  return value
    .split("\n")
    .map((line) => line.replace(/[ \t]+/g, " ").trim())
    .filter(Boolean)
    .join("\n");
}

/**
 * Text of an ASS/SSA "Dialogue" event as stored in Matroska:
 * `ReadOrder,Layer,Style,Name,MarginL,MarginR,MarginV,Effect,Text`.
 * Returns undefined for events that are not text (vector drawings) or are empty.
 */
export function assEventText(line: string): string | undefined {
  let commas = 0;
  let index = -1;
  for (let i = 0; i < line.length && commas < 8; i++) {
    if (line[i] === ",") {
      commas++;
      index = i;
    }
  }
  if (commas < 8) return undefined;
  const raw = line.slice(index + 1);
  // `{\p1}...{\p0}` switches the text to a vector drawing: nothing readable.
  if (/\{[^}]*\\p[1-9]/.test(raw)) return undefined;
  const text = raw
    .replace(/\{[^}]*\}/g, "")
    .replace(/\\[Nn]/g, "\n")
    .replace(/\\h/g, " ");
  return tidy(text) || undefined;
}

/** Converts one container block to a cue, or undefined when it holds no text. */
export function blockToCue(block: MkvBlock, format: SubtitleFormat): TextCue | undefined {
  const body = new TextDecoder().decode(block.data).replace(/\r\n?/g, "\n");
  let text: string | undefined;
  if (format === "ass") text = assEventText(body);
  else text = tidy(stripMarkup(body)) || undefined; // SRT and WebVTT: the block is the cue payload
  if (!text) return undefined;
  const duration = block.duration && block.duration > 0 ? block.duration : DEFAULT_DURATION;
  return { start: block.time, end: block.time + duration, text };
}

/**
 * The player looks up the active caption by binary search, so cues must be sorted and must not
 * overlap. Overlapping cues (a sign and a line of dialogue, say) are combined into one cue per
 * interval that lists every line that is on screen, in the order the lines started.
 */
export function mergeOverlapping(cues: readonly TextCue[]): TextCue[] {
  const sorted = [...cues].sort((a, b) => a.start - b.start || a.end - b.end);
  const boundaries = new Set<number>();
  for (const cue of sorted) {
    boundaries.add(cue.start);
    boundaries.add(cue.end);
  }
  const points = [...boundaries].sort((a, b) => a - b);
  const out: TextCue[] = [];
  let active: TextCue[] = [];
  let next = 0;
  for (let i = 0; i < points.length - 1; i++) {
    const from = points[i];
    const to = points[i + 1];
    while (next < sorted.length && sorted[next].start <= from) active.push(sorted[next++]);
    active = active.filter((cue) => cue.end > from);
    if (active.length === 0) continue;
    const lines: string[] = [];
    for (const cue of active) if (!lines.includes(cue.text)) lines.push(cue.text);
    const text = lines.join("\n");
    const previous = out[out.length - 1];
    // Join consecutive intervals that show the same text.
    if (previous && previous.end === from && previous.text === text) previous.end = to;
    else out.push({ start: from, end: to, text });
  }
  return out;
}
