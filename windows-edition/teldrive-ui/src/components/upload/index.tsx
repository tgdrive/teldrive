import { useQueryClient } from "@tanstack/react-query";
import { Button, Listbox, ListboxItem } from "@tw-material/react";
import clsx from "clsx";
import type React from "react";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { useShallow } from "zustand/react/shallow";

import IconParkOutlineCloseOne from "~icons/icon-park-outline/close-one";
import IconParkOutlineDownC from "~icons/icon-park-outline/down-c";

import { $api } from "@/utils/api";
import { filesize } from "@/utils/common";
import { useSession } from "@/utils/query-options";
import { FileUploadStatus, useFileUploadStore } from "@/utils/stores";
import { useSettingsStore } from "@/utils/stores/settings";
import { useSearch } from "@tanstack/react-router";
import type { UploadProps } from "./types";
import { uploadFile } from "./upload-file";
import { UploadFileEntry } from "./upload-file-entry";

export const Upload = ({ queryKey }: UploadProps) => {
  const {
    fileIds,
    currentFile,
    collapse,
    fileDialogOpen,
    folderDialogOpen,
    actions,
    fileMap,
  } = useFileUploadStore(
    useShallow((state) => ({
      fileIds: state.filesIds,
      fileMap: state.fileMap,
      currentFile: state.fileMap[state.currentFileId],
      collapse: state.collapse,
      actions: state.actions,
      fileDialogOpen: state.fileDialogOpen,
      folderDialogOpen: state.folderDialogOpen,
    })),
  );

  const isDialogOpening = useRef(false);

  const uploadSummary = useMemo(() => {
    const topLevelIds = fileIds.filter((id) => {
      const file = fileMap[id];
      if (!file) return false;
      const isChildFile =
        file.parentFolderId && fileIds.includes(file.parentFolderId);
      return !isChildFile;
    });

    // Filter out cancelled, failed, and skipped files from progress calculations
    const validFileIds = fileIds.filter((id) => {
      const status = fileMap[id]?.status;
      return (
        status !== FileUploadStatus.CANCELLED &&
        status !== FileUploadStatus.FAILED &&
        status !== FileUploadStatus.SKIPPED
      );
    });

    const validTopLevelIds = topLevelIds.filter((id) => {
      const status = fileMap[id]?.status;
      return (
        status !== FileUploadStatus.CANCELLED &&
        status !== FileUploadStatus.FAILED &&
        status !== FileUploadStatus.SKIPPED
      );
    });

    const folders = validTopLevelIds.filter(
      (id) => fileMap[id]?.isFolder,
    ).length;
    const files = validTopLevelIds.filter(
      (id) => !fileMap[id]?.isFolder,
    ).length;

    const totalSize = validFileIds.reduce(
      (sum, id) => sum + (fileMap[id]?.file.size || 0),
      0,
    );
    const uploadedSize = validFileIds.reduce((sum, id) => {
      const file = fileMap[id];
      // For uploaded files, count as 100% progress
      const progress =
        file?.status === FileUploadStatus.UPLOADED ? 100 : file?.progress || 0;
      return sum + (progress / 100) * (file?.file.size || 0);
    }, 0);

    const totalProgress = totalSize > 0 ? (uploadedSize / totalSize) * 100 : 0;

    return {
      folders,
      files,
      totalProgress,
      totalSize,
      uploadedSize,
    };
  }, [fileIds, fileMap]);

  const topLevelFileIds = useMemo(() => {
    return fileIds.filter((id) => {
      const file = fileMap[id];
      if (!file) return false;
      const isChildFile =
        file.parentFolderId && fileIds.includes(file.parentFolderId);
      return !isChildFile;
    });
  }, [fileIds, fileMap]);

  const { settings } = useSettingsStore();

  const [session] = useSession();

  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);

  const openFileSelector = useCallback(() => {
    if (!isDialogOpening.current) {
      isDialogOpening.current = true;
      fileInputRef?.current?.click();
      setTimeout(() => {
        isDialogOpening.current = false;
      }, 200);
    }
  }, []);

  const openFolderSelector = useCallback(() => {
    if (!isDialogOpening.current) {
      isDialogOpening.current = true;
      folderInputRef?.current?.click();
      setTimeout(() => {
        isDialogOpening.current = false;
      }, 200);
    }
  }, []);

  useEffect(() => {
    const handleFileSelect = () => {
      actions.setFileDialogOpen(false);
    };

    if (fileDialogOpen) {
      openFileSelector();
      fileInputRef.current?.addEventListener("change", handleFileSelect, {
        once: true,
      });
    }

    return () => {
      fileInputRef.current?.removeEventListener("change", handleFileSelect);
    };
  }, [fileDialogOpen, actions]);

  useEffect(() => {
    const handleFolderSelect = () => {
      actions.setFolderDialogOpen(false);
    };

    if (folderDialogOpen) {
      openFolderSelector();
      folderInputRef.current?.addEventListener("change", handleFolderSelect, {
        once: true,
      });
    }

    return () => {
      folderInputRef.current?.removeEventListener("change", handleFolderSelect);
    };
  }, [folderDialogOpen, actions]);

  const handleFileChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      actions.handleSelection(event.target.files);
      event.target.value = "";
    },
    [actions],
  );

  const handleFolderChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      actions.handleSelection(event.target.files);
      event.target.value = "";
    },
    [actions],
  );

  const queryClient = useQueryClient();

  const creatFile = $api.useMutation("post", "/files", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
    },
  });
  const { path } = useSearch({ from: "/_authed/$view" });

  useEffect(() => {
    if (
      currentFile?.id &&
      currentFile?.status === FileUploadStatus.NOT_STARTED
    ) {
      if (currentFile.isFolder) {
        actions.setFileUploadStatus(currentFile.id, FileUploadStatus.UPLOADING);
        creatFile
          .mutateAsync({
            body: {
              name: currentFile.file.name,
              type: "folder",
              path: currentFile.relativePath
                ? `${path || "/"}/${currentFile.relativePath.split("/").slice(0, -1).join("/")}`
                : path || "/",
            },
          })
          .then(() => {
            actions.setFileUploadStatus(
              currentFile.id,
              FileUploadStatus.UPLOADED,
            );
            actions.startNextUpload();
          })
          .catch((err) => {
            if (
              err.message.includes("already exists") ||
              err.message.includes("exists")
            ) {
              actions.setFileUploadStatus(
                currentFile.id,
                FileUploadStatus.SKIPPED,
              );
            } else {
              actions.setError(currentFile.id, err.message);
              actions.setFileUploadStatus(
                currentFile.id,
                FileUploadStatus.FAILED,
              );
            }
          });
      } else {
        actions.setFileUploadStatus(currentFile.id, FileUploadStatus.UPLOADING);
        uploadFile(
          currentFile.file,
          currentFile.parentFolderId
            ? `${path || "/"}/${currentFile.relativePath?.split("/").slice(0, -1).join("/")}`
            : path || "/",
          Number(settings.splitFileSize),
          session?.userId as number,
          Number(settings.uploadConcurrency),
          Number(settings.uploadRetries),
          Number(settings.uploadRetryDelay),
          Boolean(settings.encryptFiles),
          Boolean(settings.randomChunking),
          currentFile.controller.signal,
          (progress) => actions.setProgress(currentFile.id, progress),
          (chunks) => actions.setChunksCompleted(currentFile.id, chunks),
          async (payload) => {
            await creatFile.mutateAsync({
              body: payload,
            });
            if (creatFile.isSuccess) {
              actions.setFileUploadStatus(
                currentFile.id,
                FileUploadStatus.UPLOADED,
              );
            }
          },
          currentFile.parentFolderId !== undefined, // Skip check for folder files
        )
          .then(() => {
            if (currentFile.status !== FileUploadStatus.SKIPPED) {
              actions.setFileUploadStatus(
                currentFile.id,
                FileUploadStatus.UPLOADED,
              );
            }
            actions.startNextUpload();
          })
          .catch((error) => {
            if (error.message.includes("already exists")) {
              actions.setFileUploadStatus(
                currentFile.id,
                FileUploadStatus.SKIPPED,
              );
            } else if (error.message.includes("aborted")) {
              actions.setFileUploadStatus(
                currentFile.id,
                FileUploadStatus.CANCELLED,
              );
            } else {
              actions.setError(
                currentFile.id,
                error instanceof Error ? error.message : "upload failed",
              );
              actions.setFileUploadStatus(
                currentFile.id,
                FileUploadStatus.FAILED,
              );
            }
          });
      }
    }
  }, [currentFile?.id, currentFile?.status]);

  return (
    <div className="fixed bottom-4 right-4 z-50 max-w-sm">
      <input
        className="opacity-0 size-0"
        ref={fileInputRef}
        type="file"
        multiple
        onChange={handleFileChange}
      />
      <input
        className="opacity-0 size-0"
        ref={folderInputRef}
        type="file"
        {...({ webkitdirectory: "" } as any)}
        onChange={handleFolderChange}
      />
      {fileIds.length > 0 && (
        <div className="relative w-96 shadow-2xl rounded-xl overflow-hidden bg-surface-container-high border border-outline-variant/10">
          <div
            className={clsx(
              "transition-all duration-300 ease-[cubic-bezier(0.2,0,0,1)]",
              collapse ? "translate-y-0" : "translate-y-0",
            )}
          >
            <div
              className={clsx(
                "relative overflow-hidden transition-colors duration-300",
                collapse
                  ? "bg-surface-container-high"
                  : "bg-surface-container-highest",
              )}
            >
              <div className="h-[3px] w-full bg-primary/10 relative overflow-hidden">
                <div
                  className="h-full bg-primary transition-all duration-500 ease-[cubic-bezier(0.2,0,0,1)]"
                  style={{ width: `${uploadSummary.totalProgress}%` }}
                />
              </div>
              <div className="flex items-center px-4 py-2.5 justify-between">
                <div className="flex flex-1 items-center gap-3">
                  <span className="text-label-large text-on-surface">
                    {uploadSummary.totalProgress === 100
                      ? "Carga completada"
                      : "Subiendo…"}
                  </span>
                  {uploadSummary.totalSize > 0 && (
                    <span className="text-label-medium text-on-surface-variant">
                      {filesize(uploadSummary.uploadedSize)} de{" "}
                      {filesize(uploadSummary.totalSize)}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-1">
                  <Button
                    variant="text"
                    className="text-on-surface-variant size-8 min-w-8 p-0"
                    isIconOnly
                    onPress={actions.toggleCollapse}
                  >
                    <IconParkOutlineDownC
                      className={clsx(
                        "size-5 transition-transform duration-300 ease-[cubic-bezier(0.2,0,0,1)]",
                        collapse ? "rotate-180" : "rotate-0",
                      )}
                    />
                  </Button>
                  <Button
                    variant="text"
                    className="text-on-surface-variant size-8 min-w-8 p-0"
                    isIconOnly
                    onPress={actions.cancelUpload}
                  >
                    <IconParkOutlineCloseOne className="size-5" />
                  </Button>
                </div>
              </div>
            </div>
            <div
              className={clsx(
                "bg-surface-container-low overflow-hidden transition-all duration-300 ease-[cubic-bezier(0.2,0,0,1)]",
                collapse
                  ? "opacity-0 scale-95 pointer-events-none h-0"
                  : "opacity-100 scale-100 pointer-events-auto max-h-96 overflow-y-auto",
                "scrollbar-thin scrollbar-thumb-outline-variant scrollbar-track-transparent",
              )}
            >
              <div className="px-2 py-2">
                <Listbox
                  aria-label="Subir archivos"
                  isVirtualized={fileIds.length > 100}
                  className="select-none gap-1"
                >
                  {topLevelFileIds.map((id) => (
                    <ListboxItem
                      className="data-[hover=true]:bg-transparent px-0"
                      key={id}
                      textValue={id}
                    >
                      <UploadFileEntry
                        id={id}
                        chunkSize={Number(settings.splitFileSize)}
                        fileIds={fileIds}
                      />
                    </ListboxItem>
                  ))}
                </Listbox>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
