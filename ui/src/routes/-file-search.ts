import { z } from "zod";

export const fileSearchSchema = z.object({
  path: z.string().catch("/").default("/"),
  parentId: z.string().uuid().optional().catch(undefined),
  q: z.string().optional().catch(undefined),
  sort: z.enum(["name", "updatedAt", "size", "id"]).catch("name").default("name"),
  order: z.enum(["asc", "desc"]).catch("asc").default("asc"),
  category: z
    .enum(["archive", "audio", "document", "image", "video", "other"])
    .optional()
    .catch(undefined),
  cursor: z.string().optional().catch(undefined),
  cursorHistory: z.string().optional().catch(undefined),
  view: z.enum(["list", "grid"]).catch("list").default("list"),
  preview: z.string().uuid().optional().catch(undefined),
  read: z.string().uuid().optional().catch(undefined),
  action: z
    .enum(["new-folder", "rename", "move", "copy", "trash", "restore", "purge"])
    .optional()
    .catch(undefined),
});

export type FileSearch = z.infer<typeof fileSearchSchema>;

export const defaultFileSearch: FileSearch = fileSearchSchema.parse({});
