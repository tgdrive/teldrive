import type { FileCategory, FileSort } from "@/api/types";
import type { DriveSearchOptions } from "./queries";

export const searchCategories: FileCategory[] = [
  "archive",
  "audio",
  "document",
  "image",
  "video",
  "other",
];

export type SearchState = {
  q?: string;
  scope?: "drive" | "recursive";
  parentId?: string;
  folderPath?: string;
  kind?: "file" | "folder";
  category?: FileCategory[];
  updatedAfter?: string;
  updatedBefore?: string;
  sort?: FileSort;
  order?: "asc" | "desc";
  view?: "list" | "grid";
};

export function searchDate(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const match =
    /^(\d{4}-\d{2}-\d{2})(?:T([01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))?$/.exec(
      value,
    );
  if (!match) return undefined;
  const midnight = new Date(`${match[1]}T00:00:00.000Z`);
  if (Number.isNaN(midnight.valueOf()) || midnight.toISOString().slice(0, 10) !== match[1])
    return undefined;
  const date = match[2] ? new Date(value) : midnight;
  return Number.isNaN(date.valueOf()) ? undefined : date.toISOString();
}

export function invalidSearchDates(search: SearchState): boolean {
  return Boolean(
    search.updatedAfter && search.updatedBefore && search.updatedAfter >= search.updatedBefore,
  );
}

export function validateDriveSearch(value: Record<string, unknown>): SearchState {
  const requestedCategories =
    typeof value.category === "string" ? [value.category] : value.category;
  const category = searchCategories.filter(
    (category) => Array.isArray(requestedCategories) && requestedCategories.includes(category),
  );
  const parentId =
    typeof value.parentId === "string" &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
      value.parentId,
    )
      ? value.parentId
      : undefined;
  return {
    q: typeof value.q === "string" ? value.q.trim().slice(0, 512) || undefined : undefined,
    scope: value.scope === "recursive" ? "recursive" : "drive",
    parentId,
    folderPath:
      parentId && typeof value.folderPath === "string" && value.folderPath.startsWith("/")
        ? value.folderPath.slice(0, 4096)
        : undefined,
    kind: value.kind === "file" || value.kind === "folder" ? value.kind : undefined,
    category: category.length ? category : undefined,
    updatedAfter: searchDate(value.updatedAfter),
    updatedBefore: searchDate(value.updatedBefore),
    sort: value.sort === "updatedAt" || value.sort === "size" ? value.sort : "name",
    order: value.order === "desc" ? "desc" : "asc",
    view: value.view === "grid" ? "grid" : "list",
  };
}

export function driveSearchOptions(search: SearchState): DriveSearchOptions {
  return {
    q: search.q,
    scope: search.scope ?? "drive",
    parentId: search.scope === "recursive" ? search.parentId : undefined,
    kind: search.kind,
    category: search.category,
    updatedAfter: search.updatedAfter,
    updatedBefore: search.updatedBefore,
    sort: search.sort ?? "name",
    order: search.order ?? "asc",
  };
}

export function hasSearchCriteria(search: SearchState): boolean {
  return Boolean(
    search.q ||
      search.kind ||
      search.category?.length ||
      search.updatedAfter ||
      search.updatedBefore ||
      search.scope === "recursive",
  );
}
