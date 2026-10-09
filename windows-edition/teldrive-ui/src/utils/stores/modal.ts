import type { FileData } from "@tw-material/file-browser";
import { create } from "zustand";

type ModalState = {
  open: boolean;
  operation: string;
  type: string;
  currentFile: FileData;
  selectedFiles?: string[];
  name?: string;
  actions: {
    setOperation: (operation: string) => void;
    setOpen: (open: boolean) => void;
    setCurrentFile: (currentFile: FileData) => void;
    setSelectedFiles: (selectedFiles: string[]) => void;
    set: (payload: Partial<ModalState>) => void;
  };
};

export const useModalStore = create<ModalState>((set) => ({
  open: false,
  operation: "",
  type: "",
  selectedFiles: [],
  name: "",
  currentFile: {} as FileData,
  actions: {
    setOperation: (operation: string) => set((state) => ({ ...state, operation })),
    setOpen: (open: boolean) => set((state) => ({ ...state, open })),
    setCurrentFile: (currentFile: FileData) => set((state) => ({ ...state, currentFile })),
    setSelectedFiles: (selectedFiles: string[]) => set((state) => ({ ...state, selectedFiles })),
    set: (payload) => set((state) => ({ ...state, ...payload })),
  },
}));
