import { useCallback, useSyncExternalStore } from "react";

// Settings of the web video player that are kept in this browser.
//
// "Embedded tracks" lets the player read the audio tracks and subtitles stored inside .mkv and
// .webm files (see the vplayer `embeddedTracks` option). It is off unless the viewer turns it on,
// and while it is off nothing about it is loaded or requested.

const EMBEDDED_TRACKS_KEY = "teldrive-player-embedded-tracks";

function readEmbeddedTracks(): boolean {
  try {
    return window.localStorage.getItem(EMBEDDED_TRACKS_KEY) === "true";
  } catch {
    // Private mode and similar: the setting simply stays off.
    return false;
  }
}

const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  // Other tabs change the stored value without telling this one.
  const onStorage = (event: StorageEvent) => {
    if (event.key === EMBEDDED_TRACKS_KEY || event.key === null) listener();
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", onStorage);
  };
}

export function setEmbeddedTracks(enabled: boolean) {
  try {
    if (enabled) window.localStorage.setItem(EMBEDDED_TRACKS_KEY, "true");
    else window.localStorage.removeItem(EMBEDDED_TRACKS_KEY);
  } catch {
    // The choice still applies until the page is reloaded.
  }
  for (const listener of listeners) listener();
}

/** Whether the player reads the tracks stored inside video files. Off by default. */
export function useEmbeddedTracks(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, readEmbeddedTracks, () => false);
  const set = useCallback((next: boolean) => setEmbeddedTracks(next), []);
  return [enabled, set];
}
