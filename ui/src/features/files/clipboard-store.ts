import { create } from "zustand";
import type { FileEntry } from "@/api/types";

export type FileClipboardMode = "copy" | "cut";

type FileClipboardState = {
  mode?: FileClipboardMode;
  items: FileEntry[];
  sourceParentId?: string;
  sourcePath?: string;
  set: (
    mode: FileClipboardMode,
    items: FileEntry[],
    sourceParentId?: string,
    sourcePath?: string,
  ) => void;
  clear: () => void;
};

export const useFileClipboardStore = create<FileClipboardState>((set) => ({
  items: [],
  set: (mode, items, sourceParentId, sourcePath) =>
    set({
      mode,
      items: [...items],
      sourceParentId,
      sourcePath,
    }),
  clear: () =>
    set({ mode: undefined, items: [], sourceParentId: undefined, sourcePath: undefined }),
}));
