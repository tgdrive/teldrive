export type FileSort = "name" | "updatedAt" | "size";
export type SortOrder = "asc" | "desc";

export function validFileSort(value: unknown): FileSort | undefined {
  return value === "name" || value === "updatedAt" || value === "size" ? value : undefined;
}

export function validSortOrder(value: unknown): SortOrder | undefined {
  return value === "asc" || value === "desc" ? value : undefined;
}
