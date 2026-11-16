import type { DirectoryDropItem, DropItem, FileDropItem } from "react-aria-components";
import type { UploadItem } from "@/features/uploads/store";

// DroppedSources is what a drop yielded: the files that could be read, and how
// many entries were skipped because reading them failed. A directory whose
// reader fails counts as one skipped entry, so the count is a lower bound.
type DroppedSources = { sources: UploadItem[]; skipped: number };

// collectDroppedSources turns the items of a drop event into upload sources,
// keeping the shape of every folder that was dropped.
//
// The browser exposes a dropped folder as a directory entry that is read one
// level at a time, so the whole tree is walked before anything is queued. An
// entry that cannot be read is counted and skipped rather than failing the whole
// drop, so one unreadable file does not lose the rest. Text dragged alongside
// files is ignored, and an empty folder produces no source, exactly like a
// folder chosen through the file picker.
export async function collectDroppedSources(items: readonly DropItem[]): Promise<DroppedSources> {
  const sources: UploadItem[] = [];
  let skipped = 0;
  for (const item of items) {
    if (item.kind === "file" || item.kind === "directory") {
      const collected = await collectEntry(item, item.name);
      appendAll(sources, collected.sources);
      skipped += collected.skipped;
    }
  }
  return { sources, skipped };
}

// appendAll appends without a spread: a dropped folder can hold more entries
// than an engine accepts as call arguments, and a spread throws once it does.
function appendAll(target: UploadItem[], values: UploadItem[]) {
  for (const value of values) target.push(value);
}

async function collectEntry(
  entry: FileDropItem | DirectoryDropItem,
  relativePath: string,
): Promise<DroppedSources> {
  if (entry.kind === "file") {
    try {
      return { sources: [{ file: await entry.getFile(), relativePath }], skipped: 0 };
    } catch {
      return { sources: [], skipped: 1 };
    }
  }
  const collected: DroppedSources = { sources: [], skipped: 0 };
  try {
    for await (const child of entry.getEntries()) {
      const result = await collectEntry(child, `${relativePath}/${child.name}`);
      appendAll(collected.sources, result.sources);
      collected.skipped += result.skipped;
    }
  } catch {
    // The directory reader failed part-way through; keep what it already
    // returned and count the directory itself as skipped.
    collected.skipped += 1;
  }
  return collected;
}
