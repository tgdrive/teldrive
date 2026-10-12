import { Card, Chip, Spinner, Typography } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import ChannelIcon from "~icons/gravity-ui/database";
import CleanupIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { queryClient } from "@/api/query-client";
import type { components } from "@/api/schema";
import { LinkButton } from "@/components/link-button";
import { Page, PageHeader } from "@/components/page";
import { useAppLocale } from "@/lib/locale";

type StorageActivity = components["schemas"]["StorageActivity"];
type StorageGrowthPoint = components["schemas"]["StorageGrowthPoint"];

type TFunction = (key: string, vars?: Record<string, string | number>) => string;

function categoryLabel(t: TFunction, category: string) {
  switch (category) {
    case "archive":
      return t("storage.cat.archive");
    case "audio":
      return t("storage.cat.audio");
    case "document":
      return t("storage.cat.document");
    case "image":
      return t("storage.cat.image");
    case "video":
      return t("storage.cat.video");
    case "other":
      return t("storage.cat.other");
    default:
      return category;
  }
}

function activityLabel(t: TFunction, type: string) {
  switch (type) {
    case "file.created":
      return t("storage.act.file.created");
    case "file.trashed":
      return t("storage.act.file.trashed");
    case "file.restored":
      return t("storage.act.file.restored");
    case "file.purged":
      return t("storage.act.file.purged");
    case "upload.completed":
      return t("storage.act.upload.completed");
    case "upload.aborted":
      return t("storage.act.upload.aborted");
    case "upload.expired":
      return t("storage.act.upload.expired");
    case "share.created":
      return t("storage.act.share.created");
    case "share.deleted":
      return t("storage.act.share.deleted");
    case "channel.created":
      return t("storage.act.channel.created");
    case "channel.updated":
      return t("storage.act.channel.updated");
    case "channel.deleted":
      return t("storage.act.channel.deleted");
    default:
      return type;
  }
}

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
  const { t } = useAppLocale();
  const summary = data.summary;
  const configuredChannels = data.channels.length;
  const selectedChannels = data.channels.filter((channel) => channel.selected).length;
  const totalChannelParts = data.channels.reduce((total, channel) => total + channel.partCount, 0);

  return (
    <Page>
      <PageHeader title={t("storage.title")} description={t("storage.desc")} />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={t("storage.total")}
          value={formatBytes(summary.logicalBytes)}
          detail={t("storage.addedToday", {
            bytes: formatBytes(data.growth.at(-1)?.addedBytes ?? 0),
          })}
        />
        <StatCard
          label={t("storage.active")}
          value={summary.activeFiles.toLocaleString()}
          detail={t("storage.folders", { count: summary.activeFolders.toLocaleString() })}
        />
        <StatCard
          label={t("storage.trash")}
          value={formatBytes(summary.trashBytes)}
          detail={t("storage.trashedFiles", { count: summary.trashedFiles.toLocaleString() })}
        />
        <StatCard
          label={t("storage.channels")}
          value={configuredChannels.toLocaleString()}
          detail={t("storage.selectedCount", { count: selectedChannels.toLocaleString() })}
        />
        <StatCard
          label={t("storage.reclaimable")}
          value={formatBytes(data.cleanup.totalReclaimableBytes)}
          detail={t("storage.staleUploads", { count: data.cleanup.staleUploads.toLocaleString() })}
        />
      </div>

      <Card className="gap-5 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              {t("storage.growth")}
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              {t("storage.growthDesc")}
            </Typography.Paragraph>
          </div>
          <Chip variant="tertiary">{t("storage.days")}</Chip>
        </div>
        <StorageGrowthChart points={data.growth} />
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card className="gap-5 p-5">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              {t("storage.composition")}
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              {t("storage.compositionDesc")}
            </Typography.Paragraph>
          </div>
          <div className="grid gap-4">
            {data.categories.map((category) => {
              const percent =
                summary.logicalBytes > 0 ? (category.totalSize / summary.logicalBytes) * 100 : 0;
              return (
                <div key={category.category} className="grid gap-2">
                  <div className="flex items-center justify-between gap-4 text-sm">
                    <span className="font-medium">{categoryLabel(t, category.category)}</span>
                    <span className="text-muted">
                      {t("storage.filesLine", {
                        bytes: formatBytes(category.totalSize),
                        count: category.totalFiles.toLocaleString(),
                      })}
                    </span>
                  </div>
                  <ProgressTrack value={percent} label={`${category.category} storage`} />
                </div>
              );
            })}
            {data.categories.length === 0 && <EmptyCopy>{t("storage.emptyCats")}</EmptyCopy>}
          </div>
        </Card>

        <Card className="gap-5 p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Typography type="h2" className="text-base font-semibold">
                {t("storage.distribution")}
              </Typography>
              <Typography.Paragraph className="text-sm text-muted">
                {t("storage.distributionDesc")}
              </Typography.Paragraph>
            </div>
            <LinkButton to="/settings/channels" size="sm" variant="tertiary">
              {t("storage.manage")}
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
                            {t("storage.selected")}
                          </Chip>
                        )}
                      </div>
                      <div className="mt-1 text-xs text-muted">
                        {t("storage.parts", {
                          count: channel.partCount.toLocaleString(),
                          pct: percent.toFixed(1),
                        })}
                      </div>
                    </div>
                  </div>
                  <ProgressTrack value={percent} label={`${channel.name} storage distribution`} />
                </div>
              );
            })}
            {data.channels.length === 0 && <EmptyCopy>{t("storage.emptyChannels")}</EmptyCopy>}
          </div>
        </Card>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card className="gap-5 p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Typography type="h2" className="text-base font-semibold">
                {t("storage.cleanup")}
              </Typography>
              <Typography.Paragraph className="text-sm text-muted">
                {t("storage.cleanupDesc")}
              </Typography.Paragraph>
            </div>
            <CleanupIcon className="size-5 text-muted" />
          </div>
          <div className="divide-y divide-border rounded-xl border border-border">
            <MetricRow label={t("storage.trash")} value={formatBytes(data.cleanup.trashBytes)} />
            <MetricRow
              label={t("storage.staleRow")}
              value={formatBytes(data.cleanup.staleUploadBytes)}
              detail={t("storage.sessions", {
                count: data.cleanup.staleUploads.toLocaleString(),
              })}
            />
            <MetricRow
              label={t("storage.totalReclaim")}
              value={formatBytes(data.cleanup.totalReclaimableBytes)}
              strong
            />
          </div>
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs text-muted">{t("storage.noAuto")}</p>
            <LinkButton to="/trash" size="sm" variant="primary">
              {t("storage.review")}
            </LinkButton>
          </div>
        </Card>

        <Card className="gap-5 p-5">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              {t("storage.recent")}
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              {t("storage.recentDesc")}
            </Typography.Paragraph>
          </div>
          <div className="divide-y divide-border rounded-xl border border-border">
            {data.activity.map((activity) => (
              <ActivityRow key={activity.id} activity={activity} />
            ))}
            {data.activity.length === 0 && <EmptyCopy>{t("storage.emptyActivity")}</EmptyCopy>}
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
  const { t } = useAppLocale();
  return (
    <div className="flex items-start justify-between gap-4 px-4 py-3">
      <div className="min-w-0">
        <div className="text-sm font-medium">{activityLabel(t, activity.type)}</div>
        <div className="truncate text-xs text-muted">{activity.label}</div>
      </div>
      <time className="shrink-0 text-xs text-muted" dateTime={activity.occurredAt}>
        {formatRelative(t, activity.occurredAt)}
      </time>
    </div>
  );
}

function StorageGrowthChart({ points }: { points: StorageGrowthPoint[] }) {
  const { t } = useAppLocale();
  if (points.length === 0) return <EmptyCopy>{t("storage.noHistory")}</EmptyCopy>;
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
          aria-label={t("storage.chartAria")}
          className="h-56 w-full"
        >
          <title>{t("storage.chartAria")}</title>
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

function formatRelative(
  t: (key: string, vars?: Record<string, string | number>) => string,
  value: string,
) {
  const delta = Math.max(0, Date.now() - new Date(value).getTime());
  const minutes = Math.floor(delta / 60_000);
  if (minutes < 1) return t("tasks.time.now");
  if (minutes < 60) return t("tasks.time.min", { m: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t("tasks.time.hour", { h: hours });
  const days = Math.floor(hours / 24);
  return days < 7 ? t("tasks.time.day", { d: days }) : new Date(value).toLocaleDateString();
}
