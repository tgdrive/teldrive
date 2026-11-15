import { expect, test } from "@playwright/test";
import type { DirectoryDropItem, DropItem, FileDropItem } from "react-aria-components";

import { collectDroppedSources } from "../src/features/uploads/drop";

// The real drag path is covered in file-manager.spec.ts, which hands Chromium a
// drag backed by actual paths. These tests drive the walker directly for the
// cases a drag cannot reach cheaply: several roots, empty folders, dragged text,
// and entries that fail to read.

function droppedFile(name: string): FileDropItem {
  return {
    kind: "file",
    type: "application/octet-stream",
    name,
    getFile: () => Promise.resolve(new File([`contents of ${name}`], name)),
    getText: () => Promise.resolve(`contents of ${name}`),
  };
}

function unreadableFile(name: string): FileDropItem {
  return {
    kind: "file",
    type: "application/octet-stream",
    name,
    getFile: () => Promise.reject(new Error(`${name} cannot be read`)),
    getText: () => Promise.resolve(""),
  };
}

function droppedDirectory(
  name: string,
  entries: Array<FileDropItem | DirectoryDropItem>,
): DirectoryDropItem {
  return {
    kind: "directory",
    name,
    getEntries: async function* () {
      for (const entry of entries) yield entry;
    },
  };
}

function droppedText(value: string): DropItem {
  return {
    kind: "text",
    types: new Set(["text/plain"]),
    getText: () => Promise.resolve(value),
  };
}

function relativePaths(sources: Array<{ relativePath: string }>) {
  return sources.map((source) => source.relativePath);
}

test("a dropped file keeps its relative path", async () => {
  const { sources, skipped } = await collectDroppedSources([droppedFile("report.pdf")]);
  expect(relativePaths(sources)).toEqual(["report.pdf"]);
  expect(skipped).toBe(0);
});

test("a dropped folder keeps its shape", async () => {
  const { sources, skipped } = await collectDroppedSources([
    droppedDirectory("photos", [
      droppedFile("cover.jpg"),
      droppedDirectory("2026", [droppedFile("detail.jpg"), droppedFile("notes.txt")]),
    ]),
  ]);
  expect(relativePaths(sources)).toEqual([
    "photos/cover.jpg",
    "photos/2026/detail.jpg",
    "photos/2026/notes.txt",
  ]);
  expect(skipped).toBe(0);
});

test("several dropped folders and files each keep their own path", async () => {
  const { sources } = await collectDroppedSources([
    droppedDirectory("albums", [droppedFile("one.jpg")]),
    droppedFile("loose.jpg"),
    droppedDirectory("notes", [droppedFile("todo.md")]),
  ]);
  expect(relativePaths(sources)).toEqual(["albums/one.jpg", "loose.jpg", "notes/todo.md"]);
});

test("an empty folder and dragged text contribute nothing", async () => {
  const { sources, skipped } = await collectDroppedSources([
    droppedDirectory("empty", []),
    droppedText("just some text"),
    droppedDirectory("kept", [droppedFile("file.txt")]),
  ]);
  expect(relativePaths(sources)).toEqual(["kept/file.txt"]);
  expect(skipped).toBe(0);
});

test("an unreadable entry is skipped without losing the rest", async () => {
  const { sources, skipped } = await collectDroppedSources([
    droppedDirectory("photos", [unreadableFile("broken.jpg"), droppedFile("kept.jpg")]),
    unreadableFile("gone.txt"),
  ]);
  expect(relativePaths(sources)).toEqual(["photos/kept.jpg"]);
  expect(skipped).toBe(2);
});

test("a directory that fails part-way keeps what it already read", async () => {
  const directory: DirectoryDropItem = {
    kind: "directory",
    name: "photos",
    getEntries: async function* () {
      yield droppedFile("kept.jpg");
      throw new Error("the reader failed");
    },
  };
  const { sources, skipped } = await collectDroppedSources([directory]);
  expect(relativePaths(sources)).toEqual(["photos/kept.jpg"]);
  expect(skipped).toBe(1);
});

test("a directory that cannot be read at all is skipped", async () => {
  const directory: DirectoryDropItem = {
    kind: "directory",
    name: "gone",
    getEntries: async function* () {
      throw new Error("the reader failed");
    },
  };
  const { sources, skipped } = await collectDroppedSources([directory]);
  expect(sources).toEqual([]);
  expect(skipped).toBe(1);
});

test("a folder larger than the engine's spread limit is collected", async () => {
  // Appending with a spread throws once a folder holds more entries than the
  // engine accepts as call arguments, so keep more than that and assert none
  // are lost.
  const shared = new File(["x"], "shared.bin");
  const entries: FileDropItem[] = Array.from({ length: 150_000 }, (_, index) => ({
    kind: "file" as const,
    type: "application/octet-stream",
    name: `file-${index}.bin`,
    getFile: () => Promise.resolve(shared),
    getText: () => Promise.resolve(""),
  }));
  const { sources, skipped } = await collectDroppedSources([droppedDirectory("big", entries)]);
  expect(sources.length).toBe(entries.length);
  expect(sources.at(-1)?.relativePath).toBe("big/file-149999.bin");
  expect(skipped).toBe(0);
});
