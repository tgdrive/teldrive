import { Button, Dropdown, Label } from "@heroui/react";
import { useMemo } from "react";
import {
  Button as AriaButton,
  GridList,
  GridListItem,
  useDragAndDrop,
} from "react-aria-components";
import FolderIcon from "~icons/gravity-ui/folder";
import PlusIcon from "~icons/gravity-ui/plus";
import CloseIcon from "~icons/gravity-ui/xmark";
import MoreIcon from "~icons/gravity-ui/ellipsis";
import PinIcon from "~icons/gravity-ui/pin";
import ArrowBackIcon from "~icons/gravity-ui/arrow-left";
import ArrowForwardIcon from "~icons/gravity-ui/arrow-right";
import ArrowUpIcon from "~icons/gravity-ui/arrow-up";
import { useUploadStore } from "@/features/uploads/store";
import { useFileTabsStore } from "./store";

type FileTabsProps = {
  onSwitch: (id: string) => void;
  onNew: () => void;
};

export function FileTabs({ onSwitch, onNew }: FileTabsProps) {
  const tabs = useFileTabsStore((state) => state.tabs);
  const activeTabId = useFileTabsStore((state) => state.activeTabId);
  const close = useFileTabsStore((state) => state.close);
  const reorder = useFileTabsStore((state) => state.reorder);
  const duplicate = useFileTabsStore((state) => state.duplicate);
  const closeOthers = useFileTabsStore((state) => state.closeOthers);
  const closeRight = useFileTabsStore((state) => state.closeRight);
  const togglePinned = useFileTabsStore((state) => state.togglePinned);
  const reopenClosed = useFileTabsStore((state) => state.reopenClosed);
  const closedTabs = useFileTabsStore((state) => state.closedTabs);
  const tasks = useUploadStore((state) => state.tasks);

  const activeUploads = useMemo(() => {
    const paths = new Set(
      tasks
        .filter((task) => task.status === "queued" || task.status === "running")
        .map((task) => task.path),
    );
    return paths;
  }, [tasks]);

  const { dragAndDropHooks } = useDragAndDrop({
    getItems: (keys) => Array.from(keys, (key) => ({ "text/plain": String(key) })),
    onReorder: (event) => {
      const sourceId = String(Array.from(event.keys)[0] ?? "");
      const targetId = String(event.target.key);
      const placement = event.target.dropPosition;
      if (!sourceId || (placement !== "before" && placement !== "after")) return;
      reorder(sourceId, targetId, placement);
    },
  });

  return (
    <div className="shrink-0 border-b border-border bg-default/10">
      <div className="hidden min-w-0 items-end gap-1 px-2 pt-2 sm:flex">
        <GridList
          aria-label="Open file tabs"
          orientation="horizontal"
          items={tabs}
          selectionMode="single"
          selectionBehavior="replace"
          selectedKeys={new Set([activeTabId])}
          onSelectionChange={(selection) => {
            if (selection === "all") return;
            const id = Array.from(selection)[0];
            if (id) onSwitch(String(id));
          }}
          dragAndDropHooks={dragAndDropHooks}
          className="flex min-w-0 w-fit max-w-[calc(100%-2.75rem)] items-end gap-1 overflow-x-auto outline-none"
        >
          {(tab) => {
            const uploading = activeUploads.has(tab.path);
            return (
              <GridListItem
                id={tab.id}
                textValue={tab.title}
                data-path={tab.path}
                aria-label={`${tab.title} file tab`}
                onAuxClick={(event) => {
                  if (event.button === 1 && !tab.pinned) close(tab.id);
                }}
                className={({ isSelected, isDragging, isFocusVisible }) =>
                  [
                    "group relative flex h-10 min-w-28 max-w-56 shrink-0 items-center gap-2 rounded-t-xl border border-b-0 px-3 text-sm outline-none transition",
                    isSelected
                      ? "border-border bg-surface text-foreground shadow-sm"
                      : "border-transparent bg-default/20 text-muted hover:bg-default/35 hover:text-foreground",
                    isDragging ? "opacity-50" : "",
                    isFocusVisible ? "border-accent/40" : "",
                  ]
                    .filter(Boolean)
                    .join(" ")
                }
              >
                {({ allowsDragging }) => (
                  <>
                    {allowsDragging ? (
                      <AriaButton
                        slot="drag"
                        aria-label={`Reorder ${tab.title} tab`}
                        className="shrink-0 cursor-grab rounded-md outline-none focus-visible:outline-none focus-visible:ring-0 active:cursor-grabbing"
                      >
                        <FolderIcon className="size-4 text-accent" />
                      </AriaButton>
                    ) : (
                      <FolderIcon className="size-4 shrink-0 text-accent" />
                    )}
                    {tab.pinned ? null : (
                      <span className="min-w-0 flex-1 truncate">{tab.title}</span>
                    )}
                    {uploading ? (
                      <>
                        <span
                          className="size-1.5 shrink-0 animate-pulse rounded-full bg-accent"
                          aria-hidden="true"
                        />
                        <span className="sr-only">Upload active</span>
                      </>
                    ) : null}
                    {tab.pinned ? (
                      <PinIcon className="size-3.5 shrink-0 text-muted" />
                    ) : (
                      <button
                        type="button"
                        aria-label={`Close ${tab.title}`}
                        onClick={() => close(tab.id)}
                        className="rounded-md p-0.5 opacity-0 hover:bg-default/50 group-hover:opacity-100 focus:opacity-100"
                      >
                        <CloseIcon className="size-3.5" />
                      </button>
                    )}
                    <Dropdown>
                      <Button
                        isIconOnly
                        size="sm"
                        variant="ghost"
                        aria-label={`${tab.title} tab menu`}
                        className="-mr-2 size-7 min-w-7 opacity-0 group-hover:opacity-100 focus:opacity-100"
                      >
                        <MoreIcon className="size-3.5" />
                      </Button>
                      <Dropdown.Popover>
                        <Dropdown.Menu
                          aria-label="Tab actions"
                          onAction={(key) => {
                            if (key === "duplicate") duplicate(tab.id);
                            if (key === "pin") togglePinned(tab.id);
                            if (key === "close") close(tab.id);
                            if (key === "others") closeOthers(tab.id);
                            if (key === "right") closeRight(tab.id);
                            if (key === "reopen") reopenClosed();
                          }}
                        >
                          <Dropdown.Item id="duplicate" textValue="Duplicate tab">
                            <Label>Duplicate tab</Label>
                          </Dropdown.Item>
                          <Dropdown.Item id="pin" textValue={tab.pinned ? "Unpin tab" : "Pin tab"}>
                            <Label>{tab.pinned ? "Unpin tab" : "Pin tab"}</Label>
                          </Dropdown.Item>
                          {!tab.pinned ? (
                            <Dropdown.Item id="close" textValue="Close tab">
                              <Label>Close tab</Label>
                            </Dropdown.Item>
                          ) : null}
                          <Dropdown.Item id="others" textValue="Close other tabs">
                            <Label>Close other tabs</Label>
                          </Dropdown.Item>
                          <Dropdown.Item id="right" textValue="Close tabs to the right">
                            <Label>Close tabs to the right</Label>
                          </Dropdown.Item>
                          <Dropdown.Item
                            id="reopen"
                            textValue="Reopen closed tab"
                            isDisabled={closedTabs.length === 0}
                          >
                            <Label>Reopen closed tab</Label>
                          </Dropdown.Item>
                        </Dropdown.Menu>
                      </Dropdown.Popover>
                    </Dropdown>
                  </>
                )}
              </GridListItem>
            );
          }}
        </GridList>
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label="New file tab"
          className="mb-1 shrink-0"
          onPress={onNew}
        >
          <PlusIcon className="size-4" />
        </Button>
      </div>

      <div className="flex items-center gap-2 px-2 py-2 sm:hidden">
        <Dropdown>
          <Button variant="secondary" className="min-w-0 flex-1 justify-start">
            <FolderIcon className="size-4 shrink-0 text-accent" />
            <span className="truncate">
              {tabs.find((tab) => tab.id === activeTabId)?.title || "My files"}
            </span>
          </Button>
          <Dropdown.Popover className="min-w-64">
            <Dropdown.Menu aria-label="Open file tabs" onAction={(key) => onSwitch(String(key))}>
              {tabs.map((tab) => (
                <Dropdown.Item key={tab.id} id={tab.id} textValue={tab.title}>
                  <FolderIcon className="size-4 text-accent" />
                  <Label>{tab.title}</Label>
                </Dropdown.Item>
              ))}
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
        <Button isIconOnly variant="primary" aria-label="New file tab" onPress={onNew}>
          <PlusIcon className="size-4" />
        </Button>
      </div>
    </div>
  );
}

export function FileTabNavigation({
  onBack,
  onForward,
  onUp,
  canBack,
  canForward,
  canUp,
}: {
  onBack: () => void;
  onForward: () => void;
  onUp: () => void;
  canBack: boolean;
  canForward: boolean;
  canUp: boolean;
}) {
  return (
    <div className="flex shrink-0 items-center gap-0.5">
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label="Back"
        isDisabled={!canBack}
        onPress={onBack}
      >
        <ArrowBackIcon className="size-4" />
      </Button>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label="Forward"
        isDisabled={!canForward}
        onPress={onForward}
      >
        <ArrowForwardIcon className="size-4" />
      </Button>
      <Button
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label="Up one folder"
        isDisabled={!canUp}
        onPress={onUp}
      >
        <ArrowUpIcon className="size-4" />
      </Button>
    </div>
  );
}
