import type { FileEntry } from "@/api/types";

const ebookExtensions = [".epub", ".mobi", ".azw", ".azw3", ".fb2", ".fbz", ".cbz"];

export type ReaderKind = "ebook" | "pdf";

export function readerKind(file: FileEntry): ReaderKind | undefined {
  const name = file.name.toLowerCase();
  const mime = (file.mimeType || "").toLowerCase();
  if (mime === "application/pdf" || name.endsWith(".pdf")) return "pdf";
  if (
    mime.includes("epub") ||
    mime.includes("mobipocket") ||
    mime.includes("fictionbook") ||
    mime.includes("comicbook") ||
    ebookExtensions.some((extension) => name.endsWith(extension))
  ) {
    return "ebook";
  }
  return undefined;
}

export function supportsReader(file: FileEntry) {
  return readerKind(file) !== undefined;
}
