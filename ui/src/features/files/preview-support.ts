import type { FileEntry } from "@/api/types";

const previewExtensions = new Set([
  "bash",
  "c",
  "cc",
  "cpp",
  "cs",
  "css",
  "dockerfile",
  "go",
  "graphql",
  "h",
  "hpp",
  "html",
  "java",
  "js",
  "json",
  "jsx",
  "kt",
  "kts",
  "md",
  "php",
  "py",
  "rb",
  "rs",
  "scss",
  "sh",
  "sql",
  "swift",
  "toml",
  "ts",
  "tsx",
  "vue",
  "xml",
  "yaml",
  "yml",
]);

const mediaTypes: Record<string, string> = {
  aac: "audio/aac",
  avif: "image/avif",
  bmp: "image/bmp",
  flac: "audio/flac",
  gif: "image/gif",
  ico: "image/x-icon",
  jpeg: "image/jpeg",
  jpg: "image/jpeg",
  m4a: "audio/mp4",
  m4v: "video/x-m4v",
  mkv: "video/x-matroska",
  mov: "video/quicktime",
  mp3: "audio/mpeg",
  mp4: "video/mp4",
  oga: "audio/ogg",
  ogg: "audio/ogg",
  ogv: "video/ogg",
  opus: "audio/ogg",
  png: "image/png",
  svg: "image/svg+xml",
  wav: "audio/wav",
  webm: "video/webm",
  webp: "image/webp",
};

export type PreviewMediaKind = "image" | "video" | "audio";

export function previewMedia(file: FileEntry): { kind: PreviewMediaKind; type: string } | undefined {
  const mime = file.mimeType?.toLowerCase() || "";
  for (const kind of ["image", "video", "audio"] as const) {
    if (mime.startsWith(`${kind}/`)) return { kind, type: mime };
  }
  const extension = file.name.toLowerCase().split(".").pop() || "";
  const type = mediaTypes[extension];
  if (!type) return undefined;
  return { kind: type.split("/", 1)[0] as PreviewMediaKind, type };
}

export function supportsCodePreview(file: FileEntry) {
  if (file.kind !== "file" || file.status !== "active") return false;
  const lower = file.name.toLowerCase();
  const extension = lower === "dockerfile" ? "dockerfile" : lower.split(".").pop() || "";
  const mime = file.mimeType?.toLowerCase() || "";
  return (
    previewExtensions.has(extension) ||
    mime.startsWith("text/") ||
    mime.includes("json") ||
    mime.includes("javascript") ||
    mime.includes("typescript") ||
    mime.includes("xml") ||
    mime.includes("yaml")
  );
}
