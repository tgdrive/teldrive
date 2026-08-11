import { apiFetch } from "@/api/client";

export type ViewerKind = "image" | "video" | "audio" | "pdf" | "ebook" | "text";

export type ViewState = {
  fileId: string;
  kind: ViewerKind;
  position: Record<string, unknown>;
  preferences: Record<string, unknown>;
  bookmarks: Record<string, unknown>[];
  updatedAt: string;
};

export async function getViewState(fileId: string, signal?: AbortSignal) {
  const response = await apiFetch(`/v1/files/${encodeURIComponent(fileId)}/view-state`, { signal });
  return response.status === 204 ? undefined : ((await response.json()) as ViewState);
}

export async function putViewState(
  fileId: string,
  kind: ViewerKind,
  position: Record<string, unknown>,
  preferences: Record<string, unknown> = {},
  bookmarks: Record<string, unknown>[] = [],
) {
  await apiFetch(`/v1/files/${encodeURIComponent(fileId)}/view-state`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ kind, position, preferences, bookmarks }),
  });
}
