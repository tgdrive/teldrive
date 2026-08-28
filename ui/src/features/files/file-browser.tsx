import { Button, Card, Checkbox, InputGroup, Spinner, TextField } from "@heroui/react";
import type { ReactNode, RefObject } from "react";
import {
  Collection,
  GridLayout,
  GridList,
  GridListItem,
  GridListLoadMoreItem,
  ListLayout,
  type Selection,
  Virtualizer,
} from "react-aria-components";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import GridIcon from "~icons/gravity-ui/layout-cells";
import ListIcon from "~icons/gravity-ui/list-ul";
import SearchIcon from "~icons/gravity-ui/magnifier";
import type { FileEntry } from "@/api/types";

export type FileBrowserView = "list" | "grid";

type FileBrowserProps = {
  files: FileEntry[];
  path: string;
  rootLabel: string;
  view: FileBrowserView;
  query: string;
  loading: boolean;
  onQueryChange: (value: string) => void;
  onNavigatePath: (path: string) => void;
  onViewChange: (view: FileBrowserView) => void;
  onOpen: (file: FileEntry) => void;
  toolbar?: ReactNode;
  selection?: {
    selectedKeys: Selection;
    onSelectionChange: (selection: Selection) => void;
    onClearSelection: () => void;
  };
  selectionOverlay?: ReactNode;
  hasNextPage?: boolean;
  isLoadingMore?: boolean;
  onLoadMore?: () => void;
  searchInputRef?: RefObject<HTMLInputElement | null>;
  emptyHint?: string;
};

export function FileBrowser({
  files,
  path,
  rootLabel,
  view,
  query,
  loading,
  onQueryChange,
  onNavigatePath,
  onViewChange,
  onOpen,
  toolbar,
  selection,
  selectionOverlay,
  hasNextPage = false,
  isLoadingMore = false,
  onLoadMore,
  searchInputRef,
  emptyHint = "This folder is empty.",
}: FileBrowserProps) {
  return (
    <Card className="relative flex min-h-0 min-w-0 flex-1 flex-col gap-0 overflow-hidden border border-border bg-surface/80 shadow-sm">
      <Card.Header className="shrink-0 border-b border-border px-3 py-3 sm:px-4">
        <div className="flex min-w-0 items-center gap-2">
          <div className="min-w-0 flex-1 overflow-hidden">
            <FileBrowserBreadcrumb
              path={path}
              rootLabel={rootLabel}
              onNavigatePath={onNavigatePath}
            />
          </div>
          <TextField
            name="file-search"
            aria-label="Search this folder"
            value={query}
            onChange={onQueryChange}
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
            {toolbar}
            <Button
              isIconOnly
              size="sm"
              variant={view === "list" ? "secondary" : "ghost"}
              aria-label="List view"
              onPress={() => onViewChange("list")}
            >
              <ListIcon className="size-4" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant={view === "grid" ? "secondary" : "ghost"}
              aria-label="Grid view"
              onPress={() => onViewChange("grid")}
            >
              <GridIcon className="size-4" />
            </Button>
          </div>
        </div>
      </Card.Header>
      <Card.Content className="min-h-0 flex-1 overflow-hidden p-0">
        {loading ? (
          <div className="flex h-full min-h-64 items-center justify-center">
            <Spinner size="lg" />
          </div>
        ) : (
          <div className="h-full min-h-0">
            <FileCollection
              files={files}
              view={view}
              selection={selection}
              onOpen={onOpen}
              hasNextPage={hasNextPage}
              isLoadingMore={isLoadingMore}
              onLoadMore={onLoadMore}
              emptyHint={emptyHint}
            />
          </div>
        )}
      </Card.Content>
      {selectionOverlay}
    </Card>
  );
}

function FileCollection({
  files,
  view,
  selection,
  onOpen,
  hasNextPage,
  isLoadingMore,
  onLoadMore,
  emptyHint,
}: {
  files: FileEntry[];
  view: FileBrowserView;
  selection?: FileBrowserProps["selection"];
  onOpen: (file: FileEntry) => void;
  hasNextPage: boolean;
  isLoadingMore: boolean;
  onLoadMore?: () => void;
  emptyHint: string;
}) {
  const grid = view === "grid";
  const selectedKeys = selection?.selectedKeys ?? new Set<React.Key>();
  const hasSelection = selection ? selectedKeys === "all" || selectedKeys.size > 0 : false;

  return (
    <Virtualizer key={view} layout={grid ? GridLayout : ListLayout}>
      <GridList
        aria-label="Files and folders"
        layout={grid ? "grid" : "stack"}
        selectionMode={selection ? "multiple" : "none"}
        selectionBehavior="replace"
        selectedKeys={selection?.selectedKeys}
        onSelectionChange={selection?.onSelectionChange}
        onClick={(event) => {
          if (!selection) return;
          const target = event.target as HTMLElement;
          if (!target.closest('[role="row"]')) selection.onClearSelection();
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
            <p className="mt-1 text-xs text-muted">{emptyHint}</p>
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
                    selectable={Boolean(selection)}
                    showCheckbox={state.isHovered || state.isFocusVisible || state.isSelected}
                  />
                ) : (
                  <ListFile
                    file={file}
                    selectable={Boolean(selection)}
                    showCheckbox={state.isHovered || state.isFocusVisible || state.isSelected}
                  />
                )
              }
            </GridListItem>
          )}
        </Collection>
        {hasNextPage ? (
          <GridListLoadMoreItem
            onLoadMore={() => onLoadMore?.()}
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

function GridFile({
  file,
  selectable,
  showCheckbox,
}: {
  file: FileEntry;
  selectable: boolean;
  showCheckbox: boolean;
}) {
  const Icon = file.kind === "folder" ? FolderIcon : FileIcon;
  return (
    <>
      <div className="flex items-start gap-2">
        {selectable ? <SelectionCheckbox file={file} isVisible={showCheckbox} /> : null}
        <div className="flex size-11 items-center justify-center rounded-xl bg-accent/10 text-accent">
          <Icon className="size-5" />
        </div>
      </div>
      <div className="min-w-0">
        <p className="truncate text-sm font-medium group-hover:text-accent">{file.name}</p>
        <p className="mt-1 text-[11px] text-muted">
          {file.kind === "folder" ? "Folder" : formatFileBytes(file.size ?? 0)}
        </p>
      </div>
    </>
  );
}

function ListFile({
  file,
  selectable,
  showCheckbox,
}: {
  file: FileEntry;
  selectable: boolean;
  showCheckbox: boolean;
}) {
  const Icon = file.kind === "folder" ? FolderIcon : FileIcon;
  return (
    <>
      <div className="flex min-w-0 items-center gap-3">
        {selectable ? <SelectionCheckbox file={file} isVisible={showCheckbox} /> : null}
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent">
          <Icon className="size-4" />
        </div>
        <span className="truncate text-sm font-medium group-hover:text-accent">{file.name}</span>
      </div>
      <span className="hidden text-xs text-muted sm:block">
        {file.kind === "folder" ? "Folder" : formatFileBytes(file.size ?? 0)}
      </span>
      <span className="hidden text-xs text-muted lg:block">
        {new Date(file.modTime).toLocaleString()}
      </span>
    </>
  );
}

function FileBrowserBreadcrumb({
  path,
  rootLabel,
  onNavigatePath,
}: {
  path: string;
  rootLabel: string;
  onNavigatePath: (path: string) => void;
}) {
  const parts = path.split("/").filter(Boolean);
  return (
    <nav
      aria-label="Current folder"
      className="flex min-w-0 items-center gap-1 overflow-hidden text-sm"
    >
      <Button
        size="sm"
        variant="ghost"
        className="h-auto min-w-0 rounded-lg px-2 py-1 text-muted hover:text-foreground"
        onPress={() => onNavigatePath("/")}
      >
        {rootLabel}
      </Button>
      {parts.map((part, index) => {
        const currentPath = `/${parts.slice(0, index + 1).join("/")}`;
        return (
          <span key={currentPath} className="flex min-w-0 items-center gap-1">
            <span className="text-muted">/</span>
            <Button
              size="sm"
              variant="ghost"
              className="h-auto min-w-0 truncate rounded-lg px-2 py-1 text-muted hover:text-foreground"
              onPress={() => onNavigatePath(currentPath)}
            >
              {part}
            </Button>
          </span>
        );
      })}
    </nav>
  );
}

export function formatFileBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}
