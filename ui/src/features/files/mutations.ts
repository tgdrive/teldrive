import { useQueryClient } from "@tanstack/react-query";
import { $api } from "@/api/client";
import type { FileEntry, NameConflictPolicy } from "@/api/types";
import { newIdempotencyKey } from "@/features/shared/idempotency";

export function useFileActions() {
  const queryClient = useQueryClient();
  const createFolderMutation = $api.useMutation("post", "/v1/folders");
  const renameMutation = $api.useMutation("patch", "/v1/files/{fileId}");
  const moveMutation = $api.useMutation("post", "/v1/files/{fileId}/move");
  const copyMutation = $api.useMutation("post", "/v1/files/{fileId}/copy");
  const trashMutation = $api.useMutation("delete", "/v1/files/{fileId}");
  const restoreMutation = $api.useMutation("post", "/v1/files/{fileId}/restore");
  const purgeMutation = $api.useMutation("delete", "/v1/files/{fileId}/purge");

  const cleanTrashMutation = $api.useMutation("delete", "/v1/files/trash");
  const bulkMoveMutation = $api.useMutation("post", "/v1/files/bulk/move");
  const bulkTrashMutation = $api.useMutation("post", "/v1/files/bulk/trash");

  async function invalidateFiles() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/files"] }),
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/files/{fileId}"] }),
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/files/statistics/drive"] }),
    ]);
  }

  async function createFolder(name: string, parentId?: string) {
    const result = await createFolderMutation.mutateAsync({
      params: { header: { "Idempotency-Key": newIdempotencyKey() } },
      body: { parentId, name, conflictPolicy: "fail" },
    });
    await invalidateFiles();
    return result;
  }

  async function rename(file: FileEntry, name: string) {
    const result = await renameMutation.mutateAsync({
      params: {
        path: { fileId: file.id },
        header: { "If-Match": `"${file.generation}"` },
      },
      body: { name },
    });
    await invalidateFiles();
    return result;
  }

  async function move(
    file: FileEntry,
    parentId?: string,
    conflictPolicy: NameConflictPolicy = "fail",
  ) {
    const result = await moveMutation.mutateAsync({
      params: {
        path: { fileId: file.id },
        header: {
          "Idempotency-Key": newIdempotencyKey(),
          "If-Match": `"${file.generation}"`,
        },
      },
      body: { parentId, conflictPolicy },
    });
    await invalidateFiles();
    return result;
  }

  async function copy(
    file: FileEntry,
    parentId?: string,
    name?: string,
    conflictPolicy: NameConflictPolicy = "fail",
  ) {
    const result = await copyMutation.mutateAsync({
      params: {
        path: { fileId: file.id },
        header: { "Idempotency-Key": newIdempotencyKey() },
      },
      body: { parentId, name, conflictPolicy },
    });
    await invalidateFiles();
    return result;
  }

  async function copyMany(
    files: FileEntry[],
    parentId?: string,
    conflictPolicy: NameConflictPolicy = "fail",
  ) {
    const results = await Promise.all(
      files.map((file) =>
        copyMutation.mutateAsync({
          params: {
            path: { fileId: file.id },
            header: { "Idempotency-Key": newIdempotencyKey() },
          },
          body: { parentId, conflictPolicy },
        }),
      ),
    );
    await invalidateFiles();
    return results;
  }

  async function trash(fileId: string) {
    const result = await trashMutation.mutateAsync({ params: { path: { fileId } } });
    await invalidateFiles();
    return result;
  }

  async function restore(fileId: string) {
    const result = await restoreMutation.mutateAsync({
      params: {
        path: { fileId },
        header: { "Idempotency-Key": newIdempotencyKey() },
      },
    });
    await invalidateFiles();
    return result;
  }

  async function purge(fileId: string) {
    const result = await purgeMutation.mutateAsync({ params: { path: { fileId } } });
    await invalidateFiles();
    return result;
  }

  async function cleanTrash() {
    const result = await cleanTrashMutation.mutateAsync({});
    await invalidateFiles();
    return result;
  }

  async function bulkMove(
    fileIds: string[],
    parentId?: string,
    conflictPolicy: NameConflictPolicy = "fail",
  ) {
    const result = await bulkMoveMutation.mutateAsync({
      params: { header: { "Idempotency-Key": newIdempotencyKey() } },
      body: { fileIds, parentId, conflictPolicy },
    });
    await invalidateFiles();
    return result;
  }

  async function bulkTrash(fileIds: string[]) {
    const result = await bulkTrashMutation.mutateAsync({
      params: { header: { "Idempotency-Key": newIdempotencyKey() } },
      body: { fileIds },
    });
    await invalidateFiles();
    return result;
  }

  async function bulkRestore(fileIds: string[]) {
    await Promise.all(
      fileIds.map((fileId) =>
        restoreMutation.mutateAsync({
          params: {
            path: { fileId },
            header: { "Idempotency-Key": newIdempotencyKey() },
          },
        }),
      ),
    );
    await invalidateFiles();
  }

  const mutations = [
    createFolderMutation,
    renameMutation,
    moveMutation,
    copyMutation,
    trashMutation,
    restoreMutation,
    purgeMutation,

    cleanTrashMutation,
    bulkMoveMutation,
    bulkTrashMutation,
  ];

  return {
    createFolder,
    rename,
    move,
    copy,
    copyMany,
    trash,
    restore,
    purge,

    cleanTrash,
    bulkMove,
    bulkTrash,
    bulkRestore,
    pending: mutations.some((mutation) => mutation.isPending),
    error: mutations.find((mutation) => mutation.isError)?.error,
  };
}

export type ReturnTypeUseFileActions = ReturnType<typeof useFileActions>;
