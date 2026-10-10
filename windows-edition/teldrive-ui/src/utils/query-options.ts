import type { FileListParams, ShareListParams } from "@/types";
import { infiniteQueryOptions, queryOptions, useQuery } from "@tanstack/react-query";
import type { FileData } from "@tw-material/file-browser";

import type { components } from "@/lib/api";
import { fetchClient } from "./api";
import { getExtension, mediaUrl, sharedMediaUrl } from "./common";
import { getSortState, sortIdsMap, sortViewMap } from "./defaults";
import { getPreviewType } from "./preview-type";
import { useSettingsStore } from "./stores/settings";

export const sessionOptions = queryOptions({
  queryKey: ["session"],
  queryFn: async ({ signal }) => {
    const res = await fetchClient.GET("/auth/session", {
      signal,
    });
    if (res.response.status === 204) {
      return null;
    }
    return res.data;
  },
});

export const useSession = () => {
  const { data, isLoading, refetch } = useQuery(sessionOptions);
  const status = isLoading ? "loading" : data?.userId ? "success" : "unauthenticated";
  return [data ?? null, status, refetch] as const;
};

export const fileQueries = {
  list: (params: FileListParams, sessionHash?: string) =>
    infiniteQueryOptions({
      queryKey: ["Files_list", params.view, params.params],
      queryFn: fetchFiles(params),
      initialPageParam: 1,
      getNextPageParam: (lastPage, _) =>
        lastPage?.meta.currentPage! + 1 > lastPage?.meta.totalPages!
          ? undefined
          : lastPage?.meta.currentPage! + 1,
      select: (data) =>
        data.pages.flatMap((page) =>
          page?.items ? mapFilesToFb(page?.items!, sessionHash as string) : [],
        ),
    }),
};

export const shareQueries = {
  list: (params: ShareListParams) =>
    infiniteQueryOptions({
      queryKey: ["Shares_listFiles", params],
      queryFn: async ({ signal, pageParam }) =>
        (
          await fetchClient.GET("/shares/{id}/files", {
            params: {
              path: { id: params.id },
              query: { page: pageParam, ...(params.path ? { path: params.path } : {}) },
            },
            signal,
          })
        ).data,
      initialPageParam: 1,
      getNextPageParam: (lastPage, _) =>
        lastPage?.meta.currentPage! + 1 > lastPage?.meta.totalPages!
          ? undefined
          : lastPage?.meta.currentPage! + 1,
      select: (data) =>
        data.pages.flatMap((page) => (page?.items ? mapFilesToFb(page?.items, "", params.id) : [])),
    }),
};

const fetchFiles =
  (qparams: FileListParams) =>
  async ({ pageParam, signal }: { pageParam: number; signal: AbortSignal }) => {
    const { view, params } = qparams;
    const sortState = view === "my-drive" ? getSortState() : sortViewMap[view];
    const query: FileListParams["params"] = {
      page: pageParam,
      order: params?.order || sortState.order,
      sort: params?.sort || sortIdsMap[sortState.sortId],
    };
    if (view === "my-drive") {
      query.path = params?.path ?? "/";
    } else if (view === "search") {
      query.operation = "find";
      query.searchType = params?.searchType;
      for (const key in params) {
        query[key] = params[key];
      }
    } else if (view === "recent") {
      query.operation = "find";
      query.type = "file";
    } else if (view === "browse") {
      query.parentId = params?.parentId;
      if (params?.category) {
        query.operation = "find";
        query.type = "file";
        query.category = params?.category;
      }
    } else if (view === "shared") {
      query.operation = "find";
      query.shared = true;
    }

    return (
      await fetchClient.GET("/files", {
        params: {
          query,
        },
        signal,
      })
    ).data;
  };

const mapFilesToFb = (files: components["schemas"]["FileList"]["items"], sessionHash: string, shareId?: string) => {
  return files.map((item): FileData => {
    if (item.mimeType === "drive/folder") {
      return {
        id: item.id!,
        name: item.name,
        type: item.type,
        mimeType: item.mimeType,
        size: item.size ? Number(item.size) : 0,
        modDate: item.updatedAt,
        isDir: true,
      };
    }

    const previewType = getPreviewType(getExtension(item.name), {
      video: item.mimeType?.includes("video"),
      audio: item.mimeType?.startsWith("audio/"),
    });

    const { settings } = useSettingsStore.getState();

    let thumbnailUrl = "";
    if (previewType === "image") {
      thumbnailUrl = shareId ? sharedMediaUrl(shareId, item.id!, item.name) : mediaUrl(item.id!, item.name, "", sessionHash);
      if (settings.resizerHost && !shareId) {
        const url = mediaUrl(item.id!, item.name, "", sessionHash);
        thumbnailUrl = settings.resizerHost
          ? `${settings.resizerHost}/insecure/w:360/plain/${encodeURIComponent(url)}`
          : "";
      }
    }
    return {
      id: item.id!,
      name: item.name,
      type: item.type,
      mimeType: item.mimeType,
      size: item.size ? Number(item.size) : 0,
      previewType,
      openable: true,
      thumbnailUrl,
      modDate: item.updatedAt,
      isEncrypted: item.encrypted,
    };
  });
};
