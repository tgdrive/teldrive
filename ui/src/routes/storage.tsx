import { Card, Chip, Spinner, Typography } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import ChannelIcon from "~icons/gravity-ui/database";
import CleanupIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { queryClient } from "@/api/query-client";
import type { components } from "@/api/schema";
import { LinkButton } from "@/components/link-button";
import { Page, PageHeader } from "@/components/page";

type StorageActivity = components["schemas"]["StorageActivity"];
type StorageGrowthPoint = components["schemas"]["StorageGrowthPoint"];

const CATEGORY_LABELS: Record<string, string> = {
  archive: "Archives",
  audio: "Audio",
  document: "Documents",
  image: "Images",
  video: "Video",
  other: "Other",
};

const ACTIVITY_LABELS: Record<string, string> = {
  "file.created": "File added",
  "file.trashed": "File moved to Trash",
  "file.restored": "File restored",
  "file.purged": "File permanently deleted",
  "upload.completed": "Upload completed",
  "upload.aborted": "Upload aborted",
  "upload.expired": "Upload expired",
  "share.created": "Share created",
  "share.deleted": "Share removed",
  "channel.created": "Storage channel added",
  "channel.updated": "Storage channel updated",
  "channel.deleted": "Storage channel removed",
};

export const Route = createFileRoute("/storage")({
  component: StoragePage,
  pendingComponent: () => (
    <div className="flex items-center justify-center py-20">
      <Spinner size="lg" />
    </div>
  ),
  loader: () => queryClient.ensureQueryData($api.queryOptions("get", "/v1/storage/stats")),
});

function StoragePage() {
  const { data } = $api.useSuspenseQuery("get", "/v1/storage/stats");
  const summary = data.summary;
  const configuredChannels = data.channels.length;
  const activeChannels = data.channels.filter((channel) => channel.health === "healthy").length;
  const totalChannelParts = data.channels.reduce((total, channel) => total + channel.partCount, 0);

  return (
    <Page>
      <PageHeader
        title="Storage"
        description="Telegram-backed storage usage, growth, distribution, cleanup, and recent activity."
      />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Total stored"
          value={formatBytes(summary.logicalBytes)}
          detail={`${formatBytes(data.growth.at(-1)?.addedBytes ?? 0)} added today`}
        />
        <StatCard
          label="Active files"
          value={summary.activeFiles.toLocaleString()}
          detail={`${summary.activeFolders.toLocaleString()} folders`}
        />
        <StatCard
          label="Trash"
          value={formatBytes(summary.trashBytes)}
          detail={`${summary.trashedFiles.toLocaleString()} files`}
        />
        <StatCard
          label="Channels"
          value={configuredChannels.toLocaleString()}
          detail={`${activeChannels.toLocaleString()} healthy`}
        />
        <StatCard
          label="Reclaimable"
          value={formatBytes(data.cleanup.totalReclaimableBytes)}
          detail={`${data.cleanup.staleUploads.toLocaleString()} stale uploads`}
        />
      </div>

      <Card className="gap-5 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              Storage growth
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              Logical bytes stored over the last 30 days.
            </Typography.Paragraph>
          </div>
          <Chip variant="tertiary">30 days</Chip>
        </div>
        <StorageGrowthChart points={data.growth} />
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card className="gap-5 p-5">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              Storage composition
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              Active file bytes grouped by content category.
            </Typography.Paragraph>
          </div>
          <div className="grid gap-4">
            {data.categories.map((category) => {
              const percent =
                summary.logicalBytes > 0 ? (category.totalSize / summary.logicalBytes) * 100 : 0;
              return (
                <div key={category.category} className="grid gap-2">
                  <div className="flex items-center justify-between gap-4 text-sm">
                    <span className="font-medium">
                      {CATEGORY_LABELS[category.category] ?? category.category}
                    </span>
                    <span className="text-muted">
                      {formatBytes(category.totalSize)} · {category.totalFiles.toLocaleString()}{" "}
                      files
                    </span>
                  </div>
                  <ProgressTrack value={percent} label={`${category.category} storage`} />
                </div>
              );
            })}
            {data.categories.length === 0 && <EmptyCopy>No active files are stored yet.</EmptyCopy>}
          </div>
        </Card>

        <Card className="gap-5 p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Typography type="h2" className="text-base font-semibold">
                Telegram channel distribution
              </Typography>
              <Typography.Paragraph className="text-sm text-muted">
                Stored Telegram parts across configured channels.
              </Typography.Paragraph>
            </div>
            <LinkButton to="/settings/channels" size="sm" variant="tertiary">
              Manage channels
            </LinkButton>
          </div>
          <div className="divide-y divide-border rounded-xl border border-border">
            {data.channels.map((channel) => {
              const percent =
                totalChannelParts > 0 ? (channel.partCount / totalChannelParts) * 100 : 0;
              return (
                <div key={channel.channelId} className="grid gap-3 p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <ChannelIcon className="size-4 shrink-0 text-muted" />
                        <span className="truncate text-sm font-semibold">{channel.name}</span>
                        {channel.selected && (
                          <Chip size="sm" variant="tertiary">
                            Selected
                          </Chip>
                        )}
                      </div>
                      <div className="mt-1 text-xs text-muted">
                        {channel.partCount.toLocaleString()} parts · {percent.toFixed(1)}%
                      </div>
                    </div>
                    <Chip
                      size="sm"
                      color={channel.health === "healthy" ? "success" : "warning"}
                      variant="tertiary"
                    >
                      {channel.health}
                    </Chip>
                  </div>
                  <ProgressTrack value={percent} label={`${channel.name} storage distribution`} />
                </div>
              );
            })}
            {data.channels.length === 0 && (
              <EmptyCopy>No storage channels are configured.</EmptyCopy>
            )}
          </div>
        </Card>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card className="gap-5 p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Typography type="h2" className="text-base font-semibold">
                Cleanup opportunities
              </Typography>
              <Typography.Paragraph className="text-sm text-muted">
                Storage that can be reviewed for permanent cleanup.
              </Typography.Paragraph>
            </div>
            <CleanupIcon className="size-5 text-muted" />
          </div>
          <div className="divide-y divide-border rounded-xl border border-border">
            <MetricRow label="Trash" value={formatBytes(data.cleanup.trashBytes)} />
            <MetricRow
              label="Stale multipart uploads"
              value={formatBytes(data.cleanup.staleUploadBytes)}
              detail={`${data.cleanup.staleUploads.toLocaleString()} sessions`}
            />
            <MetricRow
              label="Total reclaimable"
              value={formatBytes(data.cleanup.totalReclaimableBytes)}
              strong
            />
          </div>
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs text-muted">
              Nothing is deleted automatically from this dashboard.
            </p>
            <LinkButton to="/trash" size="sm" variant="primary">
              Review cleanup
            </LinkButton>
          </div>
        </Card>

        <Card className="gap-5 p-5">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              Recent storage activity
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              Durable file, upload, share, and channel events.
            </Typography.Paragraph>
          </div>
          <div className="divide-y divide-border rounded-xl border border-border">
            {data.activity.map((activity) => (
              <ActivityRow key={activity.id} activity={activity} />
            ))}
            {data.activity.length === 0 && <EmptyCopy>No recent storage activity.</EmptyCopy>}
          </div>
        </Card>
      </div>
    </Page>
  );
}

function ProgressTrack({ value, label }: { value: number; label: string }) {
  const width = `${Math.max(0, Math.min(100, value))}%`;
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(value)}
      className="h-2 overflow-hidden rounded-full bg-default/40"
    >
      <div className="h-full rounded-full bg-accent transition-[width]" style={{ width }} />
    </div>
  );
}

function StatCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <Card className="gap-1 p-4">
      <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-muted">
        {label}
      </div>
      <div className="text-2xl font-semibold tracking-tight">{value}</div>
      <div className="text-xs text-muted">{detail}</div>
    </Card>
  );
}

function MetricRow({
  label,
  value,
  detail,
  strong,
}: {
  label: string;
  value: string;
  detail?: string;
  strong?: boolean;
}) {
  return (
    <div className="flex items-center justify-between gap-4 px-4 py-3">
      <div>
        <div className={strong ? "text-sm font-semibold" : "text-sm"}>{label}</div>
        {detail && <div className="text-xs text-muted">{detail}</div>}
      </div>
      <div className={strong ? "font-mono text-sm font-semibold" : "font-mono text-sm"}>
        {value}
      </div>
    </div>
  );
}

function ActivityRow({ activity }: { activity: StorageActivity }) {
  return (
    <div className="flex items-start justify-between gap-4 px-4 py-3">
      <div className="min-w-0">
        <div className="text-sm font-medium">{ACTIVITY_LABELS[activity.type] ?? activity.type}</div>
        <div className="truncate text-xs text-muted">{activity.label}</div>
      </div>
      <time className="shrink-0 text-xs text-muted" dateTime={activity.occurredAt}>
        {formatRelative(activity.occurredAt)}
      </time>
    </div>
  );
}

function StorageGrowthChart({ points }: { points: StorageGrowthPoint[] }) {
  if (points.length === 0) return <EmptyCopy>No storage history is available.</EmptyCopy>;
  const width = 960;
  const height = 220;
  const padding = 18;
  const values = points.map((point) => point.logicalBytes);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = Math.max(1, max - min);
  const coordinates = points.map((point, index) => {
    const x = padding + (index / Math.max(1, points.length - 1)) * (width - padding * 2);
    const y = height - padding - ((point.logicalBytes - min) / range) * (height - padding * 2);
    return [x, y] as const;
  });
  const path = coordinates
    .map(([x, y], index) => `${index === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`)
    .join(" ");

  return (
    <div className="grid gap-3">
      <div className="overflow-hidden rounded-xl border border-border bg-surface-secondary/40 p-3">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label="Logical storage growth over 30 days"
          className="h-56 w-full"
        >
          <title>Logical storage growth over 30 days</title>
          {[0.25, 0.5, 0.75].map((ratio) => (
            <line
              key={ratio}
              x1={padding}
              x2={width - padding}
              y1={height * ratio}
              y2={height * ratio}
              className="stroke-border"
              strokeWidth="1"
            />
          ))}
          <path
            d={path}
            fill="none"
            className="stroke-accent"
            strokeWidth="3"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
          {coordinates.at(-1) && (
            <circle
              cx={coordinates.at(-1)?.[0]}
              cy={coordinates.at(-1)?.[1]}
              r="5"
              className="fill-accent"
            />
          )}
        </svg>
      </div>
      <div className="flex items-center justify-between text-xs text-muted">
        <span>{new Date(points[0].day).toLocaleDateString()}</span>
        <span>{formatBytes(points.at(-1)?.logicalBytes ?? 0)}</span>
        <span>{new Date(points.at(-1)?.day ?? points[0].day).toLocaleDateString()}</span>
      </div>
    </div>
  );
}

function EmptyCopy({ children }: { children: React.ReactNode }) {
  return <div className="px-4 py-8 text-center text-sm text-muted">{children}</div>;
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const power = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** power;
  return `${value.toFixed(value >= 100 || power === 0 ? 0 : value >= 10 ? 1 : 2)} ${units[power]}`;
}

function formatRelative(value: string) {
  const delta = Math.max(0, Date.now() - new Date(value).getTime());
  const minutes = Math.floor(delta / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return days < 7 ? `${days}d ago` : new Date(value).toLocaleDateString();
}
