import { Button, Input, Label, Spinner, TextField } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { useDeferredValue, useEffect, useRef, useState } from "react";
import { FileTrigger, type Selection } from "react-aria-components";
import { toast } from "sonner";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import PencilIcon from "~icons/gravity-ui/pencil";
import PlusIcon from "~icons/gravity-ui/plus";
import LinkIcon from "~icons/gravity-ui/link";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import CloseIcon from "~icons/gravity-ui/xmark";
import { $api, fetchClient } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FilePreviewDialog, isPreviewable } from "@/components/file-preview-dialog";
import { Page, PageContent } from "@/components/page";
import { startFileDownload } from "@/features/files/download";
import { FileBrowser, type FileBrowserView } from "@/features/files/file-browser";
import { useFileActions } from "@/features/files/mutations";
import { ShareDialog } from "@/features/files/share-dialog";
import { useUploadStore } from "@/features/uploads/store";
import { getQueryClient } from "@/lib/queryClient";

type Permission = "read" | "edit";

export type SharedBrowserSearch = {
  path: string;
  parentId?: string;
  query: string;
  view: FileBrowserView;
  permission?: Permission;
};

type SharedFileBrowserProps = {
  mode: "shared" | "with-me";
  search: SharedBrowserSearch;
  navigate: (search: SharedBrowserSearch, replace?: boolean) => void;
};

export function sharedBrowserSearch(search: Record<string, unknown>): SharedBrowserSearch {
  return {
    path: typeof search.path === "string" && search.path ? search.path : "/",
    parentId: typeof search.parentId === "string" ? search.parentId : undefined,
    query: typeof search.query === "string" ? search.query : "",
    view: search.view === "grid" ? "grid" : "list",
    permission:
      search.permission === "edit" ? "edit" : search.permission === "read" ? "read" : undefined,
  };
}

export function SharedFileBrowser({ mode, search, navigate }: SharedFileBrowserProps) {
  const [queryDraft, setQueryDraft] = useState(search.query);
  const deferredQuery = useDeferredValue(queryDraft.trim());
  const [previewFile, setPreviewFile] = useState<FileEntry>();
  const [selectedKeys, setSelectedKeys] = useState<Selection>(new Set());
  const [folderDialogOpen, setFolderDialogOpen] = useState(false);
  const [folderName, setFolderName] = useState("");
  const [renameFile, setRenameFile] = useState<FileEntry>();
  const [renameName, setRenameName] = useState("");
  const [shareFile, setShareFile] = useState<FileEntry>();
  const uploadTriggerRef = useRef<HTMLButtonElement>(null);
  const enqueue = useUploadStore((state) => state.enqueue);
  const fileActions = useFileActions();
  const atRoot = !search.parentId;

  const sharedQuery = useQuery({
    ...$api.queryOptions("get", "/v1/shared"),
    enabled: atRoot && mode === "shared",
  });
  const sharedWithMeQuery = useQuery({
    ...$api.queryOptions("get", "/v1/shared/with-me"),
    enabled: atRoot && mode === "with-me",
  });
  const childrenQuery = useQuery({
    ...$api.queryOptions("get", "/v1/files", {
      params: {
        query: {
          parentId: search.parentId,
          search: search.query || undefined,
          sort: "name",
          order: "asc",
          status: "active",
          limit: 200,
        },
      },
    }),
    enabled: !atRoot,
  });

  useEffect(() => setQueryDraft(search.query), [search.query]);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (deferredQuery !== search.query) navigate({ ...search, query: deferredQuery }, true);
    }, 250);
    return () => window.clearTimeout(timer);
  }, [deferredQuery, navigate, search]);
  useEffect(() => setSelectedKeys(new Set()), [search.parentId, search.path, search.query]);

  const incomingEntries = sharedWithMeQuery.data ?? [];
  const incomingPermission = new Map(
    incomingEntries.map((entry) => [entry.file.id, entry.permission]),
  );
  const roots =
    mode === "with-me" ? incomingEntries.map((entry) => entry.file) : (sharedQuery.data ?? []);
  const rootFiles = roots.filter(
    (file) =>
      !search.query || file.name.toLocaleLowerCase().includes(search.query.toLocaleLowerCase()),
  );
  const files = atRoot ? rootFiles : (childrenQuery.data?.items ?? []);
  const loading = atRoot
    ? mode === "with-me"
      ? sharedWithMeQuery.isPending
      : sharedQuery.isPending
    : childrenQuery.isPending;
  const rootLabel = mode === "with-me" ? "Shared with me" : "Shared";

  const permissionFor = (file: FileEntry): Permission => {
    if (mode === "shared") return "edit";
    if (!atRoot) return search.permission ?? "read";
    return incomingPermission.get(file.id) ?? "read";
  };

  const selectedIds =
    selectedKeys === "all" ? files.map((file) => file.id) : Array.from(selectedKeys, String);
  const selectedFiles = files.filter((file) => selectedIds.includes(file.id));
  const selectedCount = selectedFiles.length;
  const singleSelected = selectedFiles.length === 1 ? selectedFiles[0] : undefined;
  const selectedWritable =
    selectedFiles.length > 0 && selectedFiles.every((file) => permissionFor(file) === "edit");
  const currentFolderEditable = !atRoot && (mode === "shared" || search.permission === "edit");

  const refreshRoots = async () => {
    if (mode === "with-me") await sharedWithMeQuery.refetch();
    else await sharedQuery.refetch();
  };

  const createFolder = async () => {
    const name = folderName.trim();
    if (!name || !search.parentId || !currentFolderEditable) return;
    try {
      await fileActions.createFolder(name, search.parentId);
      setFolderName("");
      setFolderDialogOpen(false);
      await childrenQuery.refetch();
      toast.success("Folder created");
    } catch (error) {
      toast.error("Folder could not be created", {
        description: userMessage(error),
      });
    }
  };

  const renameSelected = async () => {
    if (!renameFile || !renameName.trim() || permissionFor(renameFile) !== "edit") return;
    try {
      await fileActions.rename(renameFile, renameName.trim());
      setRenameFile(undefined);
      setRenameName("");
      setSelectedKeys(new Set());
      if (atRoot) await refreshRoots();
      else await childrenQuery.refetch();
      toast.success("Item renamed");
    } catch (error) {
      toast.error("Item could not be renamed", {
        description: userMessage(error),
      });
    }
  };

  const trashSelected = async () => {
    if (!selectedWritable) return;
    try {
      await Promise.all(selectedFiles.map((file) => fileActions.trash(file.id)));
      setSelectedKeys(new Set());
      if (atRoot) await refreshRoots();
      else await childrenQuery.refetch();
      toast.success(
        `${selectedFiles.length} item${selectedFiles.length === 1 ? "" : "s"} moved to trash`,
      );
    } catch (error) {
      toast.error("Items could not be moved to trash", {
        description: userMessage(error),
      });
    }
  };

  const stopSharingSelected = async () => {
    if (mode !== "shared" || !atRoot || !selectedFiles.length) return;
    try {
      for (const file of selectedFiles) {
        const { data: grants } = await fetchClient.GET("/v1/files/{fileId}/grants", {
          params: { path: { fileId: file.id } },
        });

        const shareIds: string[] = [];
        let cursor: string | undefined;
        do {
          const { data: page } = await fetchClient.GET("/v1/files/{fileId}/shares", {
            params: {
              path: { fileId: file.id },
              query: { limit: 200, cursor },
            },
          });
          shareIds.push(...(page?.items ?? []).map((share) => share.id));
          cursor = page?.nextCursor;
        } while (cursor);

        await Promise.all([
          ...(grants ?? []).map((grant) =>
            fetchClient.DELETE("/v1/grants/{grantId}", {
              params: { path: { grantId: grant.id } },
            }),
          ),
          ...shareIds.map((shareId) =>
            fetchClient.DELETE("/v1/shares/{shareId}", {
              params: { path: { shareId } },
            }),
          ),
        ]);

        const queryClient = getQueryClient();
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: $api.queryOptions("get", "/v1/files/{fileId}/grants", {
              params: { path: { fileId: file.id } },
            }).queryKey,
          }),
          queryClient.invalidateQueries({
            queryKey: $api.queryOptions("get", "/v1/files/{fileId}/shares", {
              params: { path: { fileId: file.id }, query: { limit: 200 } },
            }).queryKey,
          }),
        ]);
      }

      setSelectedKeys(new Set());
      await refreshRoots();
      toast.success(
        `${selectedFiles.length} item${selectedFiles.length === 1 ? "" : "s"} no longer shared`,
      );
    } catch (error) {
      toast.error("Sharing could not be removed", {
        description: userMessage(error),
      });
    }
  };

  const openFile = (file: FileEntry) => {
    if (file.kind === "folder") {
      navigate({
        path: joinPath(search.path, file.name),
        parentId: file.id,
        query: "",
        view: search.view,
        permission: mode === "with-me" ? permissionFor(file) : undefined,
      });
      return;
    }
    if (isPreviewable(file)) {
      setPreviewFile(file);
      return;
    }
    startFileDownload(file);
  };

  return (
    <Page className="h-full min-h-0 gap-0 overflow-x-hidden">
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <FileBrowser
          files={files}
          path={search.path}
          rootLabel={rootLabel}
          view={search.view}
          query={queryDraft}
          loading={loading}
          onQueryChange={setQueryDraft}
          onNavigatePath={(path) => {
            if (path === "/") {
              navigate({ path: "/", query: "", view: search.view }, true);
              return;
            }
            const parts = path.split("/").filter(Boolean);
            if (parts.length < search.path.split("/").filter(Boolean).length) {
              navigate({ path: "/", query: "", view: search.view }, true);
            }
          }}
          onViewChange={(view) => navigate({ ...search, view }, true)}
          onOpen={openFile}
          selection={{
            selectedKeys,
            onSelectionChange: setSelectedKeys,
            onClearSelection: () => setSelectedKeys(new Set()),
          }}
          toolbar={
            currentFolderEditable ? (
              <>
                <Button
                  isIconOnly
                  size="sm"
                  variant="secondary"
                  aria-label="New folder"
                  isDisabled={fileActions.pending}
                  onPress={() => setFolderDialogOpen(true)}
                >
                  <PlusIcon className="size-4" />
                </Button>
                <Button
                  isIconOnly
                  size="sm"
                  variant="primary"
                  aria-label="Upload files"
                  isDisabled={fileActions.pending}
                  onPress={() => uploadTriggerRef.current?.click()}
                >
                  <UploadIcon className="size-4" />
                </Button>
                <span className="hidden" aria-hidden="true">
                  <FileTrigger
                    allowsMultiple
                    onSelect={(list) => {
                      if (list?.length) enqueue(Array.from(list), search.parentId, search.path);
                    }}
                  >
                    <Button ref={uploadTriggerRef}>Choose upload files</Button>
                  </FileTrigger>
                </span>
              </>
            ) : undefined
          }
          selectionOverlay={
            selectedCount > 0 ? (
              <div className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center px-4">
                <div className="pointer-events-auto flex max-w-full items-center gap-1.5 overflow-x-auto rounded-full border border-border bg-surface/95 p-1.5 shadow-xl backdrop-blur">
                  <span className="shrink-0 rounded-full bg-accent/10 px-3 py-2 text-sm font-medium text-accent">
                    {selectedCount} selected
                  </span>
                  {singleSelected && selectedWritable ? (
                    <Button
                      isIconOnly
                      size="sm"
                      variant="ghost"
                      aria-label="Rename selected item"
                      isDisabled={fileActions.pending}
                      onPress={() => {
                        setRenameFile(singleSelected);
                        setRenameName(singleSelected.name);
                      }}
                    >
                      <PencilIcon className="size-4" />
                    </Button>
                  ) : null}
                  {singleSelected && mode === "shared" ? (
                    <Button
                      isIconOnly
                      size="sm"
                      variant="ghost"
                      aria-label="Share selected item"
                      onPress={() => setShareFile(singleSelected)}
                    >
                      <LinkIcon className="size-4" />
                    </Button>
                  ) : null}
                  {singleSelected?.kind === "file" ? (
                    <Button
                      isIconOnly
                      size="sm"
                      variant="ghost"
                      aria-label="Download selected file"
                      onPress={() => startFileDownload(singleSelected)}
                    >
                      <DownloadIcon className="size-4" />
                    </Button>
                  ) : null}
                  {mode === "shared" && atRoot && selectedCount > 0 ? (
                    <Button
                      isIconOnly
                      size="sm"
                      variant="danger"
                      aria-label="Stop sharing selected items"
                      onPress={() => void stopSharingSelected()}
                    >
                      <LinkIcon className="size-4" />
                    </Button>
                  ) : mode === "with-me" && selectedWritable ? (
                    <Button
                      isIconOnly
                      size="sm"
                      variant="danger"
                      aria-label="Move selected items to trash"
                      isDisabled={fileActions.pending}
                      onPress={() => void trashSelected()}
                    >
                      <TrashIcon className="size-4" />
                    </Button>
                  ) : null}
                  <Button
                    isIconOnly
                    size="sm"
                    variant="ghost"
                    aria-label="Clear selection"
                    onPress={() => setSelectedKeys(new Set())}
                  >
                    <CloseIcon className="size-4" />
                  </Button>
                </div>
              </div>
            ) : undefined
          }
          emptyHint={
            atRoot
              ? mode === "with-me"
                ? "Files and folders shared with you appear here."
                : "Files and folders you shared appear here."
              : "This shared folder is empty."
          }
        />
      </PageContent>

      <AppDialog
        open={folderDialogOpen}
        onOpenChange={setFolderDialogOpen}
        title="Create folder"
        footer={
          <>
            <Button variant="secondary" onPress={() => setFolderDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              isDisabled={!folderName.trim()}
              onPress={() => void createFolder()}
            >
              Create folder
            </Button>
          </>
        }
      >
        <TextField value={folderName} onChange={setFolderName}>
          <Label>Name</Label>
          <Input autoFocus />
        </TextField>
      </AppDialog>

      <AppDialog
        open={Boolean(renameFile)}
        onOpenChange={(open) => {
          if (!open) setRenameFile(undefined);
        }}
        title="Rename item"
        footer={
          <>
            <Button variant="secondary" onPress={() => setRenameFile(undefined)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              isDisabled={!renameName.trim()}
              onPress={() => void renameSelected()}
            >
              Rename
            </Button>
          </>
        }
      >
        <TextField value={renameName} onChange={setRenameName}>
          <Label>Name</Label>
          <Input autoFocus />
        </TextField>
      </AppDialog>

      <ShareDialog
        file={shareFile}
        onOpenChange={(open) => {
          if (!open) setShareFile(undefined);
        }}
      />

      <FilePreviewDialog
        file={previewFile}
        onOpenChange={(open) => {
          if (!open) setPreviewFile(undefined);
        }}
      />
    </Page>
  );
}

export function SharedPageSpinner() {
  return (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  );
}

function joinPath(parent: string, name: string) {
  return `${parent === "/" ? "" : parent}/${name}`.replace(/\/+/g, "/") || "/";
}
