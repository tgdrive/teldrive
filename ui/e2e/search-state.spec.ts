import { expect, test } from "@playwright/test";
import {
  driveSearchOptions,
  invalidSearchDates,
  searchDate,
  validateDriveSearch,
} from "../src/features/files/search-state";

test("search state rejects invalid calendar dates and preserves precise timestamps", () => {
  for (const value of ["2026-02-30", "2026-02-30T12:00:00Z", "2026-05-10T24:00:00Z", "invalid"])
    expect(searchDate(value)).toBeUndefined();
  expect(searchDate("2026-05-10")).toBe("2026-05-10T00:00:00.000Z");
  expect(searchDate("2026-05-10T12:34:56Z")).toBe("2026-05-10T12:34:56.000Z");
  expect(searchDate("2026-05-10T14:34:56+02:00")).toBe("2026-05-10T12:34:56.000Z");
});

test("search state canonicalizes URL filters without including drive folder context in queries", () => {
  const parentId = "22222222-2222-4222-8222-222222222222";
  const state = validateDriveSearch({
    q: "  report  ",
    category: ["image", "document", "image", "bad"],
    parentId,
    folderPath: "/Documents",
    scope: "drive",
    sort: "invalid",
    view: "grid",
  });
  expect(state.q).toBe("report");
  expect(state.category).toEqual(["document", "image"]);
  expect(state.sort).toBe("name");
  expect(driveSearchOptions(state).parentId).toBeUndefined();
  expect(driveSearchOptions({ ...state, scope: "recursive" }).parentId).toBe(parentId);
  expect(validateDriveSearch({ category: "document" }).category).toEqual(["document"]);
  expect(
    validateDriveSearch({ parentId: "not-a-uuid", folderPath: "/Documents" }).folderPath,
  ).toBeUndefined();
  expect(validateDriveSearch({ q: "a".repeat(600) }).q?.length).toBe(512);
  expect(invalidSearchDates({ updatedAfter: "2026-05-10", updatedBefore: "2026-05-10" })).toBe(
    true,
  );
});
