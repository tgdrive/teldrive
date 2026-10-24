import { useCallback } from "react";
import { usePreferences, useUpdatePreferences } from "./preferences";

// Account preferences; there is deliberately no import from old browser storage.
export function useEmbeddedTracks(): [boolean, (enabled: boolean) => void] {
  const enabled = usePreferences()?.appearance.embeddedTracks === true;
  const { mutate } = useUpdatePreferences();
  const set = useCallback(
    (next: boolean) => mutate({ appearance: { embeddedTracks: next } }),
    [mutate],
  );
  return [enabled, set];
}
