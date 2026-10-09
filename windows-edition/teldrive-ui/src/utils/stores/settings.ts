import { create } from "zustand";
import { persist } from "zustand/middleware";
import { immer } from "zustand/middleware/immer";

import type { Settings } from "@/config/settings";
import { getSettingsValues } from "@/config/settings";

const settings = getSettingsValues();

export interface SettingsState {
  settings: Settings;
  updateSetting: <K extends keyof Settings>(key: K, value: Settings[K]) => void;
  updateSettings: (newSettings: Partial<Settings>) => void;
  resetSettings: () => void;
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    immer((set) => ({
      settings,
      updateSetting: <K extends keyof Settings>(key: K, value: Settings[K]) =>
        set((state) => {
          state.settings[key] = value;
        }),
      updateSettings: (newSettings) =>
        set((state) => {
          Object.assign(state.settings, newSettings);
        }),
      resetSettings: () =>
        set((state) => {
          state.settings = { ...settings };
        }),
    })),
    {
      name: "teldrive-settings",
      merge: (persistedState: any, currentState) => {
        const defaultSettings = getSettingsValues();
        const mergedSettings = { ...defaultSettings };

        if (persistedState && typeof persistedState.settings === "object") {
          for (const key in persistedState.settings) {
            const value = persistedState.settings[key];
            if (value !== undefined && value !== null && value !== "") {
              mergedSettings[key as keyof Settings] = value;
            }
          }
        }

        return {
          ...currentState,
          settings: mergedSettings,
        };
      },
    },
  ),
);
