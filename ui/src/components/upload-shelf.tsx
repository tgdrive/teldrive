import { Button, Card, Chip, ProgressBar } from "@heroui/react";
import { type CSSProperties, useState } from "react";
import { Button as AriaButton, Tree, TreeItem, TreeItemContent } from "react-aria-components";
import { type UploadTask, useUploadStore } from "@/features/uploads/store";
import UploadIcon from "~icons/gravity-ui/arrow-up-from-line";
import ChevronDownIcon from "~icons/gravity-ui/chevron-down";
import ChevronRightIcon from "~icons/gravity-ui/chevron-right";
import ChevronUpIcon from "~icons/gravity-ui/chevron-up";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import PauseIcon from "~icons/gravity-ui/pause";
import PlayIcon from "~icons/gravity-ui/play";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import CloseIcon from "~icons/gravity-ui/xmark";

type UploadNode = {
  id: string;
  name: string;
  kind: "batch" | "folder" | "file";
  children: UploadNode[];
  tasks: UploadTask[];
  task?: UploadTask;
};

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const power = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** power).toFixed(power === 0 ? 0 : 1)} ${units[power]}`;
}

function buildTree(tasks: UploadTask[]) {
  const batches = new Map<string, UploadNode>();
  for (const task of tasks) {
    let batch = batches.get(task.batchId);
    if (!batch) {
      batch = {
        id: `batch:${task.batchId}`,
        name: task.batchName,
        kind: "batch",
        children: [],
        tasks: [],
      };
      batches.set(task.batchId, batch);
    }
    batch.tasks.push(task);
    const segments = task.relativePath.split("/").filter(Boolean);
    if (segments.length > 1 && segments[0] === task.batchName) segments.shift();
    let parent = batch;
    for (const segment of segments.slice(0, -1)) {
      let folder = parent.children.find(
        (child) => child.kind === "folder" && child.name === segment,
      );
      if (!folder) {
        folder = {
          id: `${parent.id}/folder:${segment}`,
          name: segment,
          kind: "folder",
          children: [],
          tasks: [],
        };
        parent.children.push(folder);
      }
      folder.tasks.push(task);
      parent = folder;
    }
    parent.children.push({
      id: `task:${task.id}`,
      name: task.name,
      kind: "file",
      children: [],
      tasks: [task],
      task,
    });
  }
  return [...batches.values()].reverse().flatMap((batch) => {
    const hasFolderHierarchy = batch.tasks.some((task) => task.relativePath.includes("/"));
    return hasFolderHierarchy ? [batch] : batch.children;
  });
}

function summarize(tasks: UploadTask[]) {
  const included = tasks.filter((task) => task.status !== "cancelled");
  const totalBytes = included.reduce((sum, task) => sum + task.size, 0);
  const uploadedBytes = included.reduce(
    (sum, task) => sum + Math.min(task.uploadedBytes, task.size),
    0,
  );
  const completed = included.filter((task) => task.status === "completed").length;
  const progress =
    totalBytes > 0
      ? Math.round((uploadedBytes / totalBytes) * 100)
      : included.length === 0 || completed === included.length
        ? 100
        : 0;
  return { totalBytes, uploadedBytes, completed, total: included.length, progress };
}

function TaskActions({ task }: { task: UploadTask }) {
  const pause = useUploadStore((state) => state.pause);
  const retry = useUploadStore((state) => state.retry);
  const cancel = useUploadStore((state) => state.cancel);
  const remove = useUploadStore((state) => state.remove);

  return (
    <div className="ml-auto flex shrink-0 items-center gap-0.5">
      {task.status === "running" && (
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={`Pause ${task.name}`}
          onPress={() => pause(task.id)}
        >
          <PauseIcon className="size-3.5" />
        </Button>
      )}
      {(task.status === "paused" || task.status === "failed") && (
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={`Resume ${task.name}`}
          onPress={() => retry(task.id)}
        >
          <PlayIcon className="size-3.5" />
        </Button>
      )}
      {!["completed", "cancelled"].includes(task.status) ? (
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={`Cancel ${task.name}`}
          onPress={() => void cancel(task.id)}
        >
          <CloseIcon className="size-3.5" />
        </Button>
      ) : (
        <Button
          isIconOnly
          size="sm"
          variant="ghost"
          aria-label={`Remove ${task.name}`}
          onPress={() => remove(task.id)}
        >
          <TrashIcon className="size-3.5" />
        </Button>
      )}
    </div>
  );
}

function UploadTreeItem({ node, root = false }: { node: UploadNode; root?: boolean }) {
  const summary = summarize(node.tasks);
  const hasChildren = node.children.length > 0;
  const detail =
    node.kind === "file"
      ? `${formatBytes(summary.uploadedBytes)} of ${formatBytes(summary.totalBytes)}`
      : `${summary.completed} of ${summary.total} files - ${formatBytes(summary.uploadedBytes)} of ${formatBytes(summary.totalBytes)}`;

  return (
    <TreeItem
      id={node.id}
      textValue={node.name}
      className="group rounded-lg outline-none data-[focus-visible]:ring-2 data-[focus-visible]:ring-accent"
    >
      <TreeItemContent>
        <div
          className="flex min-h-14 items-center gap-2 rounded-lg px-2 py-2 hover:bg-default/40 group-data-[expanded]:bg-default/20"
          style={
            {
              paddingInlineStart: "calc((var(--tree-item-level) - 1) * 1rem + .5rem)",
            } as CSSProperties
          }
        >
          {hasChildren ? (
            <AriaButton
              slot="chevron"
              className="flex size-7 items-center justify-center rounded-md text-muted outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <ChevronRightIcon className="size-3.5 transition-transform group-data-[expanded]:rotate-90" />
            </AriaButton>
          ) : !root ? (
            <span className="size-7" />
          ) : null}
          <span
            className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${node.kind === "file" ? "bg-accent/10 text-accent" : "bg-warning/10 text-warning"}`}
          >
            {node.kind === "file" ? (
              <FileIcon className="size-4" />
            ) : (
              <FolderIcon className="size-4" />
            )}
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2">
              <span className="truncate text-sm font-medium">{node.name}</span>
              {node.task ? (
                <Chip size="sm" variant="tertiary" className="capitalize">
                  {node.task.status.replace("_", " ")}
                </Chip>
              ) : null}
            </span>
            <span className="mt-0.5 block truncate text-[11px] text-muted">{detail}</span>
            <ProgressBar
              aria-label={`${node.name} upload progress`}
              value={summary.progress}
              className="mt-1.5 h-1"
            />
            {node.task?.error ? (
              <span className="mt-1 block text-xs text-danger">{node.task.error}</span>
            ) : null}
          </span>
          {node.task ? <TaskActions task={node.task} /> : null}
        </div>
      </TreeItemContent>
      {node.children.map((child) => (
        <UploadTreeItem key={child.id} node={child} />
      ))}
    </TreeItem>
  );
}

export function UploadShelf() {
  const tasks = useUploadStore((state) => state.tasks);
  const clearCompleted = useUploadStore((state) => state.clearCompleted);
  const [expanded, setExpanded] = useState(true);
  if (tasks.length === 0) return null;

  const tree = buildTree(tasks);
  const summary = summarize(tasks);
  const active = tasks.filter((task) => !["completed", "cancelled"].includes(task.status)).length;
  const failed = tasks.filter((task) => task.status === "failed").length;
  const expandedKeys = new Set(
    tree.flatMap((batch) => [
      batch.id,
      ...batch.children.filter((node) => node.kind === "folder").map((node) => node.id),
    ]),
  );

  return (
    <Card
      data-testid="upload-shelf"
      className="fixed inset-x-3 bottom-3 z-40 gap-0 border border-border bg-surface/95 shadow-2xl backdrop-blur-xl sm:inset-x-auto sm:bottom-4 sm:right-4 sm:w-[30rem]"
    >
      <Card.Header className="px-4 py-3">
        <div className="flex w-full items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent">
            <UploadIcon className="size-4" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold">Uploads</p>
            <p className="truncate text-[11px] text-muted">
              {active
                ? `${active} active - ${summary.progress}% - ${formatBytes(summary.uploadedBytes)} of ${formatBytes(summary.totalBytes)}`
                : `${summary.completed} files completed`}
              {failed ? ` - ${failed} failed` : ""}
            </p>
          </div>
          <Button size="sm" variant="ghost" onPress={clearCompleted}>
            Clear
          </Button>
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={expanded ? "Collapse uploads" : "Expand uploads"}
            onPress={() => setExpanded((value) => !value)}
          >
            {expanded ? (
              <ChevronDownIcon className="size-4" />
            ) : (
              <ChevronUpIcon className="size-4" />
            )}
          </Button>
        </div>
      </Card.Header>
      <ProgressBar aria-label="Overall upload progress" value={summary.progress} className="h-1" />
      {expanded ? (
        <Card.Content className="max-h-[min(65vh,34rem)] overflow-y-auto px-2 pb-2">
          <Tree
            aria-label="Upload queue"
            defaultExpandedKeys={expandedKeys}
            className="outline-none"
          >
            {tree.map((node) => (
              <UploadTreeItem key={node.id} node={node} root />
            ))}
          </Tree>
        </Card.Content>
      ) : null}
    </Card>
  );
}
