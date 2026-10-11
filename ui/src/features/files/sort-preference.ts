export type FileSort = "name" | "updatedAt" | "size";
export type SortOrder = "asc" | "desc";
export type SortPreference = { sort: FileSort; order: SortOrder };

export function validFileSort(value: unknown): FileSort | undefined {
  return value === "name" || value === "updatedAt" || value === "size" ? value : undefined;
}

export function validSortOrder(value: unknown): SortOrder | undefined {
  return value === "asc" || value === "desc" ? value : undefined;
}

export function loadSortPreference(): SortPreference {
  try {
    const saved = JSON.parse(localStorage.getItem("file-sort") ?? "{}");
    return {
      sort: validFileSort(saved?.sort) ?? "name",
      order: validSortOrder(saved?.order) ?? "asc",
    };
  } catch {
    return { sort: "name", order: "asc" };
  }
}

export function saveSortPreference(value: SortPreference) {
  try {
    localStorage.setItem("file-sort", JSON.stringify(value));
  } catch {
    // Sorting still works when browser storage is unavailable.
  }
}
