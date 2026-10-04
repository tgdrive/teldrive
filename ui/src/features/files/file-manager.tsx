import { Button, Dropdown, Input, Label, TextField } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { useKeyboard } from "react-aria/useKeyboard";
import { DropZone, FileTrigger, type Selection } from "react-aria-components";
import { toast } from "sonner";
import { normalizeApiError, userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";
import { currentUserQueryOptions } from "@/auth/queries";
import { BackgroundUploadDialog } from "@/components/background-upload-dialog";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { isPreviewable } from "@/components/file-preview-dialog";
import { Page, PageContent } from "@/components/page";
import { useUploadStore } from "@/features/uploads/store";
import PasteIcon from "~icons/gravity-ui/arrow-right-to-square";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import SplitIcon from "~icons/gravity-ui/layout-split-columns";
import PlusIcon from "~icons/gravity-ui/plus";
import CloseIcon from "~icons/gravity-ui/xmark";

import { useFileClipboardStore } from "./clipboard-store";
import { absoluteFileDownloadUrl, copyText, startFileDownload } from "./download";
import { FileActionDialogs } from "./file-action-dialogs";
import { FileBrowser } from "./file-browser";
import { FileSelectionToolbar } from "./file-selection-toolbar";
import { useFileActions } from "./mutations";
import { useInfiniteFilePages } from "./queries";
import { SearchControls } from "./search-controls";
import {
  driveSearchOptions,
  hasSearchCriteria,
  invalidSearchDates,
  type SearchState,
} from "./search-state";

type FileBrowserView = "list" | "grid";
type PaneId = "primary" | "secondary";

type PaneLocation = {
  path: string;
  parentId?: string;
  query: string;
  view: FileBrowserView;
};

export type FilesLocation = PaneLocation & {
  split?: boolean;
  secondaryPath?: string;
  secondaryParentId?: string;
  secondaryQuery?: string;
  secondaryView?: FileBrowserView;
};

export function FileManagerPage({
  location: search,
  onLocationChange,
  searchMode,
}: {
  location: FilesLocation;
  onLocationChange: (location: FilesLocation, replace?: boolean) => void;
  searchMode?: {
    criteria: SearchState;
    onChange: (search: SearchState, replace?: boolean) => void;
  };
}) {
  const navigateTo = useNavigate();
  const navigate = ({ search: next, replace }: { search: FilesLocation; replace?: boolean }) =>
    onLocationChange(next, replace);
  const criteria = searchMode?.criteria;
  const missingFolder = criteria?.scope === "recursive" && !criteria.parentId;
  const invalidDates = criteria ? invalidSearchDates(criteria) : false;
  const activeSearch = criteria ? hasSearchCriteria(criteria) : true;
  const [filtersOpen, setFiltersOpen] = useState(false);
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
  const [primarySelectedKeys, setPrimarySelectedKeys] = useState<Selection>(new Set());
  const [secondarySelectedKeys, setSecondarySelectedKeys] = useState<Selection>(new Set());
  const [folderDialogOpen, setFolderDialogOpen] = useState(false);
  const [backgroundUploadOpen, setBackgroundUploadOpen] = useState(false);
  const [folderName, setFolderName] = useState("");
  const [previewFile, setPreviewFile] = useState<FileEntry>();
  const [renameFile, setRenameFile] = useState<FileEntry>();
  const [renameName, setRenameName] = useState("");
  const [destination, setDestination] = useState<{
    mode: "move" | "copy";
    files: FileEntry[];
    pane: PaneId;
  }>();
  const [actionError, setActionError] = useState<string>();
  const operationLock = useRef(false);
  const [operationPending, setOperationPending] = useState(false);
  const [shareFile, setShareFile] = useState<FileEntry>();
  const [pasteConflictPane, setPasteConflictPane] = useState<PaneId>();
  const primaryUploadFilesTriggerRef = useRef<HTMLButtonElement>(null);
  const primaryUploadFolderTriggerRef = useRef<HTMLButtonElement>(null);
  const secondaryUploadFilesTriggerRef = useRef<HTMLButtonElement>(null);
  const secondaryUploadFolderTriggerRef = useRef<HTMLButtonElement>(null);
  const enqueue = useUploadStore((state) => state.enqueue);
  const fileActions = useFileActions();
  const pending = operationPending || fileActions.pending;

  const clipboardMode = useFileClipboardStore((state) => state.mode);
  const clipboardItems = useFileClipboardStore((state) => state.items);
  const clipboardSourceParentId = useFileClipboardStore((state) => state.sourceParentId);
  const clipboardSourcePane = useFileClipboardStore((state) => state.sourcePane);
  const setClipboard = useFileClipboardStore((state) => state.set);
  const clearClipboard = useFileClipboardStore((state) => state.clear);

  const primaryFileQuery = useInfiniteFilePages(
    {
      path: primaryLocation.path,
      parentId: primaryLocation.parentId,
      q: primaryLocation.query || undefined,
      sort: "name",
      order: "asc",
      view: primaryLocation.view,
      ...(criteria ? driveSearchOptions(criteria) : {}),
    },
    "active",
    activeSearch && !missingFolder && !invalidDates,
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
    Boolean(search.split) && !searchMode,
  );
  const primaryFiles = primaryFileQuery.data?.pages.flatMap((page) => page.items) ?? [];
  const secondaryFiles = secondaryFileQuery.data?.pages.flatMap((page) => page.items) ?? [];

  const primaryCriteriaKey = JSON.stringify({
    path: primaryLocation.path,
    ...(criteria
      ? driveSearchOptions(criteria)
      : {
          parentId: primaryLocation.parentId,
          q: primaryLocation.query,
        }),
  });
  useEffect(() => setPrimarySelectedKeys(new Set()), [primaryCriteriaKey]);
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

  const cutIds =
    clipboardMode === "cut" ? new Set(clipboardItems.map((file) => file.id)) : undefined;
  const hasClipboard = Boolean(clipboardMode && clipboardItems.length > 0);

  const paneLocation = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondaryLocation : primaryLocation;
  const paneFiles = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondaryFiles : primaryFiles;
  const paneSelectedKeys = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondarySelectedKeys : primarySelectedKeys;
  const paneSelectedFiles = (pane: PaneId) =>
    pane === "secondary" && search.split ? secondarySelectedFiles : primarySelectedFiles;
  const setPaneSelectedKeys = (pane: PaneId, selection: Selection) => {
    if (searchMode && primaryFileQuery.isPlaceholderData) return;
    if (selection === "all") selection = new Set(paneFiles(pane).map((file) => file.id));
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

  const performAction = async (
    operation: () => Promise<unknown>,
    successMessage: string,
    failureMessage: string,
    onSuccess: () => void,
    inlineError = false,
  ) => {
    if (operationLock.current) return;
    operationLock.current = true;
    setOperationPending(true);
    setActionError(undefined);
    try {
      await operation();
      onSuccess();
      toast.success(successMessage);
    } catch (error) {
      if (inlineError) setActionError(`${failureMessage}. ${userMessage(error)}`);
      else toast.error(failureMessage, { description: userMessage(error) });
    } finally {
      operationLock.current = false;
      setOperationPending(false);
    }
  };

  const trashFiles = async (ids: string[], pane: PaneId) => {
    if (ids.length === 0 || pending) return;
    await performAction(
      () => fileActions.bulkTrash(ids),
      `${ids.length} item${ids.length === 1 ? "" : "s"} moved to trash`,
      "Items could not be moved to trash",
      () => setPaneSelectedKeys(pane, new Set()),
    );
  };

  const trashSelected = async (pane: PaneId) => {
    const files = paneSelectedFiles(pane);
    await trashFiles(
      files.map((file) => file.id),
      pane,
    );
  };

  const renameSelected = async () => {
    if (!renameFile || !renameName.trim() || pending) return;
    await performAction(
      () => fileActions.rename(renameFile, renameName.trim()),
      "Item renamed",
      "Item could not be renamed",
      () => {
        setRenameFile(undefined);
        setRenameName("");
        setPaneSelectedKeys(activePane, new Set());
      },
      true,
    );
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

  const transferSelected = async (parentId?: string) => {
    const target = destination;
    if (!target || target.files.length === 0 || pending) return;
    await performAction(
      async () => {
        if (target.mode === "copy") return fileActions.copyMany(target.files, parentId, "rename");
        if (target.files.length === 1) return fileActions.move(target.files[0], parentId, "rename");
        return fileActions.bulkMove(
          target.files.map((file) => file.id),
          parentId,
        );
      },
      `${target.files.length} item${target.files.length === 1 ? "" : "s"} ${target.mode === "copy" ? "copied" : "moved"}`,
      target.mode === "copy" ? "Items could not be copied" : "Selected items could not be moved",
      () => {
        setDestination(undefined);
        setPaneSelectedKeys(target.pane, new Set());
      },
      true,
    );
  };

  const stageClipboard = (mode: "copy" | "cut", pane: PaneId) => {
    const files = paneSelectedFiles(pane);
    if (files.length === 0) return;
    if (searchMode && mode === "copy") {
      setActionError(undefined);
      setDestination({ mode: "copy", files: [...files], pane });
      return;
    }
    const location = paneLocation(pane);
    setClipboard(mode, files, location.parentId, location.path, pane);
    setPaneSelectedKeys(pane, new Set());
  };

  const pasteClipboard = async (
    pane: PaneId,
    cutConflictPolicy: "fail" | "rename" | "replace" = "fail",
  ) => {
    if (!clipboardMode || clipboardItems.length === 0) return;
    const location = paneLocation(pane);
    if (clipboardMode === "cut" && clipboardSourceParentId === location.parentId) {
      toast.info("Items are already in this folder");
      return;
    }
    try {
      if (clipboardMode === "copy") {
        await fileActions.copyMany(clipboardItems, location.parentId, "rename");
      } else if (clipboardItems.length === 1) {
        await fileActions.move(clipboardItems[0], location.parentId, cutConflictPolicy);
      } else {
        await fileActions.bulkMove(
          clipboardItems.map((file) => file.id),
          location.parentId,
          cutConflictPolicy,
        );
      }
      const count = clipboardItems.length;
      const action = clipboardMode === "copy" ? "copied" : "moved";
      setPasteConflictPane(undefined);
      if (clipboardMode === "cut") clearClipboard();
      setPaneSelectedKeys(pane, new Set());
      toast.success(`${count} item${count === 1 ? "" : "s"} ${action}`);
    } catch (error) {
      const normalized = normalizeApiError(error);
      if (clipboardMode === "cut" && cutConflictPolicy === "fail" && normalized.status === 409) {
        setPasteConflictPane(pane);
        return;
      }
      toast.error("Clipboard items could not be pasted", { description: userMessage(error) });
    }
  };

  const navigateToParent = (pane: PaneId) => {
    const location = paneLocation(pane);
    if (location.path === "/") return;
    const parts = location.path.split("/").filter(Boolean);
    const parentPath = parts.length <= 1 ? "/" : `/${parts.slice(0, -1).join("/")}`;
    navigatePane(pane, { path: parentPath, query: "", view: location.view });
  };

  const { keyboardProps } = useKeyboard({
    onKeyDown: (event) => {
      if (
        event.defaultPrevented ||
        event.nativeEvent.isComposing ||
        pending ||
        primaryFileQuery.isPlaceholderData ||
        folderDialogOpen ||
        renameFile ||
        destination ||
        shareFile ||
        previewFile ||
        pasteConflictPane ||
        (event.target instanceof HTMLElement && event.target.closest('[role="dialog"]')) ||
        isEditableTarget(event.target)
      ) {
        event.continuePropagation();
        return;
      }
      const pane = activePaneRef.current;
      const selectedFiles = paneSelectedFiles(pane);
      const selectedIds = selectedFiles.map((file) => file.id);
      const singleSelectedFile = selectedFiles.length === 1 ? selectedFiles[0] : undefined;
      const command = event.ctrlKey || event.metaKey;

      const key = event.key.toLowerCase();
      if (command && key === "c" && selectedFiles.length > 0) {
        event.preventDefault();
        stageClipboard("copy", pane);
        return;
      }
      if (!searchMode && command && key === "x" && selectedFiles.length > 0) {
        event.preventDefault();
        stageClipboard("cut", pane);
        return;
      }
      if (!searchMode && command && key === "v" && hasClipboard) {
        event.preventDefault();
        void pasteClipboard(pane);
        return;
      }
      if (event.key === "F2" && singleSelectedFile) {
        event.preventDefault();
        setActivePane(pane);
        setRenameFile(singleSelectedFile);
        setActionError(undefined);
        setRenameName(singleSelectedFile.name);
        return;
      }
      if (event.key === "Delete" && selectedIds.length > 0) {
        event.preventDefault();
        void trashSelected(pane);
        return;
      }
      if (!searchMode && command && event.shiftKey && event.key.toLowerCase() === "n") {
        event.preventDefault();
        setActivePane(pane);
        setFolderDialogOpen(true);
        return;
      }
      if (!searchMode && event.altKey && event.key === "ArrowUp") {
        event.preventDefault();
        navigateToParent(pane);
        return;
      }
      event.continuePropagation();
    },
  });

  const openFile = (file: FileEntry, pane: PaneId) => {
    activePaneRef.current = pane;
    setActivePane(pane);
    if (file.kind === "folder") {
      const location = paneLocation(pane);
      if (searchMode) {
        void navigateTo({
          to: "/files",
          search: {
            path: joinPath(file.parentPath ?? "/", file.name),
            parentId: file.id,
            query: "",
            view: location.view,
          },
        });
        return;
      }
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
    if (searchMode) return undefined;
    const location = paneLocation(pane);
    const uploadFilesTriggerRef =
      pane === "secondary" ? secondaryUploadFilesTriggerRef : primaryUploadFilesTriggerRef;
    const uploadFolderTriggerRef =
      pane === "secondary" ? secondaryUploadFolderTriggerRef : primaryUploadFolderTriggerRef;
    return (
      <>
        {pane === "primary" && !search.split ? (
          <Button
            isIconOnly
            size="sm"
            variant="secondary"
            aria-label="Open split view"
            onPress={openSplitView}
          >
            <SplitIcon className="size-4" />
          </Button>
        ) : pane === "secondary" && search.split ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Close split view"
            onPress={closeSplitView}
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
    const clipboardTargetPane =
      search.split && clipboardSourcePane
        ? clipboardSourcePane === "primary"
          ? "secondary"
          : "primary"
        : "primary";
    const showClipboard =
      !searchMode && hasClipboard && (!search.split || pane === clipboardTargetPane);
    const canPasteHere =
      showClipboard &&
      !(clipboardMode === "cut" && clipboardSourceParentId === paneLocation(pane).parentId);
    if (showClipboard) {
      return (
        <div className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center px-4">
          <div className="pointer-events-auto flex max-w-full items-center gap-1.5 rounded-full border border-border bg-surface/95 p-1.5 shadow-xl backdrop-blur">
            <span className="shrink-0 rounded-full bg-default/40 px-3 py-2 text-sm font-medium text-foreground">
              {clipboardItems.length} {clipboardMode === "cut" ? "cut" : "copied"}
            </span>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={`Paste ${clipboardItems.length} clipboard item${clipboardItems.length === 1 ? "" : "s"}`}
              isDisabled={fileActions.pending || !canPasteHere}
              onPress={() => void pasteClipboard(pane)}
            >
              <PasteIcon className="size-4" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={clipboardMode === "cut" ? "Cancel cut" : "Clear copied items"}
              onPress={clearClipboard}
            >
              <CloseIcon className="size-4" />
            </Button>
          </div>
        </div>
      );
    }
    return (
      <FileSelectionToolbar
        selectedFiles={selectedFiles}
        pending={pending}
        onCut={searchMode ? undefined : () => stageClipboard("cut", pane)}
        onCopy={() => stageClipboard("copy", pane)}
        onRename={() => {
          const file = selectedFiles[0];
          setActivePane(pane);
          setRenameFile(file);
          setRenameName(file.name);
          setActionError(undefined);
        }}
        onDuplicate={searchMode ? undefined : () => void duplicateSelected(pane)}
        onShare={() => {
          setActivePane(pane);
          setShareFile(selectedFiles[0]);
        }}
        onDownload={() => startFileDownload(selectedFiles[0])}
        onCopyDownloadLinks={() => void copyDownloadLinks(pane)}
        onMove={() => {
          setActivePane(pane);
          setActionError(undefined);
          setDestination({ mode: "move", files: [...selectedFiles], pane });
        }}
        onTrash={() => void trashSelected(pane)}
        onClear={() => setPaneSelectedKeys(pane, new Set())}
      />
    );
  };

  const renderPane = (pane: PaneId) => {
    const location = paneLocation(pane);
    const files = paneFiles(pane);
    const selectedKeys = paneSelectedKeys(pane);
    const fileQuery = pane === "secondary" ? secondaryFileQuery : primaryFileQuery;
    const browser = (
      <FileBrowser
        files={files}
        path={location.path}
        rootLabel={searchMode ? "Search results" : "My files"}
        view={location.view}
        loading={fileQuery.isPending}
        onNavigatePath={(path) => navigatePane(pane, { path, query: "", view: location.view })}
        onViewChange={(view) => navigatePane(pane, { ...location, view }, true)}
        onOpen={(file) => openFile(file, pane)}
        onBack={searchMode ? undefined : () => window.history.back()}
        selection={{
          selectedKeys,
          onSelectionChange: (selection) => setPaneSelectedKeys(pane, selection),
          onClearSelection: () => setPaneSelectedKeys(pane, new Set()),
        }}
        selectionDisabled={fileQuery.isPlaceholderData || pending}
        hasNextPage={
          fileQuery.hasNextPage && !fileQuery.isPlaceholderData && !fileQuery.isFetchNextPageError
        }
        isLoadingMore={fileQuery.isFetchingNextPage}
        onLoadMore={() => {
          if (fileQuery.hasNextPage && !fileQuery.isFetching && !fileQuery.isPlaceholderData)
            void fileQuery.fetchNextPage();
        }}
        toolbar={renderToolbar(pane)}
        selectionOverlay={fileQuery.isPlaceholderData ? undefined : renderSelectionOverlay(pane)}
        dimmedIds={cutIds}
        hideFolderControls={Boolean(searchMode)}
        showLocations={Boolean(searchMode)}
        emptyTitle={searchMode ? "No matching files" : undefined}
        emptyHint={searchMode ? "Try another name or adjust your filters." : undefined}
        onOpenContainingFolder={
          searchMode
            ? (file) => {
                void navigateTo({
                  to: "/files",
                  search: {
                    path: file.parentPath ?? "/",
                    parentId: file.parentId,
                    query: "",
                    view: location.view,
                  },
                });
              }
            : undefined
        }
      />
    );
    if (searchMode)
      return (
        <div data-testid={`file-pane-${pane}`} className="flex min-h-0 min-w-0 flex-1">
          {browser}
        </div>
      );
    return (
      <div data-testid={`file-pane-${pane}`} className="flex min-h-0 min-w-0 flex-1 rounded-xl">
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
              {browser}
            </div>
          )}
        </DropZone>
      </div>
    );
  };

  return (
    <Page className={`h-full min-h-0 overflow-x-hidden ${searchMode ? "gap-4" : "gap-0"}`}>
      {searchMode && (
        <SearchControls
          search={searchMode.criteria}
          onChange={searchMode.onChange}
          isOpen={filtersOpen}
          onOpenChange={setFiltersOpen}
        />
      )}
      {searchMode && activeSearch && !missingFolder && !invalidDates && (
        <p aria-live="polite" className="text-xs text-muted">
          {primaryFileQuery.isFetching ? "Updating results…" : `${primaryFiles.length} loaded`}
        </p>
      )}
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <div {...keyboardProps} className="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden">
          {searchMode && primaryFileQuery.isError && (
            <div role="alert" className="mb-3 rounded-xl border border-danger/30 p-4 text-sm">
              <p className="font-medium">
                {primaryFileQuery.isFetchNextPageError
                  ? "More results could not be loaded."
                  : "Search could not be completed."}
              </p>
              <p className="mt-1 text-muted">{userMessage(primaryFileQuery.error)}</p>
              <Button
                size="sm"
                variant="secondary"
                className="mt-2"
                isDisabled={primaryFileQuery.isFetching}
                onPress={() =>
                  void (primaryFileQuery.isFetchNextPageError
                    ? primaryFileQuery.fetchNextPage()
                    : primaryFileQuery.refetch())
                }
              >
                Retry
              </Button>
            </div>
          )}
          {searchMode && (missingFolder || invalidDates || !activeSearch) ? (
            <div className="flex flex-1 flex-col items-center justify-center rounded-2xl border border-dashed border-border p-8 text-center">
              <p className="text-lg font-semibold">
                {missingFolder
                  ? "Choose a folder to search recursively"
                  : invalidDates
                    ? "Check the modified-date range"
                    : "Find anything in your drive"}
              </p>
              <p className="mt-2 text-sm text-muted">
                {missingFolder
                  ? "This saved search has no folder selected."
                  : invalidDates
                    ? "Start date must be before end date."
                    : "Search by filename, or apply filters to explore."}
              </p>
              {(missingFolder || invalidDates) && (
                <Button variant="secondary" className="mt-3" onPress={() => setFiltersOpen(true)}>
                  {missingFolder ? "Choose folder" : "Edit filters"}
                </Button>
              )}
            </div>
          ) : searchMode && primaryFileQuery.isError && !primaryFileQuery.data ? null : (
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
          )}
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
            if (event.key === "Enter") {
              event.preventDefault();
              void createFolder();
            } else event.continuePropagation();
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

      <FileActionDialogs
        renameFile={renameFile}
        renameName={renameName}
        onRenameNameChange={setRenameName}
        onRenameClose={() => {
          if (!pending) {
            setRenameFile(undefined);
            setActionError(undefined);
          }
        }}
        onRename={() => void renameSelected()}
        pending={pending}
        error={actionError}
        destinationAction={
          destination
            ? {
                mode: destination.mode,
                count: destination.files.length,
                onClose: () => {
                  if (!pending) {
                    setDestination(undefined);
                    setActionError(undefined);
                  }
                },
                onConfirm: (parentId) => void transferSelected(parentId),
              }
            : undefined
        }
        shareFile={shareFile}
        onShareClose={() => setShareFile(undefined)}
        previewFile={previewFile}
        onPreviewClose={() => setPreviewFile(undefined)}
      />

      <AppDialog
        open={pasteConflictPane !== undefined}
        onOpenChange={(open) => {
          if (!open) setPasteConflictPane(undefined);
        }}
        title="Item already exists"
        description="The destination already contains an item with the same name."
        size="md"
        footer={
          <>
            <Button variant="secondary" onPress={() => setPasteConflictPane(undefined)}>
              Cancel
            </Button>
            <Button
              variant="secondary"
              isDisabled={fileActions.pending || pasteConflictPane === undefined}
              onPress={() => {
                if (pasteConflictPane) void pasteClipboard(pasteConflictPane, "rename");
              }}
            >
              Keep both
            </Button>
            <Button
              variant="danger"
              isDisabled={fileActions.pending || pasteConflictPane === undefined}
              onPress={() => {
                if (pasteConflictPane) void pasteClipboard(pasteConflictPane, "replace");
              }}
            >
              Replace
            </Button>
          </>
        }
      >
        <p className="text-sm text-muted">
          Replace the existing item, or keep both by giving the moved item a new name.
        </p>
      </AppDialog>
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
