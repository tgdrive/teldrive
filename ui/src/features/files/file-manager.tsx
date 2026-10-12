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
import ArrowDownIcon from "~icons/gravity-ui/arrow-down";
import PasteIcon from "~icons/gravity-ui/arrow-right-to-square";
import ArrowUpIcon from "~icons/gravity-ui/arrow-up";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import SplitIcon from "~icons/gravity-ui/layout-split-columns";
import PlusIcon from "~icons/gravity-ui/plus";
import CloseIcon from "~icons/gravity-ui/xmark";

import { useFileClipboardStore } from "./clipboard-store";
import { absoluteFileDownloadUrl, copyText, startFileDownload } from "./download";
import { FileActionDialogs } from "./file-action-dialogs";
import { useAppLocale } from "@/lib/locale";
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

import {
  type FileSort,
  loadSortPreference,
  type SortOrder,
  saveSortPreference,
} from "./sort-preference";

type FileBrowserView = "list" | "grid";
type PaneId = "primary" | "secondary";

type PaneLocation = {
  path: string;
  parentId?: string;
  query: string;
  view: FileBrowserView;
};

export type FilesLocation = PaneLocation & {
  sort?: FileSort;
  order?: SortOrder;
  split?: boolean;
  secondaryPath?: string;
  secondaryParentId?: string;
  secondaryQuery?: string;
  secondaryView?: FileBrowserView;
  bookmarked?: boolean;
  recent?: boolean;
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
  const [savedSort, setSavedSort] = useState(loadSortPreference);
  const sorting = search.recent
    ? { sort: "updatedAt" as const, order: "desc" as const }
    : { sort: search.sort ?? savedSort.sort, order: search.order ?? savedSort.order };
  const [recentAfter] = useState(() =>
    new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
  );
  const recentCriteria = search.recent
    ? {
        scope: "drive" as const,
        kind: "file" as const,
        updatedAfter: recentAfter,
        parentId: undefined,
        bookmarked: undefined,
      }
    : {};
  const changeSorting = (next: typeof sorting) => {
    setSavedSort(next);
    saveSortPreference(next);
    onLocationChange({ ...search, ...next }, true);
  };
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
  const { t } = useAppLocale();

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
      ...sorting,
      view: primaryLocation.view,
      ...(criteria ? driveSearchOptions(criteria) : {}),
      bookmarked: search.bookmarked,
      ...recentCriteria,
    },
    "active",
    activeSearch && !missingFolder && !invalidDates,
  );
  const secondaryFileQuery = useInfiniteFilePages(
    {
      path: secondaryLocation.path,
      parentId: secondaryLocation.parentId,
      q: secondaryLocation.query || undefined,
      ...sorting,
      view: secondaryLocation.view,
      bookmarked: search.bookmarked,
      ...recentCriteria,
    },
    "active",
    Boolean(search.split) && !searchMode,
  );
  const primaryFiles = primaryFileQuery.data?.pages.flatMap((page) => page.items) ?? [];
  const secondaryFiles = secondaryFileQuery.data?.pages.flatMap((page) => page.items) ?? [];

  const primaryCriteriaKey = JSON.stringify({
    ...sorting,
    path: primaryLocation.path,
    bookmarked: search.bookmarked,
    recent: search.recent,
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
      toast.success(t("files.folderCreated"));
    } catch (error) {
      toast.error(t("files.folderCreateFailed"), { description: userMessage(error) });
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
      t(ids.length === 1 ? "files.trashedOne" : "files.trashedMany", { count: ids.length }),
      t("files.trashFailed"),
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

  const toggleBookmarks = async (pane: PaneId) => {
    const files = paneSelectedFiles(pane);
    if (files.length === 0 || pending) return;
    const ids = files.map((file) => file.id);
    await performAction(
      () => (search.bookmarked ? fileActions.removeBookmarks(ids) : fileActions.bookmarkFiles(ids)),
      search.bookmarked
        ? t(ids.length === 1 ? "manager.unbookmarkedOne" : "manager.unbookmarkedMany", {
            count: ids.length,
          })
        : t(ids.length === 1 ? "manager.bookmarkedOne" : "manager.bookmarkedMany", {
            count: ids.length,
          }),
      search.bookmarked ? t("manager.unbookmarkFailed") : t("manager.bookmarkFailed"),
      () => setPaneSelectedKeys(pane, new Set()),
    );
  };

  const renameSelected = async () => {
    if (!renameFile || !renameName.trim() || pending) return;
    await performAction(
      () => fileActions.rename(renameFile, renameName.trim()),
      t("files.renamed"),
      t("files.renameFailed"),
      () => {
        setRenameFile(undefined);
        setRenameName("");
        setPaneSelectedKeys(activePane, new Set());
      },
      true,
    );
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
      t(
        target.mode === "copy"
          ? target.files.length === 1
            ? "manager.copiedOne"
            : "manager.copiedMany"
          : target.files.length === 1
            ? "manager.movedOne"
            : "manager.movedMany",
        { count: target.files.length },
      ),
      target.mode === "copy" ? t("manager.copyFailed") : t("manager.moveFailed"),
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
      toast.info(t("manager.alreadyHere"));
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
      const action = t(clipboardMode === "copy" ? "manager.actionCopied" : "manager.actionMoved");
      setPasteConflictPane(undefined);
      if (clipboardMode === "cut") clearClipboard();
      setPaneSelectedKeys(pane, new Set());
      toast.success(t(count === 1 ? "manager.pastedOne" : "manager.pastedMany", { count, action }));
    } catch (error) {
      const normalized = normalizeApiError(error);
      if (clipboardMode === "cut" && cutConflictPolicy === "fail" && normalized.status === 409) {
        setPasteConflictPane(pane);
        return;
      }
      toast.error(t("manager.pasteFailed"), { description: userMessage(error) });
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
      if (!searchMode && !search.recent && command && key === "x" && selectedFiles.length > 0) {
        event.preventDefault();
        stageClipboard("cut", pane);
        return;
      }
      if (!searchMode && !search.recent && command && key === "v" && hasClipboard) {
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
      if (
        !searchMode &&
        !search.recent &&
        command &&
        event.shiftKey &&
        event.key.toLowerCase() === "n"
      ) {
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
      if (searchMode || search.bookmarked) {
        void navigateTo({
          to: "/files",
          search: {
            path: joinPath(file.parentPath ?? "/", file.name),
            parentId: file.id,
            query: "",
            view: location.view,
            bookmarked: undefined,
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
        t(selectedFiles.length === 1 ? "manager.linksCopiedOne" : "manager.linksCopiedMany", {
          count: selectedFiles.length,
        }),
      );
    } catch (error) {
      toast.error(t("manager.linksCopyFailed"), { description: userMessage(error) });
    }
  };

  const renderToolbar = (pane: PaneId) => {
    if (searchMode || search.recent) return undefined;
    const location = paneLocation(pane);
    const uploadFilesTriggerRef =
      pane === "secondary" ? secondaryUploadFilesTriggerRef : primaryUploadFilesTriggerRef;
    const uploadFolderTriggerRef =
      pane === "secondary" ? secondaryUploadFolderTriggerRef : primaryUploadFolderTriggerRef;
    return (
      <>
        <Dropdown>
          <Button size="sm" variant="secondary" aria-label={t("manager.sortFiles")}>
            {sorting.sort === "updatedAt"
              ? t("manager.sortModified")
              : sorting.sort === "size"
                ? t("search.sortSize")
                : t("search.sortName")}
            {sorting.order === "asc" ? " ↑" : " ↓"}
          </Button>
          <Dropdown.Popover>
            <Dropdown.Menu
              aria-label={t("manager.sortBy")}
              selectionMode="single"
              selectedKeys={new Set([sorting.sort])}
              onAction={(key) => changeSorting({ ...sorting, sort: key as FileSort })}
            >
              <Dropdown.Item id="name" textValue={t("search.sortName")}>
                <Label>{t("search.sortName")}</Label>
              </Dropdown.Item>
              <Dropdown.Item id="updatedAt" textValue={t("manager.sortModified")}>
                <Label>{t("manager.sortModified")}</Label>
              </Dropdown.Item>
              <Dropdown.Item id="size" textValue={t("search.sortSize")}>
                <Label>{t("search.sortSize")}</Label>
              </Dropdown.Item>
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
        <Button
          isIconOnly
          size="sm"
          variant="secondary"
          aria-label={t(sorting.order === "asc" ? "manager.orderToDesc" : "manager.orderToAsc")}
          onPress={() =>
            changeSorting({ ...sorting, order: sorting.order === "asc" ? "desc" : "asc" })
          }
        >
          {sorting.order === "asc" ? (
            <ArrowUpIcon className="size-4" aria-hidden="true" />
          ) : (
            <ArrowDownIcon className="size-4" aria-hidden="true" />
          )}
        </Button>
        {pane === "primary" && !search.split ? (
          <Button
            isIconOnly
            size="sm"
            variant="secondary"
            aria-label={t("manager.openSplit")}
            onPress={openSplitView}
          >
            <SplitIcon className="size-4" />
          </Button>
        ) : pane === "secondary" && search.split ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("manager.closeSplit")}
            onPress={closeSplitView}
          >
            <SplitIcon className="size-4" />
          </Button>
        ) : null}
        <Button
          isIconOnly
          size="sm"
          variant="secondary"
          aria-label={t("files.newFolder")}
          onPress={() => {
            setActivePane(pane);
            setFolderDialogOpen(true);
          }}
        >
          <PlusIcon className="size-4" />
        </Button>
        <Dropdown>
          <Button isIconOnly size="sm" variant="primary" aria-label={t("manager.upload")}>
            <UploadIcon className="size-4" />
          </Button>
          <Dropdown.Popover className="min-w-52">
            <Dropdown.Menu
              aria-label={t("manager.upload")}
              onAction={(key) => {
                setActivePane(pane);
                if (key === "files") uploadFilesTriggerRef.current?.click();
                if (key === "folder") uploadFolderTriggerRef.current?.click();
                if (key === "background") setBackgroundUploadOpen(true);
              }}
            >
              <Dropdown.Item id="files" textValue={t("files.uploadFiles")}>
                <FileIcon className="size-4" />
                <Label>{t("files.uploadFiles")}</Label>
              </Dropdown.Item>
              <Dropdown.Item id="folder" textValue={t("files.uploadFolder")}>
                <FolderIcon className="size-4" />
                <Label>{t("files.uploadFolder")}</Label>
              </Dropdown.Item>
              {canLocalImport ? (
                <Dropdown.Item id="background" textValue={t("uploads.bgTitle")}>
                  <UploadIcon className="size-4" />
                  <Label>{t("uploads.bgTitle")}</Label>
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
            <Button ref={uploadFilesTriggerRef}>{t("files.chooseUpload")}</Button>
          </FileTrigger>
          <FileTrigger
            acceptDirectory
            allowsMultiple
            onSelect={(list) => {
              if (list?.length) enqueue(Array.from(list), location.parentId, location.path);
            }}
          >
            <Button ref={uploadFolderTriggerRef}>{t("files.chooseFolder")}</Button>
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
      !searchMode &&
      !search.recent &&
      hasClipboard &&
      (!search.split || pane === clipboardTargetPane);
    const canPasteHere =
      showClipboard &&
      !(clipboardMode === "cut" && clipboardSourceParentId === paneLocation(pane).parentId);
    if (showClipboard) {
      return (
        <div className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center px-4">
          <div className="pointer-events-auto flex max-w-full items-center gap-1.5 rounded-full border border-border bg-surface/95 p-1.5 shadow-xl backdrop-blur">
            <span className="shrink-0 rounded-full bg-default/40 px-3 py-2 text-sm font-medium text-foreground">
              {t(clipboardMode === "cut" ? "manager.clipCut" : "manager.clipCopied", {
                count: clipboardItems.length,
              })}
            </span>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t(
                clipboardItems.length === 1 ? "manager.pasteOne" : "manager.pasteMany",
                { count: clipboardItems.length },
              )}
              isDisabled={fileActions.pending || !canPasteHere}
              onPress={() => void pasteClipboard(pane)}
            >
              <PasteIcon className="size-4" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t(clipboardMode === "cut" ? "manager.cancelCut" : "manager.clearCopied")}
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
        onCut={
          searchMode || search.bookmarked || search.recent
            ? undefined
            : () => stageClipboard("cut", pane)
        }
        onCopy={() => stageClipboard("copy", pane)}
        onBookmark={() => void toggleBookmarks(pane)}
        bookmarked={search.bookmarked}
        onRename={() => {
          const file = selectedFiles[0];
          setActivePane(pane);
          setRenameFile(file);
          setRenameName(file.name);
          setActionError(undefined);
        }}
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
        rootLabel={
          searchMode
            ? t("search.results")
            : search.recent
              ? t("title.recent")
              : search.bookmarked
                ? t("title.bookmarks")
                : t("manager.myFiles")
        }
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
        hideFolderControls={Boolean(searchMode || search.recent)}
        showLocations={Boolean(searchMode || search.bookmarked || search.recent)}
        emptyTitle={
          searchMode
            ? t("search.noMatch")
            : search.recent
              ? t("manager.noRecent")
              : search.bookmarked
                ? t("manager.noBookmarks")
                : undefined
        }
        emptyHint={
          searchMode
            ? t("search.noMatchHint")
            : search.recent
              ? t("manager.recentHint")
              : search.bookmarked
                ? t("manager.bookmarksHint")
                : undefined
        }
        onOpenContainingFolder={
          searchMode || search.bookmarked || search.recent
            ? (file) => {
                void navigateTo({
                  to: "/files",
                  search: {
                    path: file.parentPath ?? "/",
                    parentId: file.parentId,
                    query: "",
                    view: location.view,
                    bookmarked: undefined,
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
          aria-label={t("manager.dropInto", { path: location.path })}
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
                  {t("manager.dropHere", { path: location.path })}
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
          {primaryFileQuery.isFetching
            ? t("search.updating")
            : t("search.loaded", { count: primaryFiles.length })}
        </p>
      )}
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <div {...keyboardProps} className="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden">
          {searchMode && primaryFileQuery.isError && (
            <div role="alert" className="mb-3 rounded-xl border border-danger/30 p-4 text-sm">
              <p className="font-medium">
                {primaryFileQuery.isFetchNextPageError
                  ? t("search.moreFailed")
                  : t("search.failed")}
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
                {t("picker.retry")}
              </Button>
            </div>
          )}
          {searchMode && (missingFolder || invalidDates || !activeSearch) ? (
            <div className="flex flex-1 flex-col items-center justify-center rounded-2xl border border-dashed border-border p-8 text-center">
              <p className="text-lg font-semibold">
                {missingFolder
                  ? t("search.chooseRecursive")
                  : invalidDates
                    ? t("search.checkRange")
                    : t("search.findAnything")}
              </p>
              <p className="mt-2 text-sm text-muted">
                {missingFolder
                  ? t("search.noFolderSaved")
                  : invalidDates
                    ? t("search.dateError")
                    : t("search.searchHint")}
              </p>
              {(missingFolder || invalidDates) && (
                <Button variant="secondary" className="mt-3" onPress={() => setFiltersOpen(true)}>
                  {missingFolder ? t("search.chooseFolderBtn") : t("search.editFilters")}
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
        title={t("files.createFolder")}
        description={t("manager.createIn", { path: activeLocation.path })}
        size="md"
        footer={
          <>
            <Button variant="secondary" onPress={() => setFolderDialogOpen(false)}>
              {t("dialog.cancel")}
            </Button>
            <Button
              variant="primary"
              isDisabled={!folderName.trim() || fileActions.pending}
              onPress={() => void createFolder()}
            >
              {t("files.createFolder")}
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
          <Label>{t("files.folderName")}</Label>
          <Input placeholder={t("files.newFolderPlaceholder")} />
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
        title={t("manager.conflictTitle")}
        description={t("manager.conflictDesc")}
        size="md"
        footer={
          <>
            <Button variant="secondary" onPress={() => setPasteConflictPane(undefined)}>
              {t("dialog.cancel")}
            </Button>
            <Button
              variant="secondary"
              isDisabled={fileActions.pending || pasteConflictPane === undefined}
              onPress={() => {
                if (pasteConflictPane) void pasteClipboard(pasteConflictPane, "rename");
              }}
            >
              {t("manager.conflictKeep")}
            </Button>
            <Button
              variant="danger"
              isDisabled={fileActions.pending || pasteConflictPane === undefined}
              onPress={() => {
                if (pasteConflictPane) void pasteClipboard(pasteConflictPane, "replace");
              }}
            >
              {t("manager.conflictReplace")}
            </Button>
          </>
        }
      >
        <p className="text-sm text-muted">{t("manager.conflictHint")}</p>
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
