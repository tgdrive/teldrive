// Player plugin that adds the tracks stored inside a video file to the web player:
//
//   - an "Audio" button listing the audio tracks (shown when there is more than one);
//   - the embedded subtitle tracks, which appear in the player's own subtitle menu next to
//     "load from file" and are drawn with the player's caption settings.
//
// The track data comes from an EmbeddedTracks controller provided by VideoViewer.

import {
  type PlayerPlugin,
  type PluginAPI,
  type SubtitleTrack,
  usePlayerContext,
  usePlayerRemote,
  usePlayerState,
} from "@vplayer/react";
import {
  type CSSProperties,
  createContext,
  type KeyboardEvent,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { useAppLocale } from "@/lib/locale";
import CheckIcon from "~icons/gravity-ui/check";
import HeadphonesIcon from "~icons/gravity-ui/headphones";
import {
  type EmbeddedTracks,
  embeddedSubtitleNumber,
  type SubtitleChoice,
  type TracksSnapshot,
} from "./embedded-tracks";
import type { TextCue } from "./subtitle-text";

const TracksContext = createContext<EmbeddedTracks | null>(null);
export const TracksProvider = TracksContext.Provider;

function useTracks(): { tracks: EmbeddedTracks; snapshot: TracksSnapshot } {
  const tracks = useContext(TracksContext);
  if (!tracks) throw new Error("TracksProvider is missing");
  const snapshot = useSyncExternalStore(tracks.subscribe, tracks.getSnapshot);
  return { tracks, snapshot };
}

// The player parses a subtitle track when it is picked and rejects one without any cues. These
// stand-ins carry a single cue that is never reached (99 hours in); the real cues are read from
// the file and drawn by EmbeddedCaptions.
const PLACEHOLDER = "WEBVTT\n\n99:59:59.000 --> 99:59:59.500\n​\n";

/** The embedded tracks as entries for the player's subtitle menu. */
export function subtitleEntries(
  choices: SubtitleChoice[],
  imageLabel: (label: string) => string,
): SubtitleTrack[] {
  return choices.map((choice) => ({
    id: choice.id,
    lang: choice.language ?? "und",
    label: choice.supported ? choice.label : imageLabel(choice.label),
    content: PLACEHOLDER,
    format: "vtt",
    default: choice.isDefault,
  }));
}

export const embeddedTracksPlugin: PlayerPlugin = {
  name: "teldrive-embedded-tracks",
  setup(api) {
    const removeControl = api.addControl({
      name: "teldrive-audio-tracks",
      position: "right",
      index: 0,
      render: AudioTracksControl,
    });
    const removeLayer = api.addLayer({
      name: "teldrive-embedded-captions",
      render: EmbeddedCaptions,
    });
    return () => {
      removeControl();
      removeLayer();
    };
  },
};

// ── Audio ─────────────────────────────────────────────────

function AudioTracksControl({ api }: { api: PluginAPI }) {
  const { tracks, snapshot } = useTracks();
  const { t } = useAppLocale();
  const { controlsVisibility } = usePlayerContext();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const [maxHeight, setMaxHeight] = useState<number>();

  // The menu opens upwards inside the player, which can be small (a phone in portrait): limit it
  // to the room above the controls and let it scroll.
  useLayoutEffect(() => {
    if (!open) return;
    const fit = () => {
      const container = api.context.containerRef.current;
      const button = trigger.current;
      if (!container || !button) return;
      const room = button.getBoundingClientRect().top - container.getBoundingClientRect().top - 16;
      setMaxHeight(Math.max(96, Math.floor(room)));
    };
    fit();
    window.addEventListener("resize", fit);
    return () => window.removeEventListener("resize", fit);
  }, [open, api]);

  // Keep the player's controls on screen while the menu is open.
  useEffect(() => {
    if (!open) return;
    return controlsVisibility.pinControls();
  }, [open, controlsVisibility]);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    return () => document.removeEventListener("pointerdown", close);
  }, [open]);

  useEffect(() => {
    if (open) root.current?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus();
  }, [open]);

  if (snapshot.audio.length < 2) return null;

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // The player listens for arrow keys on the page (volume, seeking); keep them in the menu.
    event.stopPropagation();
    if (event.key === "Escape") {
      setOpen(false);
      trigger.current?.focus();
      return;
    }
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    event.preventDefault();
    const items = [
      ...event.currentTarget.querySelectorAll<HTMLElement>("[role=menuitemradio]:not(:disabled)"),
    ];
    const at = items.indexOf(document.activeElement as HTMLElement);
    const next = event.key === "ArrowDown" ? at + 1 : at - 1;
    items[(next + items.length) % items.length]?.focus();
  };

  const choose = async (id: number) => {
    setOpen(false);
    trigger.current?.focus();
    if (!(await tracks.selectAudio(id))) api.notify(t("player.audioSwitchFailed"), 4000);
  };

  return (
    <div ref={root} style={{ position: "relative", pointerEvents: "auto" }}>
      <button
        ref={trigger}
        type="button"
        className="vplayer__button vplayer__button--trigger"
        aria-label={t("player.audio")}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-busy={snapshot.audioBusy}
        onClick={() => setOpen((value) => !value)}
      >
        <HeadphonesIcon width={18} height={18} />
      </button>
      {open ? (
        <div
          role="menu"
          aria-label={t("player.audioTracks")}
          className="vplayer__menu-popover vplayer__menu"
          style={{
            position: "absolute",
            right: 0,
            bottom: "calc(100% + 0.5rem)",
            maxHeight,
            overflowY: "auto",
          }}
          onKeyDown={onKeyDown}
        >
          {snapshot.audio.map((track) => {
            const selected = track.id === snapshot.activeAudio;
            const unsupported = track.mode === "unsupported";
            return (
              <button
                key={track.id}
                type="button"
                role="menuitemradio"
                aria-checked={selected}
                disabled={unsupported || snapshot.audioBusy}
                className="vplayer__menu-item"
                style={ITEM_STYLE}
                onClick={() => void choose(track.id)}
              >
                <span
                  className="vplayer__menu-label"
                  style={{ color: selected ? "inherit" : undefined }}
                >
                  {track.label}
                </span>
                <span className="vplayer__menu-value" style={{ whiteSpace: "nowrap" }}>
                  {unsupported ? t("player.audioUnsupported") : track.detail}
                </span>
                <span style={{ width: 14, flex: "none" }}>
                  {selected ? (
                    <CheckIcon width={14} height={14} className="vplayer__menu-check" />
                  ) : null}
                </span>
              </button>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}

const ITEM_STYLE: CSSProperties = {
  display: "flex",
  textAlign: "left",
  background: "transparent",
  border: 0,
  color: "inherit",
  font: "inherit",
  cursor: "pointer",
};

// ── Captions ──────────────────────────────────────────────

/** The cue on screen at `time`. Cues are sorted and do not overlap, so this is a binary search. */
function cueAt(cues: readonly TextCue[], time: number): TextCue | undefined {
  let low = 0;
  let high = cues.length - 1;
  while (low <= high) {
    const middle = (low + high) >> 1;
    const cue = cues[middle];
    if (time < cue.start) high = middle - 1;
    else if (time >= cue.end) low = middle + 1;
    else return cue;
  }
  return undefined;
}

const FONT_FAMILY = {
  sans: "ui-sans-serif, system-ui, sans-serif",
  serif: "Georgia, Cambria, serif",
  monospace: "ui-monospace, SFMono-Regular, monospace",
} as const;

// Mirrors the player's own caption sizing so embedded subtitles look like loaded ones.
function scaledFontSize(size: "small" | "medium" | "large", scale: number): string {
  const multiplier = scale / 100;
  if (size === "small") {
    return `clamp(${0.75 * multiplier}rem, ${1.7 * multiplier}vw, ${multiplier}rem)`;
  }
  if (size === "large") {
    return `clamp(${1.2 * multiplier}rem, ${2.8 * multiplier}vw, ${1.75 * multiplier}rem)`;
  }
  return `clamp(${0.95 * multiplier}rem, ${2.2 * multiplier}vw, ${1.35 * multiplier}rem)`;
}

function EmbeddedCaptions({ api }: { api: PluginAPI }) {
  const { tracks, snapshot } = useTracks();
  const { t } = useAppLocale();
  const remote = usePlayerRemote();
  const active = usePlayerState("activeSubtitle");
  const currentTime = usePlayerState("currentTime");
  const settings = usePlayerState("captionSettings");
  const number = embeddedSubtitleNumber(active?.id);
  const previous = useRef<number | undefined>(undefined);

  // Hand the <video> to the track controller, and resume where playback was if it had to switch
  // to the plain file.
  useEffect(() => {
    const video = api.context.videoRef.current;
    tracks.attachVideo(video);
    if (!video) return;
    const resume = () => {
      const time = tracks.takeResumeTime();
      if (time) video.currentTime = time;
    };
    video.addEventListener("loadedmetadata", resume);
    return () => video.removeEventListener("loadedmetadata", resume);
  }, [api, tracks]);

  // Follow the player's subtitle selection.
  useEffect(() => {
    const choice = snapshot.subtitles.find((track) => track.number === number);
    if (number !== undefined && choice && !choice.supported) {
      api.notify(t("player.subtitleImage", { label: choice.label }), 5000);
      remote.setActiveSubtitle(null);
      return;
    }
    tracks.selectSubtitle(number);
    if (number !== previous.current) {
      // Picking an embedded track, or switching them off, is remembered for the next video.
      // Choosing a subtitle file is not an embedded track and leaves the memory alone.
      if (number !== undefined) tracks.rememberSubtitle(number);
      else if (previous.current !== undefined && !active) tracks.rememberSubtitle(undefined);
      previous.current = number;
    }
  }, [number, active, tracks, snapshot.subtitles]);

  useEffect(() => {
    tracks.setPlayhead(currentTime);
  }, [currentTime, tracks]);

  if (number === undefined) return null;
  const cue = cueAt(snapshot.cues, currentTime - settings.delay);
  if (!cue) return null;

  const edge =
    settings.edgeStyle === "outline"
      ? `-1px -1px 0 ${settings.edgeColor}, 1px -1px 0 ${settings.edgeColor}, -1px 1px 0 ${settings.edgeColor}, 1px 1px 0 ${settings.edgeColor}`
      : settings.edgeStyle === "shadow"
        ? `0 2px 3px ${settings.edgeColor}`
        : "none";
  const style = {
    "--vplayer-caption-size": scaledFontSize(settings.fontSize, settings.fontScale),
    "--vplayer-caption-font": FONT_FAMILY[settings.fontFamily],
    "--vplayer-caption-color": `color-mix(in srgb, ${settings.textColor} ${Math.round(settings.textOpacity * 100)}%, transparent)`,
    "--vplayer-caption-background": `color-mix(in srgb, ${settings.backgroundColor} ${Math.round(settings.backgroundOpacity * 100)}%, transparent)`,
    "--vplayer-caption-position": `calc(5rem + ${settings.position}vh)`,
    "--vplayer-caption-line-height": settings.lineHeight,
    "--vplayer-caption-edge": edge,
  } as CSSProperties;

  return (
    <div className="vplayer__captions" style={style} aria-hidden="true" data-embedded-caption>
      <span className="vplayer__caption-cue">{cue.text}</span>
    </div>
  );
}
