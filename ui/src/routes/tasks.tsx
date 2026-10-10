import { Button, Card, Chip, Input, ListBox, Popover, Select, Spinner } from "@heroui/react";
import { createFileRoute, Link, stripSearchParams } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import PrevIcon from "~icons/gravity-ui/chevron-left";
import NextIcon from "~icons/gravity-ui/chevron-right";
import FilterIcon from "~icons/gravity-ui/funnel";
import AddIcon from "~icons/gravity-ui/plus";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { ConfirmDialog } from "../components/dialogs/confirm-dialog";
import { EmptyState, Page, PageHeader, PageToolbar } from "../components/page";
import { TaskLauncher } from "../components/task-launcher";
import { TaskStatusChip } from "../components/task-status-chip";
import type { components } from "@/api/schema";
import { $api as api, fetchClient } from "@/api/client";
import { queryClient } from "@/api/query-client";
import { invalidateTaskQueries } from "@/api/tasks";

type TaskOut = components["schemas"]["Job"];
type TaskCounts = components["schemas"]["JobStatistics"];
type TaskQueueOut = components["schemas"]["JobQueue"];
type TaskSearch = {
  status: string;
  query: string;
};

const PAGE_SIZE = 20;
const CANCELLABLE_STATUSES = ["pending", "scheduled", "available", "running", "retryable"];

const STATUS_TABS = [
  { key: "pending", label: "Pending" },
  { key: "scheduled", label: "Scheduled" },
  { key: "available", label: "Available" },
  { key: "running", label: "Running" },
  { key: "retryable", label: "Retryable" },
  { key: "cancelled", label: "Cancelled" },
  { key: "discarded", label: "Discarded" },
  { key: "completed", label: "Completed" },
] as const;

export const Route = createFileRoute("/tasks")({
  validateSearch: (search: Record<string, unknown>): TaskSearch => ({
    status:
      typeof search.status === "string" &&
      [
        "pending",
        "scheduled",
        "available",
        "running",
        "retryable",
        "cancelled",
        "discarded",
        "completed",
      ].includes(search.status)
        ? search.status
        : "running",
    query: typeof search.query === "string" ? search.query : "",
  }),
  search: {
    middlewares: [
      stripSearchParams({
        status: "running" as components["schemas"]["JobState"],
        query: "",
      }),
    ],
  },
  component: TasksPage,
  pendingComponent: () => (
    <div className="flex items-center justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
  loader: async () => {
    const qc = queryClient;
    await Promise.all([
      qc.ensureQueryData(
        api.queryOptions("get", "/v1/jobs", {
          params: {
            query: { limit: PAGE_SIZE, status: "running" as components["schemas"]["JobState"] },
          },
        }),
      ),
      qc.ensureQueryData(api.queryOptions("get", "/v1/jobs/statistics")),
      qc.ensureQueryData(api.queryOptions("get", "/v1/jobs/queues")),
    ]);
  },
});

function TasksPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const qc = queryClient;
  const [cursor, setCursor] = useState<string | undefined>();
  const [cursorHistory, setCursorHistory] = useState<(string | undefined)[]>([]);
  const { data } = api.useSuspenseQuery("get", "/v1/jobs", {
    params: {
      query: {
        cursor,
        limit: PAGE_SIZE,
        status: search.status as components["schemas"]["JobState"],
      },
    },
  });
  const _statesQuery = api.queryOptions("get", "/v1/jobs/statistics");
  const { data: statesData } = api.useSuspenseQuery("get", "/v1/jobs/statistics");
  const _queuesQuery = api.queryOptions("get", "/v1/jobs/queues");
  const { data: queuesData } = api.useSuspenseQuery("get", "/v1/jobs/queues");

  const [composerOpen, setComposerOpen] = useState(false);
  const [retryingId, setRetryingId] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [cleanupStatus, setCleanupStatus] = useState<
    "completed" | "retryable" | "discarded" | null
  >(null);
  const [cleaning, setCleaning] = useState(false);

  const tasks = data.tasks ?? [];
  const meta = data.meta;
  const taskStats = statesData;
  const taskQueues = queuesData.queues ?? [];
  const hasActiveTasks =
    (taskStats?.scheduled ?? 0) +
      (taskStats?.available ?? 0) +
      (taskStats?.running ?? 0) +
      (taskStats?.retryable ?? 0) >
    0;

  useEffect(() => {
    const interval = window.setInterval(
      () => {
        qc.invalidateQueries({
          queryKey: [
            "get",
            "/v1/jobs",
            {
              params: {
                query: {
                  cursor,
                  limit: PAGE_SIZE,
                  status: search.status as components["schemas"]["JobState"],
                },
              },
            },
          ],
        });
        qc.invalidateQueries({ queryKey: ["get", "/v1/jobs/statistics"] });
        qc.invalidateQueries({ queryKey: ["get", "/v1/jobs/queues"] });
      },
      hasActiveTasks ? 2_000 : 10_000,
    );
    return () => window.clearInterval(interval);
  }, [cursor, hasActiveTasks, qc, search.status]);

  const normalizedQuery = search.query.trim().toLowerCase();
  const filteredTasks = tasks.filter((task) => {
    const statusMatches = task.status === search.status;
    const textMatches =
      !normalizedQuery ||
      [
        task.id,
        task.type,
        task.queue,
        task.description,
        task.message,
        task.output ? JSON.stringify(task.output) : undefined,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(normalizedQuery));
    return statusMatches && textMatches;
  });
  const setSearch = async (next: Partial<TaskSearch>) => {
    const nextSearch = { ...search, ...next };

    if (next.status && next.status !== search.status) {
      await qc.ensureQueryData(
        api.queryOptions("get", "/v1/jobs", {
          params: {
            query: {
              limit: PAGE_SIZE,
              status: nextSearch.status as components["schemas"]["JobState"],
            },
          },
        }),
      );
    }

    setCursor(undefined);
    setCursorHistory([]);
    await navigate({ search: nextSearch, replace: true });
  };

  const goNext = () => {
    if (!meta?.nextCursor) return;
    setCursorHistory((history) => [...history, cursor]);
    setCursor(meta.nextCursor);
  };

  const goPrevious = () => {
    setCursorHistory((history) => {
      const previous = history.at(-1);
      setCursor(previous);
      return history.slice(0, -1);
    });
  };

  const refreshTasks = () => {
    void invalidateTaskQueries(qc);
  };

  const retryTask = async (task: TaskOut) => {
    setRetryingId(task.id);
    const { error } = await fetchClient.POST("/v1/jobs/{jobId}/retry", {
      params: { path: { jobId: task.id } },
    });
    setRetryingId(null);
    if (error) {
      toast.error("Failed to retry task");
      return;
    }
    toast.success("Task queued for retry");
    refreshTasks();
  };

  const deleteTask = async (task: TaskOut) => {
    setDeletingId(task.id);
    const { error } = CANCELLABLE_STATUSES.includes(task.status)
      ? await fetchClient.POST("/v1/jobs/{jobId}/cancel", {
          params: { path: { jobId: task.id } },
        })
      : await fetchClient.DELETE("/v1/jobs/{jobId}", {
          params: { path: { jobId: task.id } },
        });
    setDeletingId(null);
    if (error) {
      toast.error(
        CANCELLABLE_STATUSES.includes(task.status)
          ? "Failed to cancel task"
          : "Failed to remove task",
      );
      return;
    }
    toast.success(
      CANCELLABLE_STATUSES.includes(task.status) ? "Task cancellation requested" : "Task removed",
    );
    refreshTasks();
  };

  const cleanTasks = async () => {
    if (!cleanupStatus) return;
    setCleaning(true);
    const { data: response, error } = await fetchClient.DELETE("/v1/jobs/purge", {
      params: { query: { status: cleanupStatus } },
    });
    setCleaning(false);
    if (error) {
      toast.error(`Failed to clean ${cleanupStatus} tasks`);
      return;
    }

    const status = cleanupStatus;
    setCleanupStatus(null);
    setCursor(undefined);
    setCursorHistory([]);
    toast.success(`Removed ${response.count} ${status} task${response.count === 1 ? "" : "s"}`);
    refreshTasks();
  };

  return (
    <Page>
      <PageHeader
        title="Tasks"
        description="Create, monitor, retry, and inspect background work."
        actions={
          <Button
            size="sm"
            variant="primary"
            className="bg-accent text-accent-foreground"
            onPress={() => setComposerOpen(true)}
          >
            <AddIcon className="size-3.5" /> New Task
          </Button>
        }
      />

      {composerOpen && (
        <TaskLauncher
          onClose={() => setComposerOpen(false)}
          onQueued={() => {
            refreshTasks();
            setComposerOpen(false);
          }}
        />
      )}

      <PageToolbar className="items-stretch">
        <div className="flex min-w-0 flex-1 flex-col gap-2 md:flex-row md:items-center">
          <Input
            aria-label="Search tasks"
            placeholder="Search tasks by type, queue, path, or ID"
            value={search.query}
            onChange={(event) => setSearch({ query: event.currentTarget.value })}
            className="min-w-0 flex-1"
          />
          <div className="flex flex-wrap items-center gap-2">
            <TaskStatusSelect
              value={search.status}
              counts={taskStats}
              onChange={(status) => setSearch({ status })}
            />
            <QueueManager queues={taskQueues} onChanged={refreshTasks} />
            <div className="w-[4.75rem] shrink-0">
              {(() => {
                const purgeStatus = ["completed", "retryable", "discarded"].includes(search.status)
                  ? (search.status as "completed" | "retryable" | "discarded")
                  : null;
                return purgeStatus ? (
                  <Button
                    size="sm"
                    variant="danger-soft"
                    className="w-full"
                    isDisabled={(taskStats?.[purgeStatus] ?? 0) === 0}
                    onPress={() => setCleanupStatus(purgeStatus)}
                  >
                    <TrashIcon className="size-3.5" /> Clean
                  </Button>
                ) : null;
              })()}
            </div>
          </div>
        </div>
      </PageToolbar>

      <Card className="overflow-hidden p-0">
        <div className="flex items-center justify-between border-border border-b px-4 py-3">
          <div className="text-sm font-semibold">Task activity</div>
          {hasActiveTasks && (
            <span className="flex items-center gap-2 text-xs text-muted">
              <span className="size-2 animate-pulse rounded-full bg-accent" />
              Live updates
            </span>
          )}
        </div>
        {filteredTasks.length > 0 ? (
          <ListBox
            aria-label="Tasks"
            selectionMode="none"
            className="w-full min-w-0 divide-y divide-border overflow-hidden p-0"
          >
            {filteredTasks.map((task) => (
              <TaskRow
                key={task.id}
                task={task}
                onRetry={() => retryTask(task)}
                onDelete={() => deleteTask(task)}
                retryPending={retryingId === task.id}
                deletePending={deletingId === task.id}
              />
            ))}
          </ListBox>
        ) : (
          <EmptyState
            title="No tasks match these filters"
            description="Adjust the active filters or queue a new task."
            action={
              <Button size="sm" variant="primary" onPress={() => setComposerOpen(true)}>
                New Task
              </Button>
            }
          />
        )}
      </Card>

      {(cursorHistory.length > 0 || Boolean(meta?.nextCursor)) && (
        <div className="flex items-center justify-end gap-2">
          <Button
            size="sm"
            variant="tertiary"
            isDisabled={cursorHistory.length === 0}
            onPress={goPrevious}
          >
            <PrevIcon className="size-3.5" /> Previous
          </Button>
          <span className="min-w-20 text-center text-xs text-muted">
            Page {cursorHistory.length + 1}
          </span>
          <Button size="sm" variant="tertiary" isDisabled={!meta?.nextCursor} onPress={goNext}>
            Next <NextIcon className="size-3.5" />
          </Button>
        </div>
      )}

      <ConfirmDialog
        open={cleanupStatus != null}
        onOpenChange={(open) => !open && setCleanupStatus(null)}
        onConfirm={cleanTasks}
        title={`Clean ${cleanupStatus ?? ""} tasks?`}
        message={`This permanently removes all ${cleanupStatus ? (taskStats?.[cleanupStatus] ?? 0) : 0} ${cleanupStatus ?? ""} task records. Other task statuses are not affected.`}
        confirmLabel="Clean"
        isPending={cleaning}
      />
    </Page>
  );
}

function TaskRow({
  task,
  onRetry,
  onDelete,
  retryPending,
  deletePending,
}: {
  task: TaskOut;
  onRetry: () => void;
  onDelete: () => void;
  retryPending: boolean;
  deletePending: boolean;
}) {
  const title = `${task.type} #${task.id}`;
  const canRetry = ["cancelled", "discarded", "retryable"].includes(task.status);

  return (
    <ListBox.Item
      id={task.id}
      textValue={`${title} ${task.type} ${task.status}`}
      className="group block w-full min-w-0 max-w-full overflow-hidden transform-none! active:transform-none! data-pressed:transform-none!"
    >
      <div className="relative grid min-w-0 gap-3 px-3 py-3 sm:px-4 sm:py-4 lg:grid-cols-[minmax(0,1fr)_6rem_auto] lg:items-center lg:gap-4">
        <div className="min-w-0 pr-11 lg:pr-0">
          <div className="flex min-w-0 flex-wrap items-center gap-1.5 sm:gap-2">
            <TaskStatusChip status={task.status} />
            {task.parentId ? (
              <Chip size="sm" variant="tertiary">
                Child task
              </Chip>
            ) : null}
          </div>

          <Link
            to="/tasks/$id"
            params={{ id: task.id }}
            className="mt-1.5 block truncate text-sm font-semibold text-foreground hover:text-accent hover:underline sm:mt-2"
          >
            {title}
          </Link>

          <div className="mt-1.5 flex min-w-0 items-center gap-2 overflow-hidden text-[11px] text-muted sm:flex-wrap sm:gap-x-3 sm:gap-y-1">
            <span className="shrink-0">{task.queue || "default"} queue</span>
            <span className="shrink-0">{taskDuration(task)}</span>
            <span className="truncate" title={formatDate(task.createdAt)}>
              {formatRelativeDate(task.createdAt)}
            </span>
            {(task.attempt ?? 0) > 0 ? (
              <span className="hidden shrink-0 sm:inline lg:hidden">
                Attempt {task.attempt} of {task.maxAttempts || "—"}
              </span>
            ) : null}
          </div>
        </div>

        <div className="hidden lg:block">
          <div className="text-[10px] font-medium uppercase tracking-wide text-muted">Attempts</div>
          <div className="mt-1 text-xs font-semibold tabular-nums">
            {task.attempt ?? 0} / {task.maxAttempts || "—"}
          </div>
        </div>

        <div className="absolute top-3 right-3 flex items-center justify-end gap-1.5 lg:static">
          {canRetry ? (
            <Button
              size="sm"
              variant="primary"
              isPending={retryPending}
              onPress={onRetry}
              aria-label={`Retry ${title}`}
            >
              Retry
            </Button>
          ) : null}
          {task.status === "running" ? (
            <Button
              size="sm"
              variant="danger-soft"
              isPending={deletePending}
              onPress={onDelete}
              aria-label={`Cancel ${title}`}
            >
              Cancel
            </Button>
          ) : (
            <Button
              size="sm"
              variant="tertiary"
              isIconOnly
              isPending={deletePending}
              onPress={onDelete}
              aria-label={`Delete ${title}`}
            >
              <TrashIcon className="size-3.5" />
            </Button>
          )}
        </div>
      </div>
    </ListBox.Item>
  );
}

function TaskStatusSelect({
  value,
  counts,
  onChange,
}: {
  value: string;
  counts?: TaskCounts;
  onChange: (value: string) => void;
}) {
  const selected = STATUS_TABS.find((item) => item.key === value) ?? STATUS_TABS[0];

  return (
    <Select
      aria-label="Task status"
      selectedKey={value}
      onSelectionChange={(key) => onChange(String(key))}
      className="w-44 shrink-0"
    >
      <Select.Trigger className="h-8 min-h-8 py-1.5">
        <Select.Value>
          <div className="flex min-w-0 items-center justify-between gap-3">
            <span className="truncate">{selected.label}</span>
            <span className="text-xs tabular-nums text-muted">{counts?.[selected.key] ?? 0}</span>
          </div>
        </Select.Value>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {STATUS_TABS.map((item) => (
            <ListBox.Item key={item.key} id={item.key} textValue={item.label}>
              <div className="flex w-full items-center justify-between gap-4">
                <TaskStatusChip status={item.key} />
                <span className="text-xs font-medium tabular-nums text-muted">
                  {counts?.[item.key] ?? 0}
                </span>
              </div>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}

function QueueManager({ queues, onChanged }: { queues: TaskQueueOut[]; onChanged: () => void }) {
  const [pendingQueue, setPendingQueue] = useState<string | null>(null);

  const toggleQueue = async (queue: TaskQueueOut) => {
    setPendingQueue(queue.name);
    const endpoint = queue.paused
      ? "/v1/jobs/queues/{queue}/resume"
      : "/v1/jobs/queues/{queue}/pause";
    const { error } = await fetchClient.POST(endpoint, { params: { path: { queue: queue.name } } });
    setPendingQueue(null);
    if (error) {
      toast.error(`Failed to ${queue.paused ? "resume" : "pause"} ${queue.name}`);
      return;
    }
    toast.success(`${queue.name} ${queue.paused ? "resumed" : "paused"}`);
    onChanged();
  };

  return (
    <Popover>
      <Button size="sm" variant="tertiary">
        <FilterIcon className="size-3.5" /> Queues
      </Button>
      <Popover.Content placement="bottom end" offset={8} className="w-[min(92vw,28rem)]">
        <Popover.Dialog className="p-0">
          <div className="border-border border-b px-4 py-3">
            <Popover.Heading className="text-sm font-semibold">Worker queues</Popover.Heading>
            <p className="mt-0.5 text-xs text-muted">
              Pause or resume River queues without stopping workers.
            </p>
          </div>
          <div className="max-h-80 divide-y divide-border overflow-y-auto">
            {queues.length > 0 ? (
              queues.map((queue) => (
                <div key={queue.name} className="flex items-center gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{queue.name}</span>
                      <span
                        className={`size-2 rounded-full ${queue.paused ? "bg-warning" : "bg-success"}`}
                      />
                      <span className="text-[10px] text-muted">
                        {queue.paused ? "Paused" : "Active"}
                      </span>
                    </div>
                    <div className="mt-1 flex gap-3 text-[11px] text-muted">
                      <span>{queue.available} available</span>
                      <span>{queue.running} running</span>
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant={queue.paused ? "primary" : "tertiary"}
                    isPending={pendingQueue === queue.name}
                    onPress={() => toggleQueue(queue)}
                  >
                    {queue.paused ? "Resume" : "Pause"}
                  </Button>
                </div>
              ))
            ) : (
              <div className="px-4 py-8 text-center text-sm text-muted">
                No active River queues.
              </div>
            )}
          </div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  );
}

function formatDate(value: string) {
  return new Date(value).toLocaleString();
}
function formatRelativeDate(value: string) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return days < 7 ? `${days}d ago` : new Date(value).toLocaleDateString();
}

function taskDuration(task: TaskOut) {
  const start = task.startedAt
    ? new Date(task.startedAt).getTime()
    : new Date(task.createdAt).getTime();
  const end = task.completedAt ? new Date(task.completedAt).getTime() : Date.now();
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}
