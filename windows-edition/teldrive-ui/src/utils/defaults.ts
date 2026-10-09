export enum SortOrder {
  ASC = "asc",
  DESC = "desc",
}

export const sortViewMap = {
  "my-drive": {
    sortId: "sort_files_by_name",
    order: SortOrder.ASC,
  },
  browse: {
    sortId: "sort_files_by_name",
    order: SortOrder.ASC,
  },
  search: { sortId: "sort_files_by_name", order: SortOrder.ASC },
  recent: { sortId: "sort_files_by_date", order: SortOrder.DESC },
  category: { sortId: "sort_files_by_name", order: SortOrder.ASC },
  shared: { sortId: "sort_files_by_date", order: SortOrder.DESC },
};

export type SortState = typeof sortViewMap;

export function getSortState(): SortState["my-drive"] {
  try {
    const value = JSON.parse(localStorage.getItem("sort") || "null");
    if (value && ["sort_files_by_name", "sort_files_by_size", "sort_files_by_date"].includes(value.sortId) && ["asc", "desc"].includes(value.order)) return value;
  } catch { /* Ignore obsolete or malformed saved preferences. */ }
  return sortViewMap["my-drive"];
}

export const defaultSortState = getSortState();

export const defaultViewId = localStorage.getItem("viewId") || "enable_list_view";

export const sortIdsMap = {
  sort_files_by_name: "name",
  sort_files_by_date: "updatedAt",
  sort_files_by_size: "size",
} as const;

export const BREAKPOINTS = { xs: 0, sm: 476, md: 576, lg: 992 };
