import { create } from "zustand";
import type { FileEntry } from "@/api/types";

export type FileClipboardMode = "copy" | "cut";
export type FileClipboardPane = "primary" | "secondary";

type FileClipboardState = {
  mode?: FileClipboardMode;
  items: FileEntry[];
  sourceParentId?: string;
  sourcePath?: string;
  sourcePane?: FileClipboardPane;
  set: (
    mode: FileClipboardMode,
    items: FileEntry[],
    sourceParentId?: string,
    sourcePath?: string,
    sourcePane?: FileClipboardPane,
  ) => void;
  clear: () => void;
};

export const useFileClipboardStore = create<FileClipboardState>((set) => ({
  items: [],
  set: (mode, items, sourceParentId, sourcePath, sourcePane) =>
    set({ mode, items: [...items], sourceParentId, sourcePath, sourcePane }),
  clear: () =>
    set({
      mode: undefined,
      items: [],
      sourceParentId: undefined,
      sourcePath: undefined,
      sourcePane: undefined,
    }),
}));
