import { hashKey, queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useSyncExternalStore } from "react";
import { toast } from "sonner";
import { fetchClient } from "@/api/client";
import { unwrap, userMessage } from "@/api/errors";
import type {
  AppearancePreferences,
  FileBrowserPreferences,
  UploadPreferences,
  UserPreferences,
  UserPreferencesUpdate,
} from "@/api/types";
import { validFileSort, validSortOrder } from "@/features/files/sort-preference";
import { isAppLocale, translate } from "@/lib/locales";
import { getQueryClient } from "@/lib/queryClient";

const MIB = 1024 * 1024;
export const DEFAULT_PART_SIZE_MIB = 512;
export const MAX_PART_SIZE_MIB = 2048;
export const PART_SIZE_STEP_MIB = 16;

export function normalizePartSizeMiB(value: number) {
  if (!Number.isFinite(value)) return DEFAULT_PART_SIZE_MIB;
  return Math.max(
    PART_SIZE_STEP_MIB,
    Math.min(MAX_PART_SIZE_MIB, Math.round(value / PART_SIZE_STEP_MIB) * PART_SIZE_STEP_MIB),
  );
}

export const preferencesQueryKey = ["auth", "preferences"] as const;
const preferencesMutationKey = ["auth", "preferences", "update"] as const;

export function preferencesQueryOptions() {
  return queryOptions({
    queryKey: preferencesQueryKey,
    queryFn: () => unwrap(fetchClient.GET("/v1/me/preferences")),
    staleTime: 30_000,
    // Preferences are read by providers and pages that hold no observer, so an idle
    // timer must not evict them. Logout clears the cache explicitly.
    gcTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
}

export function hasCachedPreferences() {
  return getQueryClient().getQueryData(preferencesQueryKey) !== undefined;
}

// Cache-only read: never fetches. Loading happens in the root route's beforeLoad.
export function usePreferences(): UserPreferences | undefined {
  const queryClient = useQueryClient();
  // Only preference events: other queries built during a render would otherwise notify every
  // reader, and React reports the resulting store update as a render-phase setState.
  const subscribe = useCallback(
    (onChange: () => void) =>
      queryClient.getQueryCache().subscribe((event) => {
        if (event.query.queryHash === hashKey(preferencesQueryKey)) onChange();
      }),
    [queryClient],
  );
  return useSyncExternalStore(subscribe, () =>
    queryClient.getQueryData<UserPreferences>(preferencesQueryKey),
  );
}

function mergePreferences(current: UserPreferences, update: UserPreferencesUpdate) {
  return {
    ...current,
    uploads: { ...current.uploads, ...update.uploads },
    files: { ...current.files, ...update.files },
    appearance: { ...current.appearance, ...update.appearance },
  };
}

// Values of `previous` for exactly the fields `changed` names, used to revert one failed patch
// without discarding optimistic changes from other in-flight patches.
function previousValues<T extends object>(previous: T, changed: Partial<T> | undefined) {
  const values: Partial<T> = {};
  for (const key of Object.keys(changed ?? {}) as (keyof T)[]) values[key] = previous[key];
  return values;
}

function displayedLocale() {
  const lang = document.documentElement.lang;
  return isAppLocale(lang) ? lang : "en";
}

// Shared by every caller so concurrent writes coordinate through one mutation key.
export function useUpdatePreferences() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: preferencesMutationKey,
    mutationFn: (patch: UserPreferencesUpdate) =>
      unwrap(fetchClient.PATCH("/v1/me/preferences", { body: patch })),
    onMutate: async (patch) => {
      await queryClient.cancelQueries({ queryKey: preferencesQueryKey });
      const previous = queryClient.getQueryData<UserPreferences>(preferencesQueryKey);
      if (previous) {
        // A local change marks the row as touched so the legacy import never overwrites it.
        queryClient.setQueryData(preferencesQueryKey, {
          ...mergePreferences(previous, patch),
          updatedAt: new Date().toISOString(),
        });
      }
      return { previous };
    },
    onSuccess: (saved) => {
      // Earlier responses can arrive out of order; only the final in-flight write is authoritative.
      if (queryClient.isMutating({ mutationKey: preferencesMutationKey }) === 1) {
        queryClient.setQueryData(preferencesQueryKey, saved);
      }
    },
    onError: (error, patch, context) => {
      const previous = context?.previous;
      if (previous) {
        queryClient.setQueryData<UserPreferences>(preferencesQueryKey, (current) =>
          current
            ? {
                ...mergePreferences(current, {
                  uploads: previousValues<UploadPreferences>(previous.uploads, patch.uploads),
                  files: previousValues<FileBrowserPreferences>(previous.files, patch.files),
                  appearance: previousValues<AppearancePreferences>(
                    previous.appearance,
                    patch.appearance,
                  ),
                }),
                updatedAt: previous.updatedAt,
              }
            : current,
        );
      }
      toast.error(translate(displayedLocale(), "preferences.saveFailed"), {
        description: userMessage(error),
      });
    },
  });
}

const legacyUploadSettingsKey = "teldrive.upload-settings.v2";
const legacyFileSortKey = "file-sort";
const legacyThemeKey = "theme";
const legacyLocaleKey = "teldrive-locale";
// The theme and locale keys stay in use as the first-paint mirror of whichever account signed in
// last, so after the first import they no longer hold browser-only settings. This marker keeps a
// later account on the same browser from importing another account's mirrored values.
const legacyImportDoneKey = "teldrive.preferences-imported.v1";

function readLegacyJson(key: string): Record<string, unknown> {
  try {
    const value: unknown = JSON.parse(window.localStorage.getItem(key) ?? "null");
    return value && typeof value === "object" ? (value as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function readLegacyString(key: string) {
  try {
    return window.localStorage.getItem(key) ?? undefined;
  } catch {
    return undefined;
  }
}

function removeLegacyKeys(keys: string[]) {
  try {
    for (const key of keys) window.localStorage.removeItem(key);
  } catch {
    // Storage can be unavailable; the stale keys are then ignored by every reader.
  }
}

function finishLegacyImport() {
  removeLegacyKeys([legacyUploadSettingsKey, legacyFileSortKey]);
  try {
    window.localStorage.setItem(legacyImportDoneKey, "1");
  } catch {
    // Without storage there are no legacy values to import again.
  }
}

// Older builds stored the API's "fail" policy as "error".
function legacyConflictPolicy(value: unknown): UploadPreferences["conflictPolicy"] | undefined {
  if (value === "error") return "fail";
  if (value === "fail" || value === "replace" || value === "rename") return value;
  return undefined;
}

// Legacy values equal to the server row are dropped: importing them would only mark an
// untouched account as changed (the provider mirrors server values into localStorage).
function changedFields<T extends object>(values: Partial<T>, current: T) {
  const changed: Partial<T> = {};
  for (const key of Object.keys(values) as (keyof T)[]) {
    if (values[key] !== current[key]) changed[key] = values[key];
  }
  return changed;
}

function legacyPreferencesUpdate(current: UserPreferences): UserPreferencesUpdate {
  const update: UserPreferencesUpdate = {};

  const upload = readLegacyJson(legacyUploadSettingsKey);
  const uploads: Partial<UploadPreferences> = {};
  const { encryption, conflictPolicy, concurrency, preferredPartSize } = upload;
  if (typeof encryption === "boolean") uploads.encryption = encryption;
  const policy = legacyConflictPolicy(conflictPolicy);
  if (policy) uploads.conflictPolicy = policy;
  if (typeof concurrency === "number" && Number.isFinite(concurrency)) {
    uploads.concurrency = Math.min(12, Math.max(1, Math.round(concurrency)));
  }
  if (typeof preferredPartSize === "number" && Number.isFinite(preferredPartSize)) {
    uploads.preferredPartSize = normalizePartSizeMiB(preferredPartSize / MIB) * MIB;
  }
  const uploadChanges = changedFields(uploads, current.uploads);
  if (Object.keys(uploadChanges).length > 0) update.uploads = uploadChanges;

  const sort = readLegacyJson(legacyFileSortKey);
  const files: Partial<FileBrowserPreferences> = {};
  const sortField = validFileSort(sort.sort);
  const orderField = validSortOrder(sort.order);
  if (sortField) files.sort = sortField;
  if (orderField) files.order = orderField;
  const fileChanges = changedFields(files, current.files);
  if (Object.keys(fileChanges).length > 0) update.files = fileChanges;

  const appearance: Partial<AppearancePreferences> = {};
  const theme = readLegacyString(legacyThemeKey);
  const locale = readLegacyString(legacyLocaleKey);
  if (theme === "system" || theme === "light" || theme === "dark") appearance.theme = theme;
  if (isAppLocale(locale)) appearance.locale = locale;
  const appearanceChanges = changedFields(appearance, current.appearance);
  if (Object.keys(appearanceChanges).length > 0) update.appearance = appearanceChanges;

  return update;
}

// One-time, per-browser import of browser-only settings into the first account that signs in.
// Runs after preferences load. Failures keep the legacy keys so the next load retries; they never
// block the app.
export async function importLegacyPreferences(): Promise<void> {
  const queryClient = getQueryClient();
  const current = queryClient.getQueryData<UserPreferences>(preferencesQueryKey);
  if (!current || readLegacyString(legacyImportDoneKey)) return;
  if (current.updatedAt) {
    finishLegacyImport();
    return;
  }

  const update = legacyPreferencesUpdate(current);
  if (Object.keys(update).length > 0) {
    try {
      const saved = await unwrap(fetchClient.PATCH("/v1/me/preferences", { body: update }));
      queryClient.setQueryData(preferencesQueryKey, saved);
    } catch {
      return;
    }
  }
  finishLegacyImport();
}
