import { useMemo, useSyncExternalStore } from "react";

// These settings describe this browser's storage/display, not an account on the server.
export const DOCUMENT_SETTINGS_KEY = "teldrive.document-settings.v1";
const changed = "teldrive:document-settings";
export type DocumentSettings = {
  cacheEnabled: boolean;
  cacheLimitMiB: number;
  pdfBrightness: number;
};
const defaults: DocumentSettings = { cacheEnabled: false, cacheLimitMiB: 256, pdfBrightness: 100 };
const initial = JSON.stringify(defaults);
let fallback = initial;
let memoryOnly = false;

function snapshot() {
  try {
    if (!memoryOnly) return localStorage.getItem(DOCUMENT_SETTINGS_KEY) ?? initial;
  } catch {
    /* Private mode may not expose storage. */
  }
  return fallback;
}

function normalize(raw: string): DocumentSettings {
  let value: Partial<DocumentSettings> = {};
  try {
    value = JSON.parse(raw) ?? {};
  } catch {
    /* Use defaults for corrupt settings. */
  }
  return {
    cacheEnabled: value.cacheEnabled === true,
    cacheLimitMiB:
      typeof value.cacheLimitMiB === "number" && Number.isFinite(value.cacheLimitMiB)
        ? Math.max(32, Math.min(1024, Math.round(value.cacheLimitMiB)))
        : defaults.cacheLimitMiB,
    pdfBrightness:
      typeof value.pdfBrightness === "number" && Number.isFinite(value.pdfBrightness)
        ? Math.max(50, Math.min(150, Math.round(value.pdfBrightness / 5) * 5))
        : defaults.pdfBrightness,
  };
}

export function getDocumentSettings() {
  return normalize(snapshot());
}

export function updateDocumentSettings(patch: Partial<DocumentSettings>) {
  fallback = JSON.stringify(normalize(JSON.stringify({ ...getDocumentSettings(), ...patch })));
  try {
    localStorage.setItem(DOCUMENT_SETTINGS_KEY, fallback);
  } catch {
    memoryOnly = true;
  }
  window.dispatchEvent(new Event(changed));
}

function subscribe(listener: () => void) {
  const storage = (event: StorageEvent) => {
    if (event.key === DOCUMENT_SETTINGS_KEY || event.key === null) {
      memoryOnly = false;
      listener();
    }
  };
  window.addEventListener(changed, listener);
  window.addEventListener("storage", storage);
  return () => {
    window.removeEventListener(changed, listener);
    window.removeEventListener("storage", storage);
  };
}

export function useDocumentSettings() {
  const raw = useSyncExternalStore(subscribe, snapshot);
  return useMemo(() => normalize(raw), [raw]);
}
