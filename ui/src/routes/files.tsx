import {
  Button,
  Card,
  Checkbox,
  Dropdown,
  Input,
  InputGroup,
  Label,
  Spinner,
  TextField,
} from "@heroui/react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useDeferredValue, useEffect, useEffectEvent, useRef, useState } from "react";
import {
  Collection,
  DropZone,
  FileTrigger,
  GridLayout,
  GridList,
  GridListItem,
  GridListLoadMoreItem,
  ListLayout,
  type Selection,
  Virtualizer,
} from "react-aria-components";
import { toast } from "sonner";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import CopyIcon from "~icons/gravity-ui/copy";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import MoveIcon from "~icons/gravity-ui/folder-arrow-right";
import GridIcon from "~icons/gravity-ui/layout-cells";
import LinkIcon from "~icons/gravity-ui/link";
import ListIcon from "~icons/gravity-ui/list-ul";
import SearchIcon from "~icons/gravity-ui/magnifier";
import PencilIcon from "~icons/gravity-ui/pencil";
import PlusIcon from "~icons/gravity-ui/plus";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import CloseIcon from "~icons/gravity-ui/xmark";
import { $api } from "../api/client";
import { userMessage } from "../api/errors";
import type { FileEntry } from "../api/types";
import { AppDialog } from "../components/dialogs/app-dialog";
import { BackgroundUploadDialog } from "../components/background-upload-dialog";
import { FilePreviewDialog, isPreviewable } from "../components/file-preview-dialog";
import { Page, PageContent } from "../components/page";
import { FolderPicker } from "../features/files/folder-picker";
import { useFileActions } from "../features/files/mutations";
import { useInfiniteFilePages } from "../features/files/queries";
import { newIdempotencyKey } from "../features/shared/idempotency";
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
  const [sharePassword, setSharePassword] = useState("");
  const [shareUrl, setShareUrl] = useState("");
  const searchInputRef = useRef<HTMLInputElement>(null);
  const uploadFilesTriggerRef = useRef<HTMLButtonElement>(null);
  const uploadFolderTriggerRef = useRef<HTMLButtonElement>(null);
  const enqueue = useUploadStore((state) => state.enqueue);
  const fileActions = useFileActions();
  const createShareMutation = $api.useMutation("post", "/v1/files/{fileId}/shares");

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
    const anchor = document.createElement("a");
    anchor.href = `/api/v1/files/${encodeURIComponent(file.id)}/content`;
    anchor.download = file.name;
    anchor.click();
  };

  const createShare = async () => {
    if (!shareFile) return;
    try {
      const result = await createShareMutation.mutateAsync({
        params: {
          path: { fileId: shareFile.id },
          header: { "Idempotency-Key": newIdempotencyKey() },
        },
        body: { password: sharePassword.trim() || undefined },
      });
      const publicUrl = new URL(result.publicUrl, window.location.origin).toString();
      setShareUrl(publicUrl);
      await navigator.clipboard.writeText(publicUrl);
      toast.success("Share link created and copied");
    } catch (error) {
      toast.error("Share link could not be created", { description: userMessage(error) });
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
              <Card className="relative flex min-h-0 min-w-0 flex-1 flex-col gap-0 overflow-hidden border border-border bg-surface/80 shadow-sm">
                <Card.Header className="shrink-0 border-b border-border px-3 py-3 sm:px-4">
                  <div className="flex min-w-0 items-center gap-2">
                    <div className="min-w-0 flex-1 overflow-hidden">
                      <Breadcrumb path={search.path} view={search.view} />
                    </div>
                    <TextField
                      name="file-search"
                      aria-label="Search this folder"
                      value={queryDraft}
                      onChange={setQueryDraft}
                      className="min-w-0 w-40 shrink sm:w-56 lg:w-72"
                    >
                      <InputGroup>
                        <InputGroup.Prefix>
                          <SearchIcon className="size-4 text-muted" />
                        </InputGroup.Prefix>
                        <InputGroup.Input
                          ref={searchInputRef}
                          aria-label="Search this folder"
                          placeholder="Search this folder"
                        />
                      </InputGroup>
                    </TextField>
                    <div className="flex shrink-0 items-center gap-1.5">
                      <Button
                        isIconOnly
                        size="sm"
                        variant="secondary"
                        aria-label="New folder"
                        onPress={() => setFolderDialogOpen(true)}
                      >
                        <PlusIcon className="size-4" />
                      </Button>
                      <Button
                        isIconOnly
                        size="sm"
                        variant={search.view === "list" ? "secondary" : "ghost"}
                        aria-label="List view"
                        onPress={() =>
                          navigate({ search: { ...search, view: "list" }, replace: true })
                        }
                      >
                        <ListIcon className="size-4" />
                      </Button>
                      <Button
                        isIconOnly
                        size="sm"
                        variant={search.view === "grid" ? "secondary" : "ghost"}
                        aria-label="Grid view"
                        onPress={() =>
                          navigate({ search: { ...search, view: "grid" }, replace: true })
                        }
                      >
                        <GridIcon className="size-4" />
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
                            <Dropdown.Item id="background" textValue="Background upload">
                              <UploadIcon className="size-4" />
                              <Label>Background upload</Label>
                            </Dropdown.Item>
                          </Dropdown.Menu>
                        </Dropdown.Popover>
                      </Dropdown>
                      <span className="hidden" aria-hidden="true">
                        <FileTrigger
                          allowsMultiple
                          onSelect={(list) => {
                            if (list?.length)
                              enqueue(Array.from(list), search.parentId, search.path);
                          }}
                        >
                          <Button ref={uploadFilesTriggerRef}>Choose upload files</Button>
                        </FileTrigger>
                        <FileTrigger
                          acceptDirectory
                          allowsMultiple
                          onSelect={(list) => {
                            if (list?.length)
                              enqueue(Array.from(list), search.parentId, search.path);
                          }}
                        >
                          <Button ref={uploadFolderTriggerRef}>Choose upload folder</Button>
                        </FileTrigger>
                      </span>
                    </div>
                  </div>
                </Card.Header>

                <Card.Content className="min-h-0 flex-1 overflow-hidden p-0">
                  {fileQuery.isPending ? (
                    <div className="flex h-full min-h-64 items-center justify-center">
                      <Spinner size="lg" />
                    </div>
                  ) : (
                    <div className="h-full min-h-0">
                      <FileCollection
                        files={files}
                        view={search.view}
                        selectedKeys={selectedKeys}
                        onSelectionChange={setSelectedKeys}
                        onClearSelection={() => setSelectedKeys(new Set())}
                        onOpen={openFile}
                        hasNextPage={fileQuery.hasNextPage}
                        isLoadingMore={fileQuery.isFetchingNextPage}
                        onLoadMore={() => {
                          if (fileQuery.hasNextPage && !fileQuery.isFetchingNextPage) {
                            void fileQuery.fetchNextPage();
                          }
                        }}
                      />
                    </div>
                  )}
                </Card.Content>

                {selectedCount > 0 ? (
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
                              setSharePassword("");
                              setShareUrl("");
                            }}
                          >
                            <LinkIcon className="size-4" />
                          </Button>
                        </>
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
                ) : null}
              </Card>
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

      <AppDialog
        open={Boolean(shareFile)}
        onOpenChange={(open) => {
          if (!open) setShareFile(undefined);
        }}
        title="Share item"
        description="Create a public link. A password is optional."
        size="md"
        footer={
          <>
            <Button variant="secondary" onPress={() => setShareFile(undefined)}>
              Close
            </Button>
            <Button
              variant="primary"
              isDisabled={createShareMutation.isPending}
              onPress={() => void createShare()}
            >
              {shareUrl ? "Create another link" : "Create and copy link"}
            </Button>
          </>
        }
      >
        <div className="grid gap-4">
          <TextField value={sharePassword} onChange={setSharePassword}>
            <Label>Optional password</Label>
            <Input type="password" placeholder="Leave empty for no password" />
          </TextField>
          {shareUrl ? (
            <div className="rounded-lg border border-border bg-default/20 p-3">
              <p className="mb-2 text-xs font-medium text-muted">Public link</p>
              <div className="flex gap-2">
                <Input readOnly value={shareUrl} className="min-w-0 flex-1" />
                <Button
                  size="sm"
                  variant="secondary"
                  onPress={() => {
                    void navigator.clipboard.writeText(shareUrl);
                    toast.success("Share link copied");
                  }}
                >
                  Copy
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </AppDialog>

      <FilePreviewDialog
        file={previewFile}
        onOpenChange={(open) => {
          if (!open) setPreviewFile(undefined);
        }}
      />
    </Page>
  );
}

function FileCollection({
  files,
  view,
  selectedKeys,
  onSelectionChange,
  onClearSelection,
  onOpen,
  hasNextPage,
  isLoadingMore,
  onLoadMore,
}: {
  files: FileEntry[];
  view: "list" | "grid";
  selectedKeys: Selection;
  onSelectionChange: (selection: Selection) => void;
  onClearSelection: () => void;
  onOpen: (file: FileEntry) => void;
  hasNextPage: boolean;
  isLoadingMore: boolean;
  onLoadMore: () => void;
}) {
  const grid = view === "grid";
  const hasSelection = selectedKeys === "all" || selectedKeys.size > 0;
  return (
    <Virtualizer key={view} layout={grid ? GridLayout : ListLayout}>
      <GridList
        aria-label="Files and folders"
        layout={grid ? "grid" : "stack"}
        selectionMode="multiple"
        selectionBehavior="replace"
        selectedKeys={selectedKeys}
        onSelectionChange={onSelectionChange}
        onClick={(event) => {
          const target = event.target as HTMLElement;
          if (!target.closest('[role="row"]')) onClearSelection();
        }}
        onAction={(key) => {
          const file = files.find((item) => item.id === key);
          if (file) onOpen(file);
        }}
        renderEmptyState={() => (
          <div className="flex min-h-80 flex-col items-center justify-center px-6 text-center">
            <div className="mb-3 flex size-11 items-center justify-center rounded-xl bg-default/30 text-muted">
              <FolderIcon className="size-5" />
            </div>
            <p className="text-sm font-medium">This folder is empty</p>
            <p className="mt-1 text-xs text-muted">Upload files to start using this folder.</p>
          </div>
        )}
        className={
          grid
            ? `grid h-full min-h-0 grid-cols-[repeat(auto-fill,minmax(8.5rem,1fr))] gap-2 overflow-x-hidden overflow-y-auto p-2 outline-none sm:grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] sm:gap-3 sm:p-4 ${hasSelection ? "pb-24 sm:pb-24" : ""}`
            : `h-full min-h-0 overflow-x-hidden overflow-y-auto outline-none ${hasSelection ? "pb-24" : ""}`
        }
      >
        <Collection items={files}>
          {(file) => (
            <GridListItem
              id={file.id}
              textValue={file.name}
              className={(state) =>
                grid
                  ? [
                      "group flex min-h-36 cursor-default flex-col justify-between rounded-xl border border-border bg-background/35 p-3 outline-none transition",
                      "hover:-translate-y-0.5 hover:border-accent/30 hover:shadow-md",
                      state.isSelected && "border-accent bg-accent/10",
                      state.isFocusVisible && "ring-2 ring-accent/30",
                    ]
                      .filter(Boolean)
                      .join(" ")
                  : [
                      "group grid min-h-14 cursor-default grid-cols-[minmax(0,1fr)] items-center gap-2 border-b border-border px-3 outline-none transition last:border-b-0 sm:grid-cols-[minmax(0,1fr)_6rem] sm:px-4 lg:grid-cols-[minmax(0,1fr)_8rem_10rem] lg:gap-4",
                      "hover:bg-default/20",
                      state.isSelected && "bg-accent/10",
                      state.isFocusVisible && "ring-2 ring-inset ring-accent/30",
                    ]
                      .filter(Boolean)
                      .join(" ")
              }
            >
              {(state) =>
                grid ? (
                  <GridFile
                    file={file}
                    showCheckbox={state.isHovered || state.isFocusVisible || state.isSelected}
                  />
                ) : (
                  <ListFile
                    file={file}
                    showCheckbox={state.isHovered || state.isFocusVisible || state.isSelected}
                  />
                )
              }
            </GridListItem>
          )}
        </Collection>
        {hasNextPage ? (
          <GridListLoadMoreItem
            onLoadMore={onLoadMore}
            isLoading={isLoadingMore}
            className="flex min-h-14 items-center justify-center p-4"
          >
            <Spinner size="sm" />
          </GridListLoadMoreItem>
        ) : null}
      </GridList>
    </Virtualizer>
  );
}

function SelectionCheckbox({ file, isVisible }: { file: FileEntry; isVisible: boolean }) {
  return (
    <Checkbox
      slot="selection"
      aria-label={`Select ${file.name}`}
      className={({ isSelected }) =>
        [
          "shrink-0 transition-opacity",
          isSelected || isVisible ? "opacity-100" : "opacity-0 [@media(hover:none)]:opacity-100",
        ].join(" ")
      }
    >
      <Checkbox.Content>
        <Checkbox.Control>
          <Checkbox.Indicator />
        </Checkbox.Control>
      </Checkbox.Content>
    </Checkbox>
  );
}

function GridFile({ file, showCheckbox }: { file: FileEntry; showCheckbox: boolean }) {
  const Icon = file.kind === "folder" ? FolderIcon : FileIcon;
  return (
    <>
      <div className="flex items-start gap-2">
        <SelectionCheckbox file={file} isVisible={showCheckbox} />
        <div className="flex size-11 items-center justify-center rounded-xl bg-accent/10 text-accent">
          <Icon className="size-5" />
        </div>
      </div>
      <div className="min-w-0">
        <p className="truncate text-sm font-medium group-hover:text-accent">{file.name}</p>
        <p className="mt-1 text-[11px] text-muted">
          {file.kind === "folder" ? "Folder" : formatBytes(file.size ?? 0)}
        </p>
      </div>
    </>
  );
}

function ListFile({ file, showCheckbox }: { file: FileEntry; showCheckbox: boolean }) {
  const Icon = file.kind === "folder" ? FolderIcon : FileIcon;
  return (
    <>
      <div className="flex min-w-0 items-center gap-3">
        <SelectionCheckbox file={file} isVisible={showCheckbox} />
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent">
          <Icon className="size-4" />
        </div>
        <span className="truncate text-sm font-medium group-hover:text-accent">{file.name}</span>
      </div>
      <span className="hidden text-xs text-muted sm:block">
        {file.kind === "folder" ? "Folder" : formatBytes(file.size ?? 0)}
      </span>
      <span className="hidden text-xs text-muted lg:block">
        {new Date(file.modTime).toLocaleString()}
      </span>
    </>
  );
}

function Breadcrumb({ path, view }: { path: string; view: "list" | "grid" }) {
  const parts = path.split("/").filter(Boolean);
  return (
    <nav
      aria-label="Current folder"
      className="flex min-w-0 items-center gap-1 overflow-hidden text-sm"
    >
      <Link
        to="/files"
        search={{ path: "/", query: "", view }}
        className="rounded-lg px-2 py-1 text-muted transition hover:bg-default/30 hover:text-foreground"
      >
        My files
      </Link>
      {parts.map((part, index) => {
        const currentPath = `/${parts.slice(0, index + 1).join("/")}`;
        return (
          <span key={currentPath} className="flex min-w-0 items-center gap-1">
            <span className="text-muted">/</span>
            <Link
              to="/files"
              search={{ path: currentPath, query: "", view }}
              className="truncate rounded-lg px-2 py-1 text-muted transition hover:bg-default/30 hover:text-foreground"
            >
              {part}
            </Link>
          </span>
        );
      })}
    </nav>
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

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}
