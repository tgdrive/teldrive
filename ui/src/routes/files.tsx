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
import SplitIcon from "~icons/gravity-ui/layout-split-columns";
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

type FileBrowserView = "list" | "grid";
type PaneId = "primary" | "secondary";

type PaneLocation = {
  path: string;
  parentId?: string;
  query: string;
  view: FileBrowserView;
};

type FilesSearch = PaneLocation & {
  split?: boolean;
  secondaryPath?: string;
  secondaryParentId?: string;
  secondaryQuery?: string;
  secondaryView?: FileBrowserView;
};

export const Route = createFileRoute("/files")({
  validateSearch: (search: Record<string, unknown>): FilesSearch => ({
    path: typeof search.path === "string" && search.path ? search.path : "/",
    parentId: typeof search.parentId === "string" ? search.parentId : undefined,
    query: typeof search.query === "string" ? search.query : "",
    view: search.view === "grid" ? "grid" : "list",
    split: search.split === true || search.split === "true",
    secondaryPath:
      typeof search.secondaryPath === "string" && search.secondaryPath
        ? search.secondaryPath
        : undefined,
    secondaryParentId:
      typeof search.secondaryParentId === "string" ? search.secondaryParentId : undefined,
    secondaryQuery: typeof search.secondaryQuery === "string" ? search.secondaryQuery : undefined,
    secondaryView:
      search.secondaryView === "grid" || search.secondaryView === "list"
        ? search.secondaryView
        : undefined,
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

  const primaryLocation: PaneLocation = {
    path: search.path,
    parentId: search.parentId,
    query: search.query,
    view: search.view,
  };
  const secondaryLocation: PaneLocation = search.secondaryPath
    ? {
        path: search.secondaryPath,
        parentId: search.secondaryParentId,
        query: search.secondaryQuery ?? "",
        view: search.secondaryView ?? search.view,
      }
    : primaryLocation;

  const [activePane, setActivePane] = useState<PaneId>("primary");
  const activePaneRef = useRef<PaneId>("primary");
  const [primaryQueryDraft, setPrimaryQueryDraft] = useState(primaryLocation.query);
  const [secondaryQueryDraft, setSecondaryQueryDraft] = useState(secondaryLocation.query);
  const deferredPrimaryQuery = useDeferredValue(primaryQueryDraft.trim());
  const deferredSecondaryQuery = useDeferredValue(secondaryQueryDraft.trim());
  const [primarySelectedKeys, setPrimarySelectedKeys] = useState<Selection>(new Set());
  const [secondarySelectedKeys, setSecondarySelectedKeys] = useState<Selection>(new Set());
  const [folderDialogOpen, setFolderDialogOpen] = useState(false);
  const [backgroundUploadOpen, setBackgroundUploadOpen] = useState(false);
  const [folderName, setFolderName] = useState("");
  const [previewFile, setPreviewFile] = useState<FileEntry>();
  const [renameFile, setRenameFile] = useState<FileEntry>();
  const [renameName, setRenameName] = useState("");
  const [moveDialogOpen, setMoveDialogOpen] = useState(false);
  const [shareFile, setShareFile] = useState<FileEntry>();
  const primarySearchInputRef = useRef<HTMLInputElement>(null);
  const secondarySearchInputRef = useRef<HTMLInputElement>(null);
  const primaryUploadFilesTriggerRef = useRef<HTMLButtonElement>(null);
  const primaryUploadFolderTriggerRef = useRef<HTMLButtonElement>(null);
  const secondaryUploadFilesTriggerRef = useRef<HTMLButtonElement>(null);
  const secondaryUploadFolderTriggerRef = useRef<HTMLButtonElement>(null);
  const enqueue = useUploadStore((state) => state.enqueue);
  const fileActions = useFileActions();

  const primaryFileQuery = useInfiniteFilePages(
    {
      path: primaryLocation.path,
      parentId: primaryLocation.parentId,
      q: primaryLocation.query || undefined,
      sort: "name",
      order: "asc",
      view: primaryLocation.view,
    },
    "active",
  );
  const secondaryFileQuery = useInfiniteFilePages(
    {
      path: secondaryLocation.path,
      parentId: secondaryLocation.parentId,
      q: secondaryLocation.query || undefined,
      sort: "name",
      order: "asc",
      view: secondaryLocation.view,
    },
    "active",
    search.split,
  );
  const primaryFiles = primaryFileQuery.data?.pages.flatMap((page) => page.items) ?? [];
  const secondaryFiles = secondaryFileQuery.data?.pages.flatMap((page) => page.items) ?? [];

  useEffect(() => setPrimaryQueryDraft(primaryLocation.query), [primaryLocation.query]);
  useEffect(() => setSecondaryQueryDraft(secondaryLocation.query), [secondaryLocation.query]);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (deferredPrimaryQuery !== primaryLocation.query) {
        navigate({ search: { ...search, query: deferredPrimaryQuery }, replace: true });
      }
    }, 250);
    return () => window.clearTimeout(timer);
  }, [deferredPrimaryQuery, navigate, primaryLocation.query, search]);
  useEffect(() => {
    if (!search.split) return;
    const timer = window.setTimeout(() => {
      if (deferredSecondaryQuery !== secondaryLocation.query) {
        navigate({
          search: { ...search, secondaryQuery: deferredSecondaryQuery },
          replace: true,
        });
      }
    }, 250);
    return () => window.clearTimeout(timer);
  }, [deferredSecondaryQuery, navigate, search, secondaryLocation.query]);
  useEffect(
    () => setPrimarySelectedKeys(new Set()),
    [primaryLocation.parentId, primaryLocation.path, primaryLocation.query],
  );
  useEffect(
    () => setSecondarySelectedKeys(new Set()),
    [secondaryLocation.parentId, secondaryLocation.path, secondaryLocation.query],
  );
  useEffect(() => {
    if (!search.split) {
      activePaneRef.current = "primary";
      if (activePane === "secondary") setActivePane("primary");
    }
  }, [activePane, search.split]);

  const primarySelectedIds = selectionIds(primarySelectedKeys, primaryFiles);
  const secondarySelectedIds = selectionIds(secondarySelectedKeys, secondaryFiles);
  const primarySelectedFiles = primaryFiles.filter((file) => primarySelectedIds.includes(file.id));
  const secondarySelectedFiles = secondaryFiles.filter((file) =>
    secondarySelectedIds.includes(file.id),
  );
  const activeLocation =
    activePane === "secondary" && search.split ? secondaryLocation : primaryLocation;
  const activeSelectedFiles =
    activePane === "secondary" && search.split ? secondarySelectedFiles : primarySelectedFiles;
  const activeSelectedIds =
    activePane === "secondary" && search.split ? secondarySelectedIds : primarySelectedIds;
  const activeSelectedCount = activeSelectedFiles.length;
  const activeSingleSelectedFile =
    activeSelectedFiles.length === 1 ? activeSelectedFiles[0] : undefined;

  const paneLocation = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondaryLocation : primaryLocation;
  const paneFiles = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondaryFiles : primaryFiles;
  const paneSelectedKeys = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondarySelectedKeys : primarySelectedKeys;
  const paneSelectedFiles = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondarySelectedFiles : primarySelectedFiles;
  const setPaneSelectedKeys = (pane: PaneId, selection: Selection) => {
    activePaneRef.current = pane;
    if (pane === "secondary") setSecondarySelectedKeys(selection);
    else setPrimarySelectedKeys(selection);
  };

  const navigatePane = (pane: PaneId, location: PaneLocation, replace = false) => {
    if (pane === "secondary") {
      navigate({
        search: {
          ...search,
          split: true,
          secondaryPath: location.path,
          secondaryParentId: location.parentId,
          secondaryQuery: location.query,
          secondaryView: location.view,
        },
        replace,
      });
      return;
    }
    navigate({
      search: {
        ...search,
        path: location.path,
        parentId: location.parentId,
        query: location.query,
        view: location.view,
      },
      replace,
    });
  };

  const openSplitView = () => {
    if (search.split) return;
    setSecondaryQueryDraft(primaryLocation.query);
    setSecondarySelectedKeys(new Set());
    navigate({
      search: {
        ...search,
        split: true,
        secondaryPath: primaryLocation.path,
        secondaryParentId: primaryLocation.parentId,
        secondaryQuery: primaryLocation.query,
        secondaryView: primaryLocation.view,
      },
    });
  };

  const closeSplitView = () => {
    setActivePane("primary");
    setSecondarySelectedKeys(new Set());
    navigate({
      search: {
        path: primaryLocation.path,
        parentId: primaryLocation.parentId,
        query: primaryLocation.query,
        view: primaryLocation.view,
        split: false,
      },
    });
  };

  const createFolder = async () => {
    const name = folderName.trim();
    if (!name) return;
    try {
      await fileActions.createFolder(name, activeLocation.parentId);
      setFolderName("");
      setFolderDialogOpen(false);
      toast.success("Folder created");
    } catch (error) {
      toast.error("Folder could not be created", { description: userMessage(error) });
    }
  };

  const trashFiles = async (ids: string[], pane: PaneId) => {
    if (ids.length === 0) return;
    try {
      await fileActions.bulkTrash(ids);
      setPaneSelectedKeys(pane, new Set());
      toast.success(`${ids.length} item${ids.length === 1 ? "" : "s"} moved to trash`);
    } catch (error) {
      toast.error("Items could not be moved to trash", { description: userMessage(error) });
    }
  };

  const trashSelected = async (pane: PaneId) => {
    const files = paneSelectedFiles(pane);
    await trashFiles(
      files.map((file) => file.id),
      pane,
    );
  };

  const renameSelected = async () => {
    if (!renameFile || !renameName.trim()) return;
    try {
      await fileActions.rename(renameFile, renameName.trim());
      setRenameFile(undefined);
      setRenameName("");
      setPaneSelectedKeys(activePane, new Set());
      toast.success("Item renamed");
    } catch (error) {
      toast.error("Item could not be renamed", { description: userMessage(error) });
    }
  };

  const duplicateFile = async (file: FileEntry, pane: PaneId) => {
    try {
      await fileActions.copy(file, paneLocation(pane).parentId, `${file.name} copy`, "rename");
      setPaneSelectedKeys(pane, new Set());
      toast.success("Item duplicated");
    } catch (error) {
      toast.error("Item could not be duplicated", { description: userMessage(error) });
    }
  };

  const duplicateSelected = async (pane: PaneId) => {
    const selected = paneSelectedFiles(pane);
    if (selected.length === 1) await duplicateFile(selected[0], pane);
  };

  const moveSelected = async (parentId?: string) => {
    if (activeSelectedFiles.length === 0) return;
    try {
      if (activeSelectedFiles.length === 1) {
        await fileActions.move(activeSelectedFiles[0], parentId, "rename");
      } else {
        await fileActions.bulkMove(activeSelectedIds, parentId);
      }
      setMoveDialogOpen(false);
      setPaneSelectedKeys(activePane, new Set());
      toast.success(
        `${activeSelectedFiles.length} item${activeSelectedFiles.length === 1 ? "" : "s"} moved`,
      );
    } catch (error) {
      toast.error("Selected items could not be moved", { description: userMessage(error) });
    }
  };

  const navigateToParent = (pane: PaneId) => {
    const location = paneLocation(pane);
    if (location.path === "/") return;
    const parts = location.path.split("/").filter(Boolean);
    const parentPath = parts.length <= 1 ? "/" : `/${parts.slice(0, -1).join("/")}`;
    navigatePane(pane, { path: parentPath, query: "", view: location.view });
  };

  const handleFileShortcut = useEffectEvent((event: KeyboardEvent) => {
    if (event.defaultPrevented) return;
    const pane = activePaneRef.current;
    const selectedFiles = paneSelectedFiles(pane);
    const selectedIds = selectedFiles.map((file) => file.id);
    const singleSelectedFile = selectedFiles.length === 1 ? selectedFiles[0] : undefined;
    if (event.key === "Escape") {
      setFolderDialogOpen(false);
      setRenameFile(undefined);
      setMoveDialogOpen(false);
      setShareFile(undefined);
      setPreviewFile(undefined);
      setPaneSelectedKeys(pane, new Set());
      return;
    }
    if (isEditableTarget(event.target)) return;
    const command = event.ctrlKey || event.metaKey;
    if (event.key === "F2" && singleSelectedFile) {
      event.preventDefault();
      setActivePane(pane);
      setRenameFile(singleSelectedFile);
      setRenameName(singleSelectedFile.name);
      return;
    }
    if (event.key === "Delete" && selectedIds.length > 0) {
      event.preventDefault();
      void trashSelected(pane);
      return;
    }
    if (command && event.shiftKey && event.key.toLowerCase() === "n") {
      event.preventDefault();
      setActivePane(pane);
      setFolderDialogOpen(true);
      return;
    }
    if (command && event.key.toLowerCase() === "f") {
      event.preventDefault();
      const input =
        pane === "secondary" && search.split
          ? secondarySearchInputRef.current
          : primarySearchInputRef.current;
      input?.focus();
      input?.select();
      return;
    }
    if (event.altKey && event.key === "ArrowUp") {
      event.preventDefault();
      navigateToParent(pane);
    }
  });

  useEffect(() => {
    document.addEventListener("keydown", handleFileShortcut, true);
    return () => document.removeEventListener("keydown", handleFileShortcut, true);
  }, []);

  const openFile = (file: FileEntry, pane: PaneId) => {
    activePaneRef.current = pane;
    setActivePane(pane);
    if (file.kind === "folder") {
      const location = paneLocation(pane);
      navigatePane(pane, {
        path: joinPath(location.path, file.name),
        parentId: file.id,
        query: "",
        view: location.view,
      });
      setPaneSelectedKeys(pane, new Set());
      return;
    }
    if (isPreviewable(file)) {
      setPreviewFile(file);
      return;
    }
    startFileDownload(file);
  };

  const copyDownloadLinks = async (pane: PaneId) => {
    const selectedFiles = paneSelectedFiles(pane);
    try {
      await copyText(selectedFiles.map(absoluteFileDownloadUrl).join("\n"));
      toast.success(
        `${selectedFiles.length} download link${selectedFiles.length === 1 ? "" : "s"} copied`,
      );
    } catch (error) {
      toast.error("Download links could not be copied", { description: userMessage(error) });
    }
  };

  const renderToolbar = (pane: PaneId) => {
    const location = paneLocation(pane);
    const uploadFilesTriggerRef =
      pane === "secondary" ? secondaryUploadFilesTriggerRef : primaryUploadFilesTriggerRef;
    const uploadFolderTriggerRef =
      pane === "secondary" ? secondaryUploadFolderTriggerRef : primaryUploadFolderTriggerRef;
    return (
      <>
        {pane === "primary" ? (
          <Button
            isIconOnly
            size="sm"
            variant="secondary"
            aria-label={search.split ? "Close split view" : "Open split view"}
            onPress={search.split ? closeSplitView : openSplitView}
          >
            <SplitIcon className="size-4" />
          </Button>
        ) : null}
        <Button
          isIconOnly
          size="sm"
          variant="secondary"
          aria-label="New folder"
          onPress={() => {
            setActivePane(pane);
            setFolderDialogOpen(true);
          }}
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
                setActivePane(pane);
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
              if (list?.length) enqueue(Array.from(list), location.parentId, location.path);
            }}
          >
            <Button ref={uploadFilesTriggerRef}>Choose upload files</Button>
          </FileTrigger>
          <FileTrigger
            acceptDirectory
            allowsMultiple
            onSelect={(list) => {
              if (list?.length) enqueue(Array.from(list), location.parentId, location.path);
            }}
          >
            <Button ref={uploadFolderTriggerRef}>Choose upload folder</Button>
          </FileTrigger>
        </span>
      </>
    );
  };

  const renderSelectionOverlay = (pane: PaneId) => {
    const selectedFiles = paneSelectedFiles(pane);
    const selectedCount = selectedFiles.length;
    const selectedOnlyFiles =
      selectedFiles.length > 0 && selectedFiles.every((file) => file.kind === "file");
    const singleSelectedFile = selectedFiles.length === 1 ? selectedFiles[0] : undefined;
    if (selectedCount === 0) return null;
    return (
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
                  setActivePane(pane);
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
                onPress={() => void duplicateSelected(pane)}
              >
                <CopyIcon className="size-4" />
              </Button>
              <Button
                isIconOnly
                size="sm"
                variant="ghost"
                aria-label="Share selected item"
                onPress={() => {
                  setActivePane(pane);
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
              onPress={() => void copyDownloadLinks(pane)}
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
            onPress={() => {
              setActivePane(pane);
              setMoveDialogOpen(true);
            }}
          >
            <MoveIcon className="size-4" />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant="danger"
            aria-label="Move selected items to trash"
            isDisabled={fileActions.pending}
            onPress={() => void trashSelected(pane)}
          >
            <TrashIcon className="size-4" />
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Clear selection"
            onPress={() => setPaneSelectedKeys(pane, new Set())}
          >
            <CloseIcon className="size-4" />
          </Button>
        </div>
      </div>
    );
  };

  const renderPane = (pane: PaneId) => {
    const location = paneLocation(pane);
    const files = paneFiles(pane);
    const selectedKeys = paneSelectedKeys(pane);
    const fileQuery = pane === "secondary" ? secondaryFileQuery : primaryFileQuery;
    const queryDraft = pane === "secondary" ? secondaryQueryDraft : primaryQueryDraft;
    const setQueryDraft = pane === "secondary" ? setSecondaryQueryDraft : setPrimaryQueryDraft;
    const searchInputRef = pane === "secondary" ? secondarySearchInputRef : primarySearchInputRef;
    return (
      <div
        data-testid={`file-pane-${pane}`}
        className={[
          "flex min-h-0 min-w-0 flex-1 rounded-xl",
          search.split ? "ring-1 ring-border/50 focus-within:ring-accent/40" : "",
        ]
          .filter(Boolean)
          .join(" ")}
      >
        <DropZone
          data-testid={pane === "primary" ? "file-drop-zone" : "file-drop-zone-secondary"}
          aria-label={`Upload files into ${location.path}`}
          getDropOperation={() => "copy"}
          className="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden outline-none"
          onDrop={async (event) => {
            const dropped = await Promise.all(
              event.items.filter((item) => item.kind === "file").map((item) => item.getFile()),
            );
            if (dropped.length) enqueue(dropped, location.parentId, location.path);
          }}
        >
          {({ isDropTarget }) => (
            <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-x-hidden">
              {isDropTarget ? (
                <div className="shrink-0 rounded-xl border-2 border-dashed border-accent bg-accent/10 px-4 py-4 text-center text-sm font-medium text-accent sm:px-6 sm:py-6">
                  Drop files to upload into {location.path}
                </div>
              ) : null}
              <FileBrowser
                files={files}
                path={location.path}
                rootLabel="My files"
                view={location.view}
                query={queryDraft}
                loading={fileQuery.isPending}
                onQueryChange={setQueryDraft}
                onNavigatePath={(path) =>
                  navigatePane(pane, { path, query: "", view: location.view })
                }
                onViewChange={(view) => navigatePane(pane, { ...location, view }, true)}
                onOpen={(file) => openFile(file, pane)}
                searchInputRef={searchInputRef}
                selection={{
                  selectedKeys,
                  onSelectionChange: (selection) => setPaneSelectedKeys(pane, selection),
                  onClearSelection: () => setPaneSelectedKeys(pane, new Set()),
                }}
                hasNextPage={fileQuery.hasNextPage}
                isLoadingMore={fileQuery.isFetchingNextPage}
                onLoadMore={() => {
                  if (fileQuery.hasNextPage && !fileQuery.isFetchingNextPage) {
                    void fileQuery.fetchNextPage();
                  }
                }}
                toolbar={renderToolbar(pane)}
                selectionOverlay={renderSelectionOverlay(pane)}
              />
            </div>
          )}
        </DropZone>
      </div>
    );
  };

  return (
    <Page className="h-full min-h-0 gap-0 overflow-x-hidden">
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden">
          <div
            className={
              search.split
                ? "grid min-h-0 min-w-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-2"
                : "flex min-h-0 min-w-0 flex-1"
            }
          >
            {renderPane("primary")}
            {search.split ? renderPane("secondary") : null}
          </div>
        </div>
      </PageContent>

      <AppDialog
        open={folderDialogOpen}
        onOpenChange={setFolderDialogOpen}
        title="Create folder"
        description={`Create a folder inside ${activeLocation.path}.`}
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
        currentPath={activeLocation.path}
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
        title={`Move ${activeSelectedCount} item${activeSelectedCount === 1 ? "" : "s"}`}
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

function selectionIds(selection: Selection, files: FileEntry[]) {
  return selection === "all" ? files.map((file) => file.id) : Array.from(selection, String);
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
