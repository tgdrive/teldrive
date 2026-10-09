import type { FileListParams, Session, ShareListParams } from "@/types";
import { useQueryClient } from "@tanstack/react-query";
import {
  type FbActionUnion,
  FbActions,
  FbIconName,
  type FileData,
  FileHelper,
  type MapFileActionsToData,
  defineFileAction,
} from "@tw-material/file-browser";
import { useCallback } from "react";
import toast from "react-hot-toast";
import IconFlatColorIconsVlc from "~icons/flat-color-icons/vlc";
import IconPotPlayerIcon from "~icons/material-symbols/play-circle-rounded";

import { $api } from "@/utils/api";
import {
  downloadFile,
  mediaUrl,
  navigateToExternalUrl,
  sharedMediaUrl,
} from "@/utils/common";
import { SortOrder, getSortState, sortIdsMap, sortViewMap } from "@/utils/defaults";
import { lifecycleChange } from "@/utils/lifecycle";
import { useFileUploadStore, useModalStore } from "@/utils/stores";
import { useNavigate } from "@tanstack/react-router";
import Share from "~icons/fluent/share-24-regular";
import MaterialSymbolsFolder from "~icons/material-symbols/folder";

export const CustomActions = {
  MarkSpam: defineFileAction({ id: "mark_spam", requiresSelection: true, button: { name: "Mover a Spam", contextMenu: true, icon: Share } } as const),
  OpenInVLCPlayer: defineFileAction({
    id: "open_vlc_player",
    requiresSelection: true,
    fileFilter: (file) => file?.previewType === "video",
    button: {
      name: "VLC",
      toolbar: true,
      group: "OpenOptions",
      icon: IconFlatColorIconsVlc,
    },
  } as const),
  OpenInPotPlayer: defineFileAction({
    id: "open_pot_player",
    requiresSelection: true,
    fileFilter: (file) => file?.previewType === "video",
    button: {
      name: "PotPlayer",
      toolbar: true,
      group: "OpenOptions",
      icon: IconPotPlayerIcon,
    },
  } as const),
  ShareFiles: defineFileAction({
    id: "share_files",
    requiresSelection: true,
    button: {
      name: "Compartir",
      toolbar: true,
      contextMenu: true,
      icon: Share,
    },
  } as const),

  CopyDownloadLink: defineFileAction({
    id: "copy_link",
    requiresSelection: true,
    fileFilter: (file) => !(file && "isDir" in file),
    button: {
      name: "Copiar enlace de descarga",
      contextMenu: true,
      icon: FbIconName.copy,
    },
  } as const),

  UploadFolder: defineFileAction({
    id: "upload_folder",
    requiresSelection: false,
    button: {
      name: "Subir carpeta",
      toolbar: true,
      icon: MaterialSymbolsFolder,
      group: "Add",
    },
  } as const),
};

type FbActionFullUnion =
  | (typeof CustomActions)[keyof typeof CustomActions]
  | FbActionUnion;

export const useFileAction = (
  { view, params: search }: FileListParams,
  session: Session,
) => {
  const queryClient = useQueryClient();

  const actions = useModalStore((state) => state.actions);

  const fileDialogOpen = useFileUploadStore(
    (state) => state.actions.setFileDialogOpen,
  );

  const setFolderDialogOpen = useFileUploadStore(
    (state) => state.actions.setFolderDialogOpen,
  );

  const uploadOpen = useFileUploadStore((state) => state.actions.setUploadOpen);

  const navigate = useNavigate();

  const moveFiles = $api.useMutation("post", "/files/move");

  return useCallback(() => {
    return async (data: MapFileActionsToData<FbActionFullUnion>) => {
      switch (data.id) {
        case CustomActions.MarkSpam.id: {
          try {
            await lifecycleChange(data.state.selectedFiles.map((item) => item.id), "spam");
            await queryClient.invalidateQueries({ queryKey: ["Files_list"] });
            void queryClient.invalidateQueries({ queryKey: ["Lifecycle"] });
            toast.success("Elementos movidos a Spam");
          } catch (error) { toast.error((error as Error).message); }
          break;
        }
        case FbActions.OpenFiles.id: {
          const { targetFile, files } = data.payload;

          const fileToOpen = targetFile ?? files[0];

          if (fileToOpen && FileHelper.isDirectory(fileToOpen)) {
            let qparams: FileListParams;

            if (view === "my-drive") {
              const basePath = search?.path ?? "/";
              qparams = {
                view,
                params: {
                  path: fileToOpen.chain
                    ? fileToOpen.path
                    : `${basePath === "/" ? "" : basePath}/${fileToOpen.name}`,
                },
              };
            } else {
              qparams = {
                view: "browse",
                params: { parentId: fileToOpen.id },
              };
            }
            navigate({
              to: "/$view",
              params: { view: qparams.view },
              search: qparams.params,
            });
          } else if (fileToOpen && FileHelper.isOpenable(fileToOpen)) {
            actions.set({
              open: true,
              currentFile: fileToOpen,
              operation: FbActions.OpenFiles.id,
            });
          }

          break;
        }
        case FbActions.DownloadFiles.id: {
          const { selectedFiles } = data.state;
          for (const file of selectedFiles) {
            if (!FileHelper.isDirectory(file)) {
              const { id, name } = file;
              const url = mediaUrl(
                id,
                name,
                search?.path || "",
                session.hash,
                true,
              );
              downloadFile(url, name);
            }
          }
          break;
        }
        case CustomActions.OpenInVLCPlayer.id: {
          const { selectedFiles } = data.state;
          const fileToOpen = selectedFiles[0];
          const { id, name } = fileToOpen!;
          const url = `vlc://${mediaUrl(id, name, search?.path || "", session.hash)}`;
          navigateToExternalUrl(url, false);
          break;
        }
        case CustomActions.OpenInPotPlayer.id: {
          const { selectedFiles } = data.state;
          const fileToOpen = selectedFiles[0];
          const { id, name } = fileToOpen!;
          const url = `potplayer://${mediaUrl(id, name, search?.path || "", session.hash)}`;
          navigateToExternalUrl(url, false);
          break;
        }
        case FbActions.RenameFile.id: {
          actions.set({
            open: true,
            currentFile: data.state.selectedFiles[0],
            operation: FbActions.RenameFile.id,
          });
          break;
        }
        case FbActions.DeleteFiles.id: {
          actions.set({
            open: true,
            selectedFiles: data.state.selectedFiles.map((item) => item.id),
            operation: FbActions.DeleteFiles.id,
          });
          break;
        }
        case FbActions.CreateFolder.id: {
          actions.set({
            open: true,
            operation: FbActions.CreateFolder.id,
            currentFile: {} as FileData,
          });
          break;
        }

        case CustomActions.ShareFiles.id: {
          if (data.state.selectedFiles.length !== 1) {
            toast.error("Selecciona un archivo o comparte la carpeta que los contiene.");
            break;
          }
          actions.set({
            open: true,
            operation: CustomActions.ShareFiles.id,
            currentFile: data.state.selectedFiles[0],
          });
          break;
        }

        case CustomActions.CopyDownloadLink.id: {
          const selections = data.state.selectedFilesForAction;
          const clipboardText = selections
            .filter((element) => !FileHelper.isDirectory(element))
            .map(({ id, name }) =>
              mediaUrl(id, name, search?.path || "", session.hash, true),
            )
            .join("\n");
          navigator.clipboard.writeText(clipboardText);
          break;
        }
        case FbActions.MoveFiles.id: {
          const { files, target } = data.payload;
          moveFiles
            .mutateAsync({
              body: {
                ids: files.map((file) => file?.id!),
                destinationParent: target.path || "/",
              },
            })
            .then(() => {
              toast.success(`${files.length} archivos movidos correctamente`);
              queryClient.invalidateQueries({
                queryKey: ["Files_list", "my-drive"],
              });
            });

          break;
        }

        case FbActions.UploadFiles.id: {
          fileDialogOpen(true);
          setFolderDialogOpen(false);
          uploadOpen(true);
          break;
        }

        case CustomActions.UploadFolder.id: {
          fileDialogOpen(false);
          setFolderDialogOpen(true);
          uploadOpen(true);
          break;
        }

        case FbActions.EnableListView.id:
        case FbActions.EnableGridView.id:
        case FbActions.EnableTileView.id: {
          localStorage.setItem("viewId", data.id);
          break;
        }
        case FbActions.SortFilesByName.id:
        case FbActions.SortFilesBySize.id:
        case FbActions.SortFilesByDate.id: {
            const currentSortState = view === "my-drive" ? getSortState() : sortViewMap[view];
            const sort = sortIdsMap[data.id];
            const order =
              (search?.sort || sortIdsMap[currentSortState.sortId]) === sort && (search?.order || currentSortState.order) === SortOrder.ASC
                ? SortOrder.DESC
                : SortOrder.ASC;
          if (view === "my-drive") {
            localStorage.setItem(
              "sort",
              JSON.stringify({ sortId: data.id, order }),
            );
          }
          navigate({ to: "/$view", params: { view }, search: { ...search, sort, order }, replace: true });
          break;
        }
        default:
          break;
      }
    };
  }, [view, search, session.hash, actions, navigate, queryClient, fileDialogOpen, setFolderDialogOpen, uploadOpen, moveFiles]);
};

export const useShareFileAction = (params: ShareListParams) => {
  const actions = useModalStore((state) => state.actions);
  const navigate = useNavigate();
  return useCallback(() => {
    return async (data: MapFileActionsToData<FbActionFullUnion>) => {
      switch (data.id) {
        case FbActions.OpenFiles.id: {
          const { targetFile, files } = data.payload;

          const fileToOpen = targetFile ?? files[0];

          if (fileToOpen && FileHelper.isDirectory(fileToOpen)) {
            const basePath = params?.path ?? "/";
            navigate({
              to: "/share/$id",
              params: {
                id: params.id,
              },
              search: {
                path: fileToOpen.chain
                  ? fileToOpen.path
                  : `${basePath === "/" ? "" : basePath}/${fileToOpen.name}`,
              },
            });
          } else if (fileToOpen && FileHelper.isOpenable(fileToOpen)) {
            actions.set({
              open: true,
              currentFile: fileToOpen,
              operation: FbActions.OpenFiles.id,
            });
          }

          break;
        }
        case FbActions.DownloadFiles.id: {
          const { selectedFiles } = data.state;
          for (const file of selectedFiles) {
            if (!FileHelper.isDirectory(file)) {
              const { id, name } = file;
              const url = sharedMediaUrl(params.id, id, name, true);
              downloadFile(url, name);
            }
          }
          break;
        }
        case CustomActions.OpenInVLCPlayer.id: {
          const { selectedFiles } = data.state;
          const fileToOpen = selectedFiles[0];
          const { id, name } = fileToOpen!;
          const url = `vlc://${sharedMediaUrl(params.id, id, name)}`;
          navigateToExternalUrl(url, false);
          break;
        }
        case CustomActions.OpenInPotPlayer.id: {
          const { selectedFiles } = data.state;
          const fileToOpen = selectedFiles[0];
          const { id, name } = fileToOpen!;
          const url = `potplayer://${sharedMediaUrl(params.id, id, name)}`;
          navigateToExternalUrl(url, false);
          break;
        }

        case CustomActions.CopyDownloadLink.id: {
          const selections = data.state.selectedFilesForAction;
          const clipboardText = selections
            .filter((element) => !FileHelper.isDirectory(element))
            .map(({ id, name }) => sharedMediaUrl(params.id, id, name, true))
            .join("\n");
          navigator.clipboard.writeText(clipboardText);
          break;
        }

        case FbActions.EnableListView.id:
        case FbActions.EnableGridView.id:
        case FbActions.EnableTileView.id: {
          localStorage.setItem("viewId", data.id);
          break;
        }
        default:
          break;
      }
    };
  }, [params.path, params.id]);
};

export const fileActions = [
  ...Object.keys(CustomActions).filter((key) => key !== "CopyDownloadLink").map(
    (t) => CustomActions[t as keyof typeof CustomActions],
  ),
];

export const sharefileActions = Object.keys(CustomActions)
  .map((t) => CustomActions[t as keyof typeof CustomActions])
  .filter((action) => action.id !== CustomActions.ShareFiles.id && action.id !== CustomActions.UploadFolder.id && action.id !== CustomActions.MarkSpam.id);
