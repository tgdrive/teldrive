// Human-readable names for audio and subtitle tracks.

// Containers tag languages with ISO 639-2, which has a "bibliographic" and a "terminology" form
// for some languages (fre / fra). Intl only knows the terminology form, so map the others.
const BIBLIOGRAPHIC: Record<string, string> = {
  alb: "sqi",
  arm: "hye",
  baq: "eus",
  bur: "mya",
  chi: "zho",
  cze: "ces",
  dut: "nld",
  fre: "fra",
  geo: "kat",
  ger: "deu",
  gre: "ell",
  ice: "isl",
  mac: "mkd",
  mao: "mri",
  may: "msa",
  per: "fas",
  rum: "ron",
  slo: "slk",
  tib: "bod",
  wel: "cym",
};

/** Canonical language key for comparing tags such as "fre", "fra", "fr" and "fr-CA". */
export function languageKey(code: string | undefined): string | undefined {
  if (!code) return undefined;
  const lower = code.toLowerCase().split(/[-_]/)[0];
  if (!lower || lower === "und" || lower === "zxx") return undefined;
  return BIBLIOGRAPHIC[lower] ?? lower;
}

const displayNames = new Map<string, Intl.DisplayNames | null>();

/** The language's name in the viewer's language, e.g. "Japanese", or undefined if unknown. */
export function languageName(code: string | undefined, locale: string): string | undefined {
  const key = languageKey(code);
  if (!key) return undefined;
  let names = displayNames.get(locale);
  if (names === undefined) {
    try {
      names = new Intl.DisplayNames([locale], { type: "language", fallback: "none" });
    } catch {
      names = null;
    }
    displayNames.set(locale, names);
  }
  try {
    return names?.of(key) ?? undefined;
  } catch {
    return undefined;
  }
}

const AUDIO_CODEC_NAMES: Record<string, string> = {
  aac: "AAC",
  ac3: "AC-3",
  eac3: "E-AC-3",
  dts: "DTS",
  flac: "FLAC",
  mp3: "MP3",
  opus: "Opus",
  vorbis: "Vorbis",
  truehd: "TrueHD",
};

function channelLabel(channels: number): string | undefined {
  if (channels === 1) return "Mono";
  if (channels === 2) return "Stereo";
  if (channels === 6) return "5.1";
  if (channels === 8) return "7.1";
  return channels > 0 ? `${channels}ch` : undefined;
}

/** "E-AC-3 5.1" */
export function audioDetail(codec: string, channels: number): string {
  return [AUDIO_CODEC_NAMES[codec] ?? codec.toUpperCase(), channelLabel(channels)]
    .filter(Boolean)
    .join(" ");
}

/**
 * The title the file gives the track, or else the language's name, or else "Track N".
 * When the file's title already says the language, it is not repeated.
 */
export function trackTitle(
  track: { name?: string; language?: string },
  index: number,
  locale: string,
  fallback: (number: number) => string,
): string {
  const language = languageName(track.language, locale);
  if (track.name) {
    if (!language || track.name.toLowerCase().includes(language.toLowerCase())) return track.name;
    return `${track.name} (${language})`;
  }
  return language ?? fallback(index + 1);
}
