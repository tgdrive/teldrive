import { useInfiniteQuery } from "@tanstack/react-query";
import { z } from "zod";
import { $api, fetchClient } from "@/api/client";
import { invalidResponse } from "@/api/errors";
import type { paths } from "@/api/schema";
import type { FileCategory, FileSort, FileStatus } from "@/api/types";

type FileListResponse = paths["/v1/files"]["get"]["responses"][200]["content"]["application/json"];

const fileListSchema = z
  .object({
    items: z.array(
      z
        .object({
          id: z.string().uuid(),
          name: z.string(),
          kind: z.enum(["file", "folder"]),
          status: z.enum(["active", "trashed", "deletion_pending"]),
        })
        .passthrough(),
    ),
    nextCursor: z.string().optional(),
  })
  .passthrough();

function validateFileList(data: FileListResponse): FileListResponse {
  const result = fileListSchema.safeParse(data);
  if (!result.success) {
    throw invalidResponse(
      "Teldrive received data from an incompatible API version. Refresh after the server and UI are upgraded together.",
    );
  }
  return data;
}

export type FileRouteAction =
  | "new-folder"
  | "rename"
  | "move"
  | "copy"
  | "trash"
  | "restore"
  | "purge";

export type FileRouteSearch = {
  path: string;
  parentId?: string;
  q?: string;
  sort: FileSort;
  order: "asc" | "desc";
  category?: FileCategory;
  cursor?: string;
  cursorHistory?: string;
  view: "list" | "grid";
  preview?: string;
  read?: string;
  action?: FileRouteAction;
};

export function filePageInit(search: FileRouteSearch, status: FileStatus, cursor = search.cursor) {
  return {
    params: {
      query: {
        parentId: search.parentId,
        path: search.parentId ? undefined : search.path === "/" ? undefined : search.path,
        status,
        search: search.q || undefined,
        category: search.category ? [search.category] : undefined,
        sort: search.sort,
        order: search.order,
        cursor,
        limit: 100,
      },
    },
  };
}

export function filePageQueryOptions(search: FileRouteSearch, status: FileStatus) {
  return $api.queryOptions("get", "/v1/files", filePageInit(search, status), {
    staleTime: 15_000,
    select: validateFileList,
  });
}

export function useFilePage(search: FileRouteSearch, status: FileStatus) {
  return $api.useSuspenseQuery("get", "/v1/files", filePageInit(search, status), {
    staleTime: 15_000,
    select: validateFileList,
  });
}

export function useInfiniteFilePages(search: FileRouteSearch, status: FileStatus, enabled = true) {
  const queryKey = [
    "get",
    "/v1/files",
    "infinite",
    {
      path: search.path,
      parentId: search.parentId,
      q: search.q,
      sort: search.sort,
      order: search.order,
      category: search.category,
      status,
    },
  ] as const;

  return useInfiniteQuery({
    queryKey,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => {
      const result = await fetchClient.GET("/v1/files", {
        ...filePageInit(search, status, pageParam),
        signal,
      });
      if (!result.data) {
        throw invalidResponse("Teldrive returned an empty file-list response.");
      }
      return validateFileList(result.data);
    },
    getNextPageParam: (lastPage) => lastPage.nextCursor,
    staleTime: 15_000,
    enabled,
  });
}

export function useFolderChildren(parentId?: string, path?: string) {
  return $api.useQuery(
    "get",
    "/v1/files",
    {
      params: {
        query: {
          parentId,
          path: parentId ? undefined : path === "/" ? undefined : path,
          kind: "folder",
          status: "active",
          limit: 200,
          sort: "name",
          order: "asc",
        },
      },
    },
    { staleTime: 20_000 },
  );
}
