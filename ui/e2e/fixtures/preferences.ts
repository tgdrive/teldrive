import type { Page } from "@playwright/test";
import { DEFAULT_CAPTION_SETTINGS } from "@vplayer/react";
import type { CaptionPreferences } from "../../src/api/types";

const MiB = 1024 * 1024;

export type Preferences = {
  captions: CaptionPreferences;
  uploads: {
    encryption: boolean;
    conflictPolicy: "fail" | "rename" | "replace";
    concurrency: number;
    preferredPartSize: number;
  };
  files: { sort: "name" | "updatedAt" | "size"; order: "asc" | "desc" };
  appearance: {
    theme: "system" | "light" | "dark";
    locale?: string;
    embeddedTracks: boolean;
    embeddedTracksOnShares: boolean;
  };
  updatedAt?: string;
};

export type PreferencesUpdate = {
  captions?: Partial<CaptionPreferences>;
  uploads?: Partial<Preferences["uploads"]>;
  files?: Partial<Preferences["files"]>;
  appearance?: Partial<Preferences["appearance"]>;
};

export type PreferencesSeed = PreferencesUpdate & { updatedAt?: string };

export type PreferencesApi = {
  current: () => Preferences;
  patches: PreferencesUpdate[];
};

// Defaults a fresh account sees: no updatedAt until the user changes something.
function defaultPreferences(): Preferences {
  return {
    captions: { ...DEFAULT_CAPTION_SETTINGS },
    uploads: {
      encryption: false,
      conflictPolicy: "rename",
      concurrency: 3,
      preferredPartSize: 512 * MiB,
    },
    files: { sort: "name", order: "asc" },
    appearance: { theme: "system", embeddedTracks: false, embeddedTracksOnShares: false },
  };
}

// Stateful GET/PATCH for /api/v1/me/preferences. Playwright runs the most recently
// registered matching route first, so call this after any broader `**/api/v1/**` route.
export async function installPreferencesApi(
  page: Page,
  seed: PreferencesSeed = {},
): Promise<PreferencesApi> {
  const defaults = defaultPreferences();
  const state: Preferences = {
    captions: { ...defaults.captions, ...seed.captions },
    uploads: { ...defaults.uploads, ...seed.uploads },
    files: { ...defaults.files, ...seed.files },
    appearance: { ...defaults.appearance, ...seed.appearance },
    updatedAt: seed.updatedAt,
  };
  const patches: PreferencesUpdate[] = [];

  await page.route("**/api/v1/me/preferences", async (route) => {
    const request = route.request();
    if (request.method() === "PATCH") {
      const update = request.postDataJSON() as PreferencesUpdate;
      patches.push(update);
      state.uploads = { ...state.uploads, ...update.uploads };
      state.files = { ...state.files, ...update.files };
      state.appearance = { ...state.appearance, ...update.appearance };
      state.captions = { ...state.captions, ...update.captions };
      // The server's rule: the option for shared links needs the one for the account.
      if (!state.appearance.embeddedTracks) state.appearance.embeddedTracksOnShares = false;
      state.updatedAt = new Date().toISOString();
      return route.fulfill({ json: state });
    }
    if (request.method() === "GET") return route.fulfill({ json: state });
    return route.fallback();
  });

  return { current: () => structuredClone(state), patches };
}
