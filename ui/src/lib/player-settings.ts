import { useCallback } from "react";
import { fetchClient } from "@/api/client";
import { unwrap } from "@/api/errors";
import type { UserPreferences } from "@/api/types";
import { getQueryClient } from "@/lib/queryClient";
import { preferencesQueryKey, usePreferences, useUpdatePreferences } from "./preferences";

// Settings of the web video player. They are account preferences, like the theme and the language,
// so they follow the account across browsers and devices.
//
// "Embedded tracks" lets the player read the audio tracks and subtitles stored inside .mkv and
// .webm files (see the vplayer `embeddedTracks` option). It is off unless it is turned on, and
// while it is off nothing about it is loaded or requested.

/** Whether the player reads the tracks stored inside video files, and a way to change that. */
export function useEmbeddedTracks(): [boolean, (enabled: boolean) => void] {
  // Cache-only read: the signed-in routes load the preferences before anything renders. A public
  // share has none, and says what the owner allows on its own.
  const enabled = usePreferences()?.appearance.embeddedTracks === true;
  const { mutate } = useUpdatePreferences();
  const set = useCallback(
    (next: boolean) => mutate({ appearance: { embeddedTracks: next } }),
    [mutate],
  );
  return [enabled, set];
}

// Before this was an account setting it was kept in this browser only.
const LEGACY_EMBEDDED_TRACKS_KEY = "teldrive-player-embedded-tracks";

function readLegacy(): string | undefined {
  try {
    return window.localStorage.getItem(LEGACY_EMBEDDED_TRACKS_KEY) ?? undefined;
  } catch {
    return undefined;
  }
}

function removeLegacy() {
  try {
    window.localStorage.removeItem(LEGACY_EMBEDDED_TRACKS_KEY);
  } catch {
    // Storage can be unavailable; nothing reads the key any more.
  }
}

// One-time, per-browser pickup of the value this browser had, so turning it on is not lost. Runs
// after the preferences load. A failure keeps the key so the next load tries again; it never blocks
// the app.
export async function importLegacyEmbeddedTracks(): Promise<void> {
  const legacy = readLegacy();
  if (legacy === undefined) return;
  const queryClient = getQueryClient();
  const current = queryClient.getQueryData<UserPreferences>(preferencesQueryKey);
  if (!current) return;
  if (legacy === "true" && !current.appearance.embeddedTracks) {
    try {
      const saved = await unwrap(
        fetchClient.PATCH("/v1/me/preferences", { body: { appearance: { embeddedTracks: true } } }),
      );
      queryClient.setQueryData(preferencesQueryKey, saved);
    } catch {
      return;
    }
  }
  removeLegacy();
}
