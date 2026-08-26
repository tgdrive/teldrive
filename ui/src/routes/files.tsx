import { Button, Dropdown, Input, Label, Spinner, TextField } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useDeferredValue, useEffect, useEffectEvent, useRef, useState } from "react";
import { DropZone, FileTrigger, type Selection } from "react-aria-components";
import { toast } from "sonner";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import CopyIcon from "~icons/gravity-ui/copy";
import CopyLinkIcon from "~icons/gravity-ui/copy-arrow-right";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import MoveIcon from "~icons/gravity-ui/folder-arrow-right";
import LinkIcon from "~icons/gravity-ui/link";
import PencilIcon from "~icons/gravity-ui/pencil";
import PlusIcon from "~icons/gravity-ui/plus";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import CloseIcon from "~icons/gravity-ui/xmark";
import { currentUserQueryOptions } from "../auth/queries";
import { userMessage } from "../api/errors";
import type { FileEntry } from "../api/types";
import { AppDialog } from "../components/dialogs/app-dialog";
import { BackgroundUploadDialog } from "../components/background-upload-dialog";
import { FilePreviewDialog, isPreviewable } from "../components/file-preview-dialog";
import { Page, PageContent } from "../components/page";
import { FileBrowser } from "../features/files/file-browser";
import { ShareDialog } from "../features/files/share-dialog";
import { FolderPicker } from "../features/files/folder-picker";
import { absoluteFileDownloadUrl, copyText, startFileDownload } from "../features/files/download";
import { useFileActions } from "../features/files/mutations";
import { useInfiniteFilePages } from "../features/files/queries";
import { useUploadStore } from "../features/uploads/store";

type FilesSearch = {
  path: string;
  parentId?: string;
  query: string;
  view: "list" | "grid";
};

export const Route = createFileRoute("/files")({
  validateSearch: (search: Record<string, unknown>): FilesSearch => ({
    path: typeof search.path === "string" && search.path ? search.path : "/",
    parentId: typeof search.parentId === "string" ? search.parentId : undefined,
    query: typeof search.query === "string" ? search.query : "",
    view: search.view === "grid" ? "grid" : "list",
  }),
  component: FilesPage,
  pendingComponent: () => (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  ),
});

function FilesPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data: currentUser } = useQuery(currentUserQueryOptions());
  const canLocalImport = Boolean(currentUser?.capabilities.includes("system.localImport"));
  const [queryDraft, setQueryDraft] = useState(search.query);
  const deferredQuery = useDeferredValue(queryDraft.trim());
  const [selectedKeys, setSelectedKeys] = useState<Selection>(new Set());
  const [folderDialogOpen, setFolderDialogOpen] = useState(false);
  const [backgroundUploadOpen, setBackgroundUploadOpen] = useState(false);
  const [folderName, setFolderName] = useState("");
  const [previewFile, setPreviewFile] = useState<FileEntry>();
  const [renameFile, setRenameFile] = useState<FileEntry>();
  const [renameName, setRenameName] = useState("");
  const [moveDialogOpen, setMoveDialogOpen] = useState(false);
  const [shareFile, setShareFile] = useState<FileEntry>();
  const searchInputRef = useRef<HTMLInputElement>(null);
  const uploadFilesTriggerRef = useRef<HTMLButtonElement>(null);
  const uploadFolderTriggerRef = useRef<HTMLButtonElement>(null);
  const enqueue = useUploadStore((state) => state.enqueue);
  const fileActions = useFileActions();

  const fileQuery = useInfiniteFilePages(
    {
      path: search.path,
      parentId: search.parentId,
      q: search.query || undefined,
      sort: "name",
      order: "asc",
      view: search.view,
    },
    "active",
  );
  const files = fileQuery.data?.pages.flatMap((page) => page.items) ?? [];

  useEffect(() => setQueryDraft(search.query), [search.query]);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (deferredQuery !== search.query) {
        navigate({ search: { ...search, query: deferredQuery }, replace: true });
      }
    }, 250);
    return () => window.clearTimeout(timer);
  }, [deferredQuery, navigate, search]);
  useEffect(() => setSelectedKeys(new Set()), [search.parentId, search.path, search.query]);
  const selectedCount = selectedKeys === "all" ? files.length : selectedKeys.size;
  const selectedIds =
    selectedKeys === "all" ? files.map((file) => file.id) : Array.from(selectedKeys, String);
  const selectedFiles = files.filter((file) => selectedIds.includes(file.id));
  const selectedOnlyFiles =
    selectedFiles.length > 0 && selectedFiles.every((file) => file.kind === "file");
  const singleSelectedFile = selectedFiles.length === 1 ? selectedFiles[0] : undefined;

  const createFolder = async () => {
    const name = folderName.trim();
    if (!name) return;
    try {
      await fileActions.createFolder(name, search.parentId);
      setFolderName("");
      setFolderDialogOpen(false);
      toast.success("Folder created");
    } catch (error) {
      toast.error("Folder could not be created", { description: userMessage(error) });
    }
  };

  const trashFiles = async (ids: string[]) => {
    if (ids.length === 0) return;
    try {
      await fileActions.bulkTrash(ids);
      setSelectedKeys(new Set());
      toast.success(`${ids.length} item${ids.length === 1 ? "" : "s"} moved to trash`);
    } catch (error) {
      toast.error("Items could not be moved to trash", { description: userMessage(error) });
    }
  };

  const trashSelected = async () => trashFiles(selectedIds);

  const renameSelected = async () => {
    if (!renameFile || !renameName.trim()) return;
    try {
      await fileActions.rename(renameFile, renameName.trim());
      setRenameFile(undefined);
      setRenameName("");
      setSelectedKeys(new Set());
      toast.success("Item renamed");
    } catch (error) {
      toast.error("Item could not be renamed", { description: userMessage(error) });
    }
  };

  const duplicateFile = async (file: FileEntry) => {
    try {
      await fileActions.copy(file, search.parentId, `${file.name} copy`, "rename");
      setSelectedKeys(new Set());
      toast.success("Item duplicated");
    } catch (error) {
      toast.error("Item could not be duplicated", { description: userMessage(error) });
    }
  };

  const duplicateSelected = async () => {
    if (singleSelectedFile) await duplicateFile(singleSelectedFile);
  };

  const moveSelected = async (parentId?: string) => {
    if (selectedFiles.length === 0) return;
    try {
      if (selectedFiles.length === 1) await fileActions.move(selectedFiles[0], parentId, "rename");
      else await fileActions.bulkMove(selectedIds, parentId);
      setMoveDialogOpen(false);
      setSelectedKeys(new Set());
      toast.success(`${selectedFiles.length} item${selectedFiles.length === 1 ? "" : "s"} moved`);
    } catch (error) {
      toast.error("Selected items could not be moved", { description: userMessage(error) });
    }
  };

  const navigateToParent = () => {
    if (search.path === "/") return;
    const parts = search.path.split("/").filter(Boolean);
    const parentPath = parts.length <= 1 ? "/" : `/${parts.slice(0, -1).join("/")}`;
    navigate({ search: { path: parentPath, parentId: undefined, query: "", view: search.view } });
  };

  const handleFileShortcut = useEffectEvent((event: KeyboardEvent) => {
    if (event.defaultPrevented) return;
    if (event.key === "Escape") {
      setFolderDialogOpen(false);
      setRenameFile(undefined);
      setMoveDialogOpen(false);
      setShareFile(undefined);
      setPreviewFile(undefined);
      setSelectedKeys(new Set());
      return;
    }
    if (isEditableTarget(event.target)) return;
    const command = event.ctrlKey || event.metaKey;
    if (event.key === "F2" && singleSelectedFile) {
      event.preventDefault();
      setRenameFile(singleSelectedFile);
      setRenameName(singleSelectedFile.name);
      return;
    }
    if (event.key === "Delete" && selectedIds.length > 0) {
      event.preventDefault();
      void trashSelected();
      return;
    }
    if (command && event.shiftKey && event.key.toLowerCase() === "n") {
      event.preventDefault();
      setFolderDialogOpen(true);
      return;
    }
    if (command && event.key.toLowerCase() === "f") {
      event.preventDefault();
      const input =
        searchInputRef.current ??
        document.querySelector<HTMLInputElement>('input[aria-label="Search this folder"]');
      input?.focus();
      input?.select();
      return;
    }
    if (event.altKey && event.key === "ArrowUp") {
      event.preventDefault();
      navigateToParent();
    }
  });

  useEffect(() => {
    document.addEventListener("keydown", handleFileShortcut, true);
    return () => document.removeEventListener("keydown", handleFileShortcut, true);
  }, []);

  const openFile = (file: FileEntry) => {
    if (file.kind === "folder") {
      const nextPath = joinPath(search.path, file.name);
      navigate({
        search: {
          path: nextPath,
          parentId: file.id,
          query: "",
          view: search.view,
        },
      });
      setSelectedKeys(new Set());
      return;
    }
    if (isPreviewable(file)) {
      setPreviewFile(file);
      return;
    }
    startFileDownload(file);
  };

  const copyDownloadLinks = async () => {
    try {
      await copyText(selectedFiles.map(absoluteFileDownloadUrl).join("\n"));
      toast.success(
        `${selectedFiles.length} download link${selectedFiles.length === 1 ? "" : "s"} copied`,
      );
    } catch (error) {
      toast.error("Download links could not be copied", { description: userMessage(error) });
    }
  };

  return (
    <Page className="h-full min-h-0 gap-0 overflow-x-hidden">
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <DropZone
          data-testid="file-drop-zone"
          aria-label={`Upload files into ${search.path}`}
          getDropOperation={() => "copy"}
          className="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden outline-none"
          onDrop={async (event) => {
            const dropped = await Promise.all(
              event.items.filter((item) => item.kind === "file").map((item) => item.getFile()),
            );
            if (dropped.length) enqueue(dropped, search.parentId, search.path);
          }}
        >
          {({ isDropTarget }) => (
            <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-x-hidden">
              {isDropTarget ? (
                <div className="shrink-0 rounded-xl border-2 border-dashed border-accent bg-accent/10 px-4 py-4 text-center text-sm font-medium text-accent sm:px-6 sm:py-6">
                  Drop files to upload into {search.path}
                </div>
              ) : null}
              <FileBrowser
                files={files}
                path={search.path}
                rootLabel="My files"
                view={search.view}
                query={queryDraft}
                loading={fileQuery.isPending}
                onQueryChange={setQueryDraft}
                onNavigatePath={(path) =>
                  navigate({ search: { path, query: "", view: search.view }, replace: true })
                }
                onViewChange={(view) => navigate({ search: { ...search, view }, replace: true })}
                onOpen={openFile}
                searchInputRef={searchInputRef}
                selection={{
                  selectedKeys,
                  onSelectionChange: setSelectedKeys,
                  onClearSelection: () => setSelectedKeys(new Set()),
                }}
                hasNextPage={fileQuery.hasNextPage}
                isLoadingMore={fileQuery.isFetchingNextPage}
                onLoadMore={() => {
                  if (fileQuery.hasNextPage && !fileQuery.isFetchingNextPage) {
                    void fileQuery.fetchNextPage();
                  }
                }}
                toolbar={
                  <>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="secondary"
                      aria-label="New folder"
                      onPress={() => setFolderDialogOpen(true)}
                    >
                      <PlusIcon className="size-4" />
                    </Button>
                    <Dropdown>
                      <Button isIconOnly size="sm" variant="primary" aria-label="Upload">
                        <UploadIcon className="size-4" />
                      </Button>
                      <Dropdown.Popover className="min-w-52">
                        <Dropdown.Menu
                          aria-label="Upload"
                          onAction={(key) => {
                            if (key === "files") uploadFilesTriggerRef.current?.click();
                            if (key === "folder") uploadFolderTriggerRef.current?.click();
                            if (key === "background") setBackgroundUploadOpen(true);
                          }}
                        >
                          <Dropdown.Item id="files" textValue="Upload files">
                            <FileIcon className="size-4" />
                            <Label>Upload files</Label>
                          </Dropdown.Item>
                          <Dropdown.Item id="folder" textValue="Upload folder">
                            <FolderIcon className="size-4" />
                            <Label>Upload folder</Label>
                          </Dropdown.Item>
                          {canLocalImport ? (
                            <Dropdown.Item id="background" textValue="Background upload">
                              <UploadIcon className="size-4" />
                              <Label>Background upload</Label>
                            </Dropdown.Item>
                          ) : null}
                        </Dropdown.Menu>
                      </Dropdown.Popover>
                    </Dropdown>
                    <span className="hidden" aria-hidden="true">
                      <FileTrigger
                        allowsMultiple
                        onSelect={(list) => {
                          if (list?.length) enqueue(Array.from(list), search.parentId, search.path);
                        }}
                      >
                        <Button ref={uploadFilesTriggerRef}>Choose upload files</Button>
                      </FileTrigger>
                      <FileTrigger
                        acceptDirectory
                        allowsMultiple
                        onSelect={(list) => {
                          if (list?.length) enqueue(Array.from(list), search.parentId, search.path);
                        }}
                      >
                        <Button ref={uploadFolderTriggerRef}>Choose upload folder</Button>
                      </FileTrigger>
                    </span>
                  </>
                }
                selectionOverlay={
                  selectedCount > 0 ? (
                    <div className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center px-4">
                      <div className="pointer-events-auto flex max-w-full items-center gap-1.5 overflow-x-auto rounded-full border border-border bg-surface/95 p-1.5 shadow-xl backdrop-blur">
                        <span className="shrink-0 rounded-full bg-accent/10 px-3 py-2 text-sm font-medium text-accent">
                          {selectedCount} selected
                        </span>
                        {singleSelectedFile ? (
                          <>
                            <Button
                              isIconOnly
                              size="sm"
                              variant="ghost"
                              aria-label="Rename selected item"
                              isDisabled={fileActions.pending}
                              onPress={() => {
                                setRenameFile(singleSelectedFile);
                                setRenameName(singleSelectedFile.name);
                              }}
                            >
                              <PencilIcon className="size-4" />
                            </Button>
                            <Button
                              isIconOnly
                              size="sm"
                              variant="ghost"
                              aria-label="Duplicate selected item"
                              isDisabled={fileActions.pending}
                              onPress={() => void duplicateSelected()}
                            >
                              <CopyIcon className="size-4" />
                            </Button>
                            <Button
                              isIconOnly
                              size="sm"
                              variant="ghost"
                              aria-label="Share selected item"
                              onPress={() => {
                                setShareFile(singleSelectedFile);
                              }}
                            >
                              <LinkIcon className="size-4" />
                            </Button>
                            {singleSelectedFile.kind === "file" ? (
                              <Button
                                isIconOnly
                                size="sm"
                                variant="ghost"
                                aria-label="Download selected file"
                                onPress={() => startFileDownload(singleSelectedFile)}
                              >
                                <DownloadIcon className="size-4" />
                              </Button>
                            ) : null}
                          </>
                        ) : null}
                        {selectedOnlyFiles ? (
                          <Button
                            isIconOnly
                            size="sm"
                            variant="ghost"
                            aria-label={
                              selectedFiles.length === 1
                                ? "Copy selected file download link"
                                : "Copy selected files download links"
                            }
                            onPress={() => void copyDownloadLinks()}
                          >
                            <CopyLinkIcon className="size-4" />
                          </Button>
                        ) : null}
                        <Button
                          isIconOnly
                          size="sm"
                          variant="ghost"
                          aria-label="Move selected items"
                          isDisabled={fileActions.pending}
                          onPress={() => setMoveDialogOpen(true)}
                        >
                          <MoveIcon className="size-4" />
                        </Button>
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
                  ) : null
                }
              />
            </div>
          )}
        </DropZone>
      </PageContent>

      <AppDialog
        open={folderDialogOpen}
        onOpenChange={setFolderDialogOpen}
        title="Create folder"
        description={`Create a folder inside ${search.path}.`}
        size="md"
        footer={
          <>
            <Button variant="secondary" onPress={() => setFolderDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              isDisabled={!folderName.trim() || fileActions.pending}
              onPress={() => void createFolder()}
            >
              Create folder
            </Button>
          </>
        }
      >
        <TextField
          autoFocus
          value={folderName}
          onChange={setFolderName}
          onKeyDown={(event) => {
            if (event.key === "Enter") void createFolder();
          }}
        >
          <Label>Folder name</Label>
          <Input placeholder="New folder" />
        </TextField>
      </AppDialog>

      <BackgroundUploadDialog
        open={backgroundUploadOpen}
        onOpenChange={setBackgroundUploadOpen}
        currentPath={search.path}
      />

      <AppDialog
        open={Boolean(renameFile)}
        onOpenChange={(open) => {
          if (!open) setRenameFile(undefined);
        }}
        title="Rename item"
        size="md"
        footer={
          <>
            <Button variant="secondary" onPress={() => setRenameFile(undefined)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              isDisabled={!renameName.trim() || fileActions.pending}
              onPress={() => void renameSelected()}
            >
              Rename
            </Button>
          </>
        }
      >
        <TextField
          autoFocus
          value={renameName}
          onChange={setRenameName}
          onKeyDown={(event) => {
            if (event.key === "Enter") void renameSelected();
          }}
        >
          <Label>New name</Label>
          <Input />
        </TextField>
      </AppDialog>

      <AppDialog
        open={moveDialogOpen}
        onOpenChange={setMoveDialogOpen}
        title={`Move ${selectedCount} item${selectedCount === 1 ? "" : "s"}`}
        description="Choose the destination folder."
      >
        <FolderPicker initialPath="/" onConfirm={(parentId) => void moveSelected(parentId)} />
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

function joinPath(parent: string, name: string) {
  return `${parent === "/" ? "" : parent}/${name}`.replace(/\/+/g, "/") || "/";
}

function isEditableTarget(target: EventTarget | null) {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      Boolean(target.closest("input, textarea, select, [contenteditable='true']")))
  );
}
