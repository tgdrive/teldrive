import type { Page } from "@playwright/test";
const now = "2026-07-22T12:00:00Z";
const rootId = "11111111-1111-4111-8111-111111111111";
export const alphaId = "22222222-2222-4222-8222-222222222222";
export const betaId = "33333333-3333-4333-8333-333333333333";

type FixtureFile = {
  id: string;
  parentId?: string;
  name: string;
  kind: "file" | "folder";
  status: "active" | "trashed";
  generation: number;
  mimeType?: string;
  size?: number;
  modTime: string;
  createdAt: string;
  updatedAt: string;
  encryption: boolean;
};

function file(
  overrides: Partial<FixtureFile> & Pick<FixtureFile, "id" | "name" | "kind">,
): FixtureFile {
  return {
    status: "active",
    generation: 1,
    modTime: now,
    createdAt: now,
    updatedAt: now,
    encryption: false,
    ...overrides,
  };
}

export async function installFileApi(page: Page) {
  let uploadSequence = 0;
  const files = new Map<string, FixtureFile>([
    [rootId, file({ id: rootId, name: "Destino", kind: "folder" })],
    [
      alphaId,
      file({ id: alphaId, name: "alpha.txt", kind: "file", mimeType: "text/plain", size: 10 }),
    ],
    [
      betaId,
      file({ id: betaId, name: "beta.txt", kind: "file", mimeType: "text/plain", size: 20 }),
    ],
  ]);
  const findNameConflict = (parentId: string | undefined, name: string, excludeId?: string) =>
    [...files.values()].find(
      (candidate) =>
        candidate.id !== excludeId &&
        candidate.status === "active" &&
        candidate.parentId === parentId &&
        candidate.name.toLowerCase() === name.toLowerCase(),
    );
  const nextAvailableName = (parentId: string | undefined, name: string, excludeId?: string) => {
    const dot = name.lastIndexOf(".");
    const stem = dot > 0 ? name.slice(0, dot) : name;
    const extension = dot > 0 ? name.slice(dot) : "";
    let index = 1;
    let candidate = `${stem} (${index})${extension}`;
    while (findNameConflict(parentId, candidate, excludeId)) {
      index += 1;
      candidate = `${stem} (${index})${extension}`;
    }
    return candidate;
  };

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = request.method();

    if (path === "/v1/me" && method === "GET") {
      return route.fulfill({
        json: {
          userId: 1,
          displayName: "Fixture User",
          username: "fixture",
          premium: true,
          role: "owner",
          capabilities: [
            "files.read",
            "files.write",
            "files.share",
            "system.manageUsers",
            "system.manageJobs",
            "system.manageQueues",
            "system.localImport",
            "system.maintenance",
            "system.owner",
          ],
          createdAt: now,
        },
      });
    }
    if (path === "/v1/files/statistics/drive" && method === "GET") {
      return route.fulfill({
        json: {
          totalFiles: files.size,
          totalFolders: 1,
          totalBytes: 30,
          trashedFiles: 0,
          activeShares: 0,
          openUploads: 0,
        },
      });
    }
    if (path === "/v1/files" && method === "GET") {
      const parentId = url.searchParams.get("parentId") ?? undefined;
      const search = url.searchParams.get("search");
      const searchType = url.searchParams.get("searchType");
      const items = [...files.values()].filter(
        (entry) =>
          entry.status === "active" &&
          entry.parentId === parentId &&
          (!search ||
            (searchType === "regex"
              ? new RegExp(search, "i").test(entry.name)
              : entry.name.toLowerCase().includes(search.toLowerCase()))),
      );
      return route.fulfill({ json: { items } });
    }
    if (path === `/v1/files/${alphaId}/content/alpha.txt` && method === "GET") {
      return route.fulfill({ contentType: "text/plain", body: "alpha preview" });
    }
    if (path === "/v1/folders" && method === "POST") {
      const body = request.postDataJSON() as { name: string; parentId?: string };
      const existing = [...files.values()].find(
        (entry) =>
          entry.status === "active" &&
          entry.parentId === body.parentId &&
          entry.name.toLowerCase() === body.name.toLowerCase(),
      );
      if (existing) {
        return route.fulfill({
          status: 409,
          json: { error: { code: "name_conflict", message: "Name already exists" } },
        });
      }
      const created = file({
        id: crypto.randomUUID(),
        name: body.name,
        kind: "folder",
        parentId: body.parentId,
      });
      files.set(created.id, created);
      return route.fulfill({ status: 201, json: created });
    }
    if (path === "/v1/uploads" && method === "POST") {
      const body = request.postDataJSON() as {
        name: string;
        parentId?: string;
        size: number;
        encryption: boolean;
      };
      uploadSequence++;
      return route.fulfill({
        status: 201,
        json: {
          id: `66666666-6666-4666-8666-${String(uploadSequence).padStart(12, "0")}`,
          userId: 1,
          parentId: body.parentId,
          name: body.name,
          expectedSize: body.size,
          partSize: 512 * 1024 * 1024,
          state: "open",
          encryption: body.encryption,
          conflictPolicy: "rename",
          createdAt: now,
          updatedAt: now,
          expiresAt: "2026-07-23T12:00:00Z",
        },
      });
    }
    const uploadMatch = path.match(
      /^\/v1\/uploads\/([^/]+)(?:\/(parts)(?:\/(\d+))?|\/(complete))?$/,
    );
    if (uploadMatch) {
      const [, uploadId, parts, , complete] = uploadMatch;
      if (method === "GET" && parts) return route.fulfill({ json: { items: [] } });
      if (method === "PUT" && parts) return route.fulfill({ status: 204 });
      if (method === "POST" && complete) {
        return route.fulfill({
          json: file({
            id: crypto.randomUUID(),
            name: uploadId,
            kind: "file",
            size: 1,
            mimeType: "application/octet-stream",
          }),
        });
      }
      if (method === "DELETE") return route.fulfill({ status: 204 });
    }
    if (path === "/v1/files/bulk/trash" && method === "POST") {
      const body = request.postDataJSON() as { fileIds: string[] };
      for (const id of body.fileIds) {
        const entry = files.get(id);
        if (entry) files.set(id, { ...entry, status: "trashed" });
      }
      return route.fulfill({ json: { items: body.fileIds } });
    }

    const match = path.match(/^\/v1\/files\/([^/]+)(?:\/(copy|move))?$/);
    if (match) {
      const [, id, operation] = match;
      const entry = files.get(id);
      if (!entry)
        return route.fulfill({
          status: 404,
          json: { error: { code: "not_found", message: "No encontrado" } },
        });
      if (method === "PATCH") {
        const body = request.postDataJSON() as { name: string };
        const renamed = { ...entry, name: body.name, generation: entry.generation + 1 };
        files.set(id, renamed);
        return route.fulfill({ json: renamed });
      }
      if (method === "POST" && operation === "copy") {
        const body = request.postDataJSON() as {
          parentId?: string;
          name?: string;
          conflictPolicy?: "fail" | "rename" | "replace";
        };
        let name = body.name ?? entry.name;
        const conflict = findNameConflict(body.parentId, name);
        if (conflict) {
          if (body.conflictPolicy === "rename") name = nextAvailableName(body.parentId, name);
          else if (body.conflictPolicy === "replace") files.delete(conflict.id);
          else {
            return route.fulfill({
              status: 409,
              json: { error: { code: "name_conflict", message: "Name already exists" } },
            });
          }
        }
        const copied = file({
          ...entry,
          id: crypto.randomUUID(),
          parentId: body.parentId,
          name,
        });
        files.set(copied.id, copied);
        return route.fulfill({ status: 201, json: copied });
      }
      if (method === "POST" && operation === "move") {
        const body = request.postDataJSON() as {
          parentId?: string;
          conflictPolicy?: "fail" | "rename" | "replace";
        };
        let name = entry.name;
        const conflict = findNameConflict(body.parentId, name, id);
        if (conflict) {
          if (body.conflictPolicy === "rename") name = nextAvailableName(body.parentId, name, id);
          else if (body.conflictPolicy === "replace") files.delete(conflict.id);
          else {
            return route.fulfill({
              status: 409,
              json: { error: { code: "name_conflict", message: "Name already exists" } },
            });
          }
        }
        const moved = { ...entry, parentId: body.parentId, name, generation: entry.generation + 1 };
        files.set(id, moved);
        return route.fulfill({ json: moved });
      }
    }

    return route.fulfill({
      status: 404,
      json: { error: { code: "not_found", message: `${method} ${path}` } },
    });
  });
}

