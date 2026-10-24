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
import { isAppLocale, translate } from "@/lib/locales";
import { getQueryClient } from "@/lib/queryClient";

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
    // Providers read the cache without observers. Only logout should evict it.
    gcTime: Number.POSITIVE_INFINITY,
    retry: false,
  });
}

export function hasCachedPreferences() {
  return getQueryClient().getQueryData(preferencesQueryKey) !== undefined;
}

// Signed-in routes load preferences before rendering; this never fetches.
export function usePreferences(): UserPreferences | undefined {
  const queryClient = useQueryClient();
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
    captions: { ...current.captions, ...update.captions },
  };
}

// Roll back only the failed patch's fields, preserving other in-flight changes.
function previousValues<T extends object>(previous: T, changed: Partial<T> | undefined) {
  const values: Partial<T> = {};
  for (const key of Object.keys(changed ?? {}) as (keyof T)[]) values[key] = previous[key];
  return values;
}

function displayedLocale() {
  const lang = document.documentElement.lang;
  return isAppLocale(lang) ? lang : "en";
}

export function useUpdatePreferences() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: preferencesMutationKey,
    mutationFn: (patch: UserPreferencesUpdate) =>
      unwrap(fetchClient.PATCH("/v1/me/preferences", { body: patch })),
    onMutate: async (patch) => {
      await queryClient.cancelQueries({ queryKey: preferencesQueryKey });
      const previous = queryClient.getQueryData<UserPreferences>(preferencesQueryKey);
      if (previous)
        queryClient.setQueryData(preferencesQueryKey, {
          ...mergePreferences(previous, patch),
          updatedAt: new Date().toISOString(),
        });
      return { previous };
    },
    onSuccess: (saved) => {
      // Responses can arrive out of order; the final in-flight write is authoritative.
      if (queryClient.isMutating({ mutationKey: preferencesMutationKey }) === 1)
        queryClient.setQueryData(preferencesQueryKey, saved);
    },
    onError: (error, patch, context) => {
      const previous = context?.previous;
      if (previous)
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
                  captions: previousValues(previous.captions, patch.captions),
                }),
                updatedAt: previous.updatedAt,
              }
            : current,
        );
      toast.error(translate(displayedLocale(), "preferences.saveFailed"), {
        description: userMessage(error),
      });
    },
  });
}
