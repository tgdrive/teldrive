import { Button, Card, Checkbox, Spinner } from "@heroui/react";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import {
  Collection,
  GridLayout,
  GridList,
  GridListItem,
  GridListLoadMoreItem,
  ListLayout,
  type Selection,
  Size,
  Virtualizer,
} from "react-aria-components";
import type { FileEntry } from "@/api/types";
import { useAppLocale } from "@/lib/locale";
import { useDateOnlyFormatter, useDateTimeFormatter, useFormatBytes } from "@/lib/format";
import BackIcon from "~icons/gravity-ui/arrow-left";
import UpIcon from "~icons/gravity-ui/chevron-up";
import FolderIcon from "~icons/gravity-ui/folder";
import GridIcon from "~icons/gravity-ui/layout-cells";
import ListIcon from "~icons/gravity-ui/list-ul";
import { FileTypeIcon } from "./file-type-icon";

export type FileBrowserView = "list" | "grid";

const fileGridLayoutOptions = {
  // Estimate a single-line filename; the virtualizer measures wrapped names.
  minItemSize: new Size(144, 160),
  maxItemSize: new Size(208, 204),
  minSpace: new Size(12, 12),
  maxHorizontalSpace: 12,
};

const locatedFileGridLayoutOptions = {
  ...fileGridLayoutOptions,
  minItemSize: new Size(144, 180),
  maxItemSize: new Size(208, 224),
};

type FileBrowserProps = {
  files: FileEntry[];
  path: string;
  rootLabel: string;
  view: FileBrowserView;
  loading: boolean;
  onNavigatePath: (path: string) => void;
  onViewChange: (view: FileBrowserView) => void;
  onOpen: (file: FileEntry) => void;
  toolbar?: ReactNode;
  compactToolbar?: boolean;
  emptyContent?: ReactNode;
  onBack?: () => void;
  selection?: {
    selectedKeys: Selection;
    onSelectionChange: (selection: Selection) => void;
    onClearSelection: () => void;
  };
  selectionOverlay?: ReactNode;
  selectionDisabled?: boolean;

  dimmedIds?: ReadonlySet<string>;
  hasNextPage?: boolean;
  isLoadingMore?: boolean;
  onLoadMore?: () => void;
  emptyHint?: string;
  emptyTitle?: string;
  hideFolderControls?: boolean;
  showLocations?: boolean;
  onOpenContainingFolder?: (file: FileEntry) => void;
};

export function FileBrowser({
  files,
  path,
  rootLabel,
  view,
  loading,
  onNavigatePath,
  onViewChange,
  onOpen,
  toolbar,
  compactToolbar = false,
  emptyContent,
  onBack,
  selection,
  selectionOverlay,
  selectionDisabled = false,

  dimmedIds,
  hasNextPage = false,
  isLoadingMore = false,
  onLoadMore,
  emptyHint,
  emptyTitle,
  hideFolderControls = false,
  showLocations = false,
  onOpenContainingFolder,
}: FileBrowserProps) {
  const { t } = useAppLocale();
  const resolvedEmptyHint = emptyHint ?? t("browser.emptyHint");
  const resolvedEmptyTitle = emptyTitle ?? t("browser.emptyTitle");
  return (
    <Card className="@container/file-browser relative flex min-h-0 min-w-0 flex-1 flex-col gap-0 overflow-hidden border border-border bg-surface/80 shadow-sm">
      <Card.Header
        className={`shrink-0 border-b border-border px-3 sm:px-4 ${compactToolbar ? "py-2" : "py-3"}`}
        data-testid="file-browser-toolbar"
      >
        <div className="flex min-w-0 flex-wrap items-center gap-1.5 [&>button]:shrink-0">
          {onBack ? (
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t("browser.back")}
              onPress={onBack}
            >
              <BackIcon className="size-4" />
            </Button>
          ) : null}
          {!hideFolderControls && (
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t("browser.up")}
              isDisabled={path === "/"}
              onPress={() => {
                const parts = path.split("/").filter(Boolean);
                const parentPath = parts.length <= 1 ? "/" : `/${parts.slice(0, -1).join("/")}`;
                onNavigatePath(parentPath);
              }}
            >
              <UpIcon className="size-4" />
            </Button>
          )}
          {!hideFolderControls && (
            <div className="min-w-20 flex-1 overflow-hidden">
              <FileBrowserBreadcrumb
                path={path}
                rootLabel={rootLabel}
                onNavigatePath={onNavigatePath}
              />
            </div>
          )}
          <div className="flex w-full min-w-0 flex-wrap items-center gap-1 sm:w-auto sm:flex-1 sm:justify-end [&>button]:shrink-0">
            {toolbar}
            <Button
              isIconOnly
              size="sm"
              variant={view === "list" ? "secondary" : "ghost"}
              aria-label={t("browser.listView")}
              onPress={() => onViewChange("list")}
            >
              <ListIcon className="size-4" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant={view === "grid" ? "secondary" : "ghost"}
              aria-label={t("browser.gridView")}
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
        ) : emptyContent ? (
          emptyContent
        ) : (
          <div className="h-full min-h-0">
            <FileCollection
              files={files}
              view={view}
              selection={selection}
              selectionDisabled={selectionDisabled}
              onOpen={onOpen}
              dimmedIds={dimmedIds}
              hasNextPage={hasNextPage}
              isLoadingMore={isLoadingMore}
              onLoadMore={onLoadMore}
              emptyHint={resolvedEmptyHint}
              emptyTitle={resolvedEmptyTitle}
              showLocations={showLocations}
              onOpenContainingFolder={onOpenContainingFolder}
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
  selectionDisabled,
  onOpen,

  dimmedIds,
  hasNextPage,
  isLoadingMore,
  onLoadMore,
  emptyHint,
  emptyTitle,
  showLocations,
  onOpenContainingFolder,
}: {
  files: FileEntry[];
  view: FileBrowserView;
  selection?: FileBrowserProps["selection"];
  selectionDisabled: boolean;
  onOpen: (file: FileEntry) => void;

  dimmedIds?: ReadonlySet<string>;
  hasNextPage: boolean;
  isLoadingMore: boolean;
  onLoadMore?: () => void;
  emptyHint: string;
  emptyTitle: string;
  showLocations: boolean;
  onOpenContainingFolder?: (file: FileEntry) => void;
}) {
  const grid = view === "grid";
  const { t } = useAppLocale();
  const selectedKeys = selection?.selectedKeys ?? new Set<React.Key>();
  const hasSelection = selection ? selectedKeys === "all" || selectedKeys.size > 0 : false;
  const gridRef = useRef<HTMLDivElement>(null);
  const [gridWidth, setGridWidth] = useState(0);

  useLayoutEffect(() => {
    const element = gridRef.current;
    if (!grid || !element) return;
    setGridWidth(element.clientWidth);
    const observer = new ResizeObserver(() => setGridWidth(element.clientWidth));
    observer.observe(element);
    return () => observer.disconnect();
  }, [grid]);

  const collection = (
    <GridList
      ref={gridRef}
      aria-label={t("browser.files")}
      layout={grid ? "grid" : "stack"}
      selectionMode={selection ? "multiple" : "none"}
      selectionBehavior="replace"
      selectedKeys={selection?.selectedKeys}
      onSelectionChange={selection?.onSelectionChange}
      disabledKeys={selectionDisabled ? files.map((file) => file.id) : undefined}
      disabledBehavior="selection"
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
          <p className="text-sm font-medium">{emptyTitle}</p>
          <p className="mt-1 text-xs text-muted">{emptyHint}</p>
        </div>
      )}
      className={`h-full min-h-0 overflow-x-hidden overflow-y-auto outline-none ${hasSelection ? "pb-24" : ""}`}
    >
      <Collection items={files} dependencies={[dimmedIds, showLocations, onOpenContainingFolder]}>
        {(file) => (
          <GridListItem
            id={file.id}
            textValue={file.name}
            className={(state) =>
              grid
                ? [
                    "group relative flex min-w-0 cursor-default flex-col rounded-md border border-transparent p-2 outline-none",
                    state.isSelected ? "border-accent/40 bg-accent/10" : "hover:bg-default/20",

                    dimmedIds?.has(file.id) && "opacity-45",
                    state.isFocusVisible && "ring-2 ring-inset ring-accent",
                  ]
                    .filter(Boolean)
                    .join(" ")
                : [
                    "group grid min-h-14 cursor-default grid-cols-[minmax(0,1fr)] items-center gap-2 border-b border-border px-3 outline-none transition-colors last:border-b-0 sm:grid-cols-[minmax(0,1fr)_6rem] sm:px-4 lg:grid-cols-[minmax(0,1fr)_8rem_10rem] lg:gap-4",
                    "hover:bg-default/20",
                    state.isSelected && "bg-accent/10",

                    dimmedIds?.has(file.id) && "opacity-45",
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
                  location={showLocations ? file.parentPath : undefined}
                  onOpenContainingFolder={
                    onOpenContainingFolder ? () => onOpenContainingFolder(file) : undefined
                  }
                />
              ) : (
                <ListFile
                  file={file}
                  selectable={Boolean(selection)}
                  showCheckbox={state.isHovered || state.isFocusVisible || state.isSelected}
                  location={showLocations ? file.parentPath : undefined}
                  onOpenContainingFolder={
                    onOpenContainingFolder ? () => onOpenContainingFolder(file) : undefined
                  }
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
  );

  return grid ? (
    <Virtualizer
      key="grid"
      layout={GridLayout}
      layoutOptions={{
        ...(showLocations ? locatedFileGridLayoutOptions : fileGridLayoutOptions),
        // Prefer ~196px cards, while allowing two smaller tiles in narrow panes.
        maxColumns: Math.max(2, Math.floor((gridWidth + 12) / 208)),
      }}
    >
      {collection}
    </Virtualizer>
  ) : (
    <Virtualizer key="list" layout={ListLayout}>
      {collection}
    </Virtualizer>
  );
}

function SelectionCheckbox({ file, isVisible }: { file: FileEntry; isVisible: boolean }) {
  const { t } = useAppLocale();
  return (
    <Checkbox
      slot="selection"
      aria-label={t("browser.selectName", { name: file.name })}
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
  location,
  onOpenContainingFolder,
}: {
  file: FileEntry;
  selectable: boolean;
  showCheckbox: boolean;
  location?: string;
  onOpenContainingFolder?: () => void;
}) {
  const { t } = useAppLocale();
  const formatBytes = useFormatBytes();
  const formatDate = useDateOnlyFormatter();
  const formatDateTime = useDateTimeFormatter();
  const modified = new Date(file.modTime);
  return (
    <>
      {selectable ? (
        <div className="absolute start-3 top-3 z-10">
          <SelectionCheckbox file={file} isVisible={showCheckbox} />
        </div>
      ) : null}
      <div
        className="flex aspect-[16/11] w-full shrink-0 items-center justify-center rounded-md bg-default/25"
        aria-hidden="true"
        data-testid="file-grid-preview"
      >
        {file.kind === "folder" ? (
          <FolderIcon className="size-12 text-accent" />
        ) : (
          <FileTypeIcon file={file} className="size-12 text-muted" />
        )}
      </div>
      <p
        className="mt-2 line-clamp-2 shrink-0 text-sm leading-5 [overflow-wrap:anywhere]"
        title={file.name}
      >
        {file.name}
      </p>
      {location && (
        <LocationButton
          location={location}
          onPress={onOpenContainingFolder}
          className="mt-1 h-4 shrink-0"
        />
      )}
      <div className="mt-1 flex min-w-0 items-center justify-between gap-2 text-[11px] leading-4 text-muted">
        <time className="truncate" dateTime={file.modTime} title={formatDateTime.format(modified)}>
          {formatDate.format(modified)}
        </time>
        <span className="shrink-0 text-end">
          {file.kind === "folder"
            ? file.size != null
              ? formatBytes(file.size)
              : t("browser.folderKind")
            : formatBytes(file.size ?? 0)}
        </span>
      </div>
    </>
  );
}

function ListFile({
  file,
  selectable,
  showCheckbox,
  location,
  onOpenContainingFolder,
}: {
  file: FileEntry;
  selectable: boolean;
  showCheckbox: boolean;
  location?: string;
  onOpenContainingFolder?: () => void;
}) {
  const { t } = useAppLocale();
  const formatBytes = useFormatBytes();
  const formatDateTime = useDateTimeFormatter();
  return (
    <>
      <div className="flex min-w-0 items-center gap-3">
        {selectable ? <SelectionCheckbox file={file} isVisible={showCheckbox} /> : null}
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent">
          {file.kind === "folder" ? (
            <FolderIcon className="size-4" />
          ) : (
            <FileTypeIcon file={file} className="size-4" />
          )}
        </div>
        <div className="min-w-0">
          <span
            className="block truncate text-sm font-medium group-hover:text-accent"
            title={file.name}
          >
            {file.name}
          </span>
          {location ? (
            <LocationButton location={location} onPress={onOpenContainingFolder} />
          ) : null}
        </div>
      </div>
      <span className="hidden text-xs text-muted sm:block">
        {file.kind === "folder"
          ? file.size != null
            ? formatBytes(file.size)
            : t("browser.folderKind")
          : formatBytes(file.size ?? 0)}
      </span>
      <time className="hidden min-w-0 text-xs text-muted lg:block" dateTime={file.modTime}>
        {formatDateTime.format(new Date(file.modTime))}
      </time>
    </>
  );
}

function LocationButton({
  location,
  onPress,
  className = "",
}: {
  location: string;
  onPress?: () => void;
  className?: string;
}) {
  const { t } = useAppLocale();
  return (
    <Button
      size="sm"
      variant="ghost"
      className={`h-auto min-h-0 max-w-full justify-start rounded-sm px-0 py-0 text-[11px] text-muted ${className}`}
      aria-label={t("browser.openFolder", { location })}
      onPress={onPress}
    >
      <span className="truncate">{location}</span>
    </Button>
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
  const lastIndex = parts.length - 1;
  const { t } = useAppLocale();
  return (
    <nav
      aria-label={t("browser.currentFolder")}
      className="flex min-w-0 items-center gap-1 overflow-hidden text-sm"
    >
      <Button
        size="sm"
        variant="ghost"
        aria-label={rootLabel}
        className="h-8 max-w-9 shrink-0 gap-1.5 overflow-hidden rounded-lg px-2 text-muted hover:text-foreground @lg/file-browser:max-w-44"
        onPress={() => onNavigatePath("/")}
      >
        <FolderIcon className="size-4 shrink-0" />
        <span className="hidden truncate @lg/file-browser:inline">{rootLabel}</span>
      </Button>
      {parts.map((part, index) => {
        const currentPath = `/${parts.slice(0, index + 1).join("/")}`;
        const isCurrent = index === lastIndex;
        const isParent = index === lastIndex - 1;
        const visibility = isCurrent
          ? "flex"
          : isParent
            ? "hidden @md/file-browser:flex"
            : "hidden @4xl/file-browser:flex";
        return (
          <span key={currentPath} className={`${visibility} min-w-0 items-center gap-1`}>
            <span className="shrink-0 text-muted/60">/</span>
            {isCurrent ? (
              <span
                aria-current="page"
                className="min-w-0 truncate px-1 text-sm font-medium text-foreground"
                title={part}
              >
                {part}
              </span>
            ) : (
              <Button
                size="sm"
                variant="ghost"
                className="h-8 min-w-0 max-w-40 truncate rounded-lg px-2 text-muted hover:text-foreground"
                onPress={() => onNavigatePath(currentPath)}
              >
                {part}
              </Button>
            )}
          </span>
        );
      })}
    </nav>
  );
}
