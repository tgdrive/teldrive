import { Button, Dropdown, Input, Label, Spinner, TextField } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  useDeferredValue,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { DropZone, FileTrigger, type Selection } from "react-aria-components";

import { useKeyboard } from "react-aria";
import { toast } from "sonner";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import CopyIcon from "~icons/gravity-ui/copy";
import CopyLinkIcon from "~icons/gravity-ui/copy-arrow-right";
import PasteIcon from "~icons/gravity-ui/arrow-right-to-square";
import CutIcon from "~icons/gravity-ui/scissors";
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
import { useFileClipboardStore } from "../features/files/clipboard-store";
import { ShareDialog } from "../features/files/share-dialog";
import { FolderPicker } from "../features/files/folder-picker";
import { absoluteFileDownloadUrl, copyText, startFileDownload } from "../features/files/download";
import { useFileActions } from "../features/files/mutations";
import { useInfiniteFilePages } from "../features/files/queries";
import { FileTabs, FileTabNavigation } from "../features/files/tabs/file-tabs";
import { selectionForTab, useFileTabsStore } from "../features/files/tabs/store";
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
  const tabs = useFileTabsStore((state) => state.tabs);
  const activeTabId = useFileTabsStore((state) => state.activeTabId);
  const hydrateTabs = useFileTabsStore((state) => state.hydrate);
  const activateTab = useFileTabsStore((state) => state.activate);
  const newTab = useFileTabsStore((state) => state.newTab);
  const closeTab = useFileTabsStore((state) => state.close);
  const reopenClosedTab = useFileTabsStore((state) => state.reopenClosed);
  const navigateTab = useFileTabsStore((state) => state.navigate);
  const backTab = useFileTabsStore((state) => state.back);
  const forwardTab = useFileTabsStore((state) => state.forward);
  const updateTab = useFileTabsStore((state) => state.update);
  const setTabSelection = useFileTabsStore((state) => state.setSelection);
  const activeTab = tabs.find((tab) => tab.id === activeTabId) ?? tabs[0];
  const hydratedRef = useRef(false);
  const [queryDraft, setQueryDraft] = useState(search.query);
  const deferredQuery = useDeferredValue(queryDraft.trim());
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
  const clipboardMode = useFileClipboardStore((state) => state.mode);
  const clipboardItems = useFileClipboardStore((state) => state.items);
  const clipboardSourceParentId = useFileClipboardStore((state) => state.sourceParentId);
  const setClipboard = useFileClipboardStore((state) => state.set);
  const clearClipboard = useFileClipboardStore((state) => state.clear);

  useEffect(() => {
    if (hydratedRef.current) return;
    hydratedRef.current = true;
    hydrateTabs(search);
  }, [hydrateTabs, search]);

  useEffect(() => {
    if (!hydratedRef.current || !activeTab) return;
    setQueryDraft(activeTab.query);
    navigate({
      search: {
        path: activeTab.path,
        parentId: activeTab.parentId,
        query: activeTab.query,
        view: activeTab.view,
      },
      replace: true,
    });
  }, [activeTab?.path, activeTab?.parentId, activeTab?.query, activeTab?.view, navigate]);

  const fileQuery = useInfiniteFilePages(
    {
      path: activeTab?.path ?? search.path,
      parentId: activeTab?.parentId,
      q: activeTab?.query || undefined,
      sort: "name",
      order: "asc",
      view: activeTab?.view ?? search.view,
    },
    "active",
  );
  const files = fileQuery.data?.pages.flatMap((page) => page.items) ?? [];
  const selectedKeys: Selection = activeTab
    ? selectionForTab(
        activeTab,
        files.map((file) => file.id),
      )
    : new Set();

  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (!activeTab || deferredQuery === activeTab.query) return;
      updateTab(activeTab.id, { query: deferredQuery });
      navigate({
        search: {
          path: activeTab.path,
          parentId: activeTab.parentId,
          query: deferredQuery,
          view: activeTab.view,
        },
        replace: true,
      });
    }, 250);
    return () => window.clearTimeout(timer);
  }, [activeTab, deferredQuery, navigate, updateTab]);

  const setSelectedKeys = (selection: Selection) => {
    if (activeTab) setTabSelection(activeTab.id, selection);
  };
  const selectedCount = selectedKeys === "all" ? files.length : selectedKeys.size;
  const selectedIds =
    selectedKeys === "all" ? files.map((file) => file.id) : Array.from(selectedKeys, String);
  const selectedFiles = files.filter((file) => selectedIds.includes(file.id));
  const selectedOnlyFiles =
    selectedFiles.length > 0 && selectedFiles.every((file) => file.kind === "file");
  const singleSelectedFile = selectedFiles.length === 1 ? selectedFiles[0] : undefined;
  const cutIds =
    clipboardMode === "cut" ? new Set(clipboardItems.map((file) => file.id)) : undefined;
  const hasClipboard = Boolean(clipboardMode && clipboardItems.length > 0);
  const canPasteHere =
    hasClipboard && !(clipboardMode === "cut" && clipboardSourceParentId === activeTab?.parentId);

  const stageClipboard = (mode: "copy" | "cut") => {
    if (!activeTab || selectedFiles.length === 0) return;
    setClipboard(mode, selectedFiles, activeTab.parentId, activeTab.path);
    toast.success(
      `${selectedFiles.length} item${selectedFiles.length === 1 ? "" : "s"} ${mode === "cut" ? "cut" : "copied"}`,
    );
  };

  const pasteClipboard = async () => {
    if (!activeTab || !clipboardMode || clipboardItems.length === 0) return;
    if (clipboardMode === "cut" && clipboardSourceParentId === activeTab.parentId) {
      toast.info("Items are already in this folder");
      return;
    }
    try {
      if (clipboardMode === "copy") {
        await fileActions.copyMany(clipboardItems, activeTab.parentId, "rename");
      } else if (clipboardItems.length === 1) {
        await fileActions.move(clipboardItems[0], activeTab.parentId, "rename");
      } else {
        await fileActions.bulkMove(
          clipboardItems.map((file) => file.id),
          activeTab.parentId,
          "rename",
        );
      }
      const count = clipboardItems.length;
      const action = clipboardMode === "copy" ? "copied" : "moved";
      if (clipboardMode === "cut") clearClipboard();
      setSelectedKeys(new Set());
      toast.success(`${count} item${count === 1 ? "" : "s"} ${action}`);
    } catch (error) {
      toast.error("Clipboard items could not be pasted", { description: userMessage(error) });
    }
  };

  const createFolder = async () => {
    const name = folderName.trim();
    if (!name) return;
    try {
      await fileActions.createFolder(name, activeTab?.parentId);
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
      await fileActions.copy(file, activeTab?.parentId, `${file.name} copy`, "rename");
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
    if (!activeTab || activeTab.path === "/") return;
    const parts = activeTab.path.split("/").filter(Boolean);
    const parentPath = parts.length <= 1 ? "/" : `/${parts.slice(0, -1).join("/")}`;
    navigateTab(activeTab.id, { path: parentPath });
  };

  const switchTab = (id: string) => {
    activateTab(id);
    setPreviewFile(undefined);
    setRenameFile(undefined);
    setMoveDialogOpen(false);
    setShareFile(undefined);
  };

  const openNewTab = (file?: FileEntry) => {
    if (file?.kind === "folder" && activeTab) {
      newTab({
        path: joinPath(activeTab.path, file.name),
        parentId: file.id,
        title: file.name,
        view: activeTab.view,
      });
      return;
    }
    newTab({ view: activeTab?.view ?? "list" });
  };

  const handleFileShortcut = useEffectEvent(
    (event: ReactKeyboardEvent<HTMLElement> & { continuePropagation?: () => void }) => {
      if (event.defaultPrevented) {
        event.continuePropagation?.();
        return;
      }
      if (event.key === "Escape") {
        setFolderDialogOpen(false);
        setRenameFile(undefined);
        setMoveDialogOpen(false);
        setShareFile(undefined);
        setPreviewFile(undefined);
        setSelectedKeys(new Set());
        clearClipboard();
        return;
      }
      if (isEditableTarget(event.target)) {
        event.continuePropagation?.();
        return;
      }
      const command = event.ctrlKey || event.metaKey;
      const key = event.key.toLowerCase();

      if (command && event.shiftKey && key === "t") {
        event.preventDefault();
        reopenClosedTab();
        return;
      }
      if (command && key === "t") {
        event.preventDefault();
        openNewTab();
        return;
      }
      if (command && key === "w" && activeTab && !activeTab.pinned && tabs.length > 1) {
        event.preventDefault();
        closeTab(activeTab.id);
        return;
      }
      if (command && event.key === "Tab" && tabs.length > 1) {
        event.preventDefault();
        const currentIndex = tabs.findIndex((tab) => tab.id === activeTabId);
        const delta = event.shiftKey ? -1 : 1;
        const nextIndex = (currentIndex + delta + tabs.length) % tabs.length;
        switchTab(tabs[nextIndex].id);
        return;
      }
      if (event.altKey && event.key === "ArrowLeft" && activeTab?.historyIndex) {
        event.preventDefault();
        backTab(activeTab.id);
        return;
      }
      if (
        event.altKey &&
        event.key === "ArrowRight" &&
        activeTab &&
        activeTab.historyIndex < activeTab.history.length - 1
      ) {
        event.preventDefault();
        forwardTab(activeTab.id);
        return;
      }
      if (event.altKey && event.key === "ArrowUp") {
        event.preventDefault();
        navigateToParent();
        return;
      }
      if (command && key === "c" && selectedFiles.length > 0) {
        event.preventDefault();
        stageClipboard("copy");
        return;
      }
      if (command && key === "x" && selectedFiles.length > 0) {
        event.preventDefault();
        stageClipboard("cut");
        return;
      }
      if (command && key === "v" && hasClipboard) {
        event.preventDefault();
        void pasteClipboard();
        return;
      }
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
      if (command && event.shiftKey && key === "n") {
        event.preventDefault();
        setFolderDialogOpen(true);
        return;
      }
      if (command && key === "f") {
        event.preventDefault();
        const input =
          searchInputRef.current ??
          document.querySelector<HTMLInputElement>('input[aria-label="Search this folder"]');
        input?.focus();
        input?.select();
        return;
      }
      event.continuePropagation?.();
    },
  );

  const { keyboardProps } = useKeyboard({ onKeyDown: handleFileShortcut });

  const openFile = (file: FileEntry) => {
    if (file.kind === "folder" && activeTab) {
      navigateTab(
        activeTab.id,
        { path: joinPath(activeTab.path, file.name), parentId: file.id },
        file.name,
      );
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
    <Page
      onKeyDownCapture={keyboardProps.onKeyDown}
      autoFocus
      tabIndex={-1}
      className="h-full min-h-0 gap-0 overflow-x-hidden"
    >
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <DropZone
          data-testid="file-drop-zone"
          aria-label={`Upload files into ${activeTab?.path ?? "/"}`}
          getDropOperation={() => "copy"}
          className="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden outline-none"
          onDrop={async (event) => {
            const dropped = await Promise.all(
              event.items.filter((item) => item.kind === "file").map((item) => item.getFile()),
            );
            if (dropped.length) enqueue(dropped, activeTab?.parentId, activeTab?.path ?? "/");
          }}
        >
          {({ isDropTarget }) => (
            <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-x-hidden">
              {isDropTarget ? (
                <div className="shrink-0 rounded-xl border-2 border-dashed border-accent bg-accent/10 px-4 py-4 text-center text-sm font-medium text-accent sm:px-6 sm:py-6">
                  Drop files to upload into {activeTab?.path ?? "/"}
                </div>
              ) : null}
              <FileTabs onSwitch={switchTab} onNew={() => openNewTab()} />
              <FileBrowser
                files={files}
                path={activeTab?.path ?? "/"}
                rootLabel="My files"
                view={activeTab?.view ?? "list"}
                query={queryDraft}
                loading={fileQuery.isPending}
                onQueryChange={setQueryDraft}
                onNavigatePath={(path) => activeTab && navigateTab(activeTab.id, { path })}
                onViewChange={(view) => activeTab && updateTab(activeTab.id, { view })}
                onOpen={openFile}
                onOpenInNewTab={openNewTab}
                headerLeading={
                  <FileTabNavigation
                    canBack={Boolean(activeTab && activeTab.historyIndex > 0)}
                    canForward={Boolean(
                      activeTab && activeTab.historyIndex < activeTab.history.length - 1,
                    )}
                    canUp={Boolean(activeTab && activeTab.path !== "/")}
                    onBack={() => activeTab && backTab(activeTab.id)}
                    onForward={() => activeTab && forwardTab(activeTab.id)}
                    onUp={navigateToParent}
                  />
                }
                searchInputRef={searchInputRef}
                selection={{
                  selectedKeys,
                  onSelectionChange: setSelectedKeys,
                  onClearSelection: () => setSelectedKeys(new Set()),
                }}
                dimmedIds={cutIds}
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
                          if (list?.length)
                            enqueue(Array.from(list), activeTab?.parentId, activeTab?.path ?? "/");
                        }}
                      >
                        <Button ref={uploadFilesTriggerRef}>Choose upload files</Button>
                      </FileTrigger>
                      <FileTrigger
                        acceptDirectory
                        allowsMultiple
                        onSelect={(list) => {
                          if (list?.length)
                            enqueue(Array.from(list), activeTab?.parentId, activeTab?.path ?? "/");
                        }}
                      >
                        <Button ref={uploadFolderTriggerRef}>Choose upload folder</Button>
                      </FileTrigger>
                    </span>
                  </>
                }
                selectionOverlay={
                  selectedCount > 0 || hasClipboard ? (
                    <div className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center px-4">
                      <div className="pointer-events-auto flex max-w-full items-center gap-1.5 overflow-x-auto rounded-full border border-border bg-surface/95 p-1.5 shadow-xl backdrop-blur">
                        {selectedCount > 0 ? (
                          <span className="shrink-0 rounded-full bg-accent/10 px-3 py-2 text-sm font-medium text-accent">
                            {selectedCount} selected
                          </span>
                        ) : null}
                        {selectedCount > 0 ? (
                          <>
                            <Button
                              isIconOnly
                              size="sm"
                              variant="ghost"
                              aria-label="Cut selected items"
                              isDisabled={fileActions.pending}
                              onPress={() => stageClipboard("cut")}
                            >
                              <CutIcon className="size-4" />
                            </Button>
                            <Button
                              isIconOnly
                              size="sm"
                              variant="ghost"
                              aria-label="Copy selected items"
                              isDisabled={fileActions.pending}
                              onPress={() => stageClipboard("copy")}
                            >
                              <CopyIcon className="size-4" />
                            </Button>
                          </>
                        ) : null}
                        {hasClipboard ? (
                          <Button
                            isIconOnly
                            size="sm"
                            variant="ghost"
                            aria-label={`Paste ${clipboardItems.length} clipboard item${clipboardItems.length === 1 ? "" : "s"}`}
                            isDisabled={fileActions.pending || !canPasteHere}
                            onPress={() => void pasteClipboard()}
                          >
                            <PasteIcon className="size-4" />
                          </Button>
                        ) : null}
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
                        {selectedCount > 0 ? (
                          <>
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
                          </>
                        ) : null}
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
        description={`Create a folder inside ${activeTab?.path ?? "/"}.`}
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
        currentPath={activeTab?.path ?? "/"}
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
