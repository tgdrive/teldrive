import { Button, Card, Chip, Spinner, Typography } from "@heroui/react";
import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import ChannelIcon from "~icons/gravity-ui/database";
import CleanupIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { queryClient } from "@/api/query-client";
import type { components } from "@/api/schema";
import { LinkButton } from "@/components/link-button";
import { Page, PageHeader } from "@/components/page";
import { useAppLocale } from "@/lib/locale";
import { useCountFormatter, useDateOnlyFormatter, useFormatBytes } from "@/lib/format";

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
  const formatBytes = useFormatBytes();
  const formatCount = useCountFormatter();
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
          value={formatCount.format(summary.activeFiles)}
          detail={t("storage.folders", { count: formatCount.format(summary.activeFolders) })}
        />
        <StatCard
          label={t("storage.trash")}
          value={formatBytes(summary.trashBytes)}
          detail={t("storage.trashedFiles", { count: formatCount.format(summary.trashedFiles) })}
        />
        <StatCard
          label={t("storage.channels")}
          value={formatCount.format(configuredChannels)}
          detail={t("storage.selectedCount", { count: formatCount.format(selectedChannels) })}
        />
        <StatCard
          label={t("storage.reclaimable")}
          value={formatBytes(data.cleanup.totalReclaimableBytes)}
          detail={t("storage.staleUploads", {
            count: formatCount.format(data.cleanup.staleUploads),
          })}
        />
      </div>

      <StorageGrowthChart points={data.growth} />

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
                        count: formatCount.format(category.totalFiles),
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
                          count: formatCount.format(channel.partCount),
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
                count: formatCount.format(data.cleanup.staleUploads),
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
  const formatDateOnly = useDateOnlyFormatter();
  return (
    <div className="flex items-start justify-between gap-4 px-4 py-3">
      <div className="min-w-0">
        <div className="text-sm font-medium">{activityLabel(t, activity.type)}</div>
        <div className="truncate text-xs text-muted">{activity.label}</div>
      </div>
      <time className="shrink-0 text-xs text-muted" dateTime={activity.occurredAt}>
        {formatRelative(t, formatDateOnly, activity.occurredAt)}
      </time>
    </div>
  );
}

function StorageGrowthChart({ points }: { points: StorageGrowthPoint[] }) {
  const { t } = useAppLocale();
  const formatBytes = useFormatBytes();
  const formatDateOnly = useDateOnlyFormatter();
  const [days, setDays] = useState(60);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const visiblePoints = points.slice(-days);
  const selected = visiblePoints.find((point) => point.day === selectedDay) ?? visiblePoints.at(-1);
  const width = 960;
  const height = 260;
  const padding = { left: 90, right: 24, top: 20, bottom: 40 };
  const plotWidth = width - padding.left - padding.right;
  const plotHeight = height - padding.top - padding.bottom;
  const max = Math.max(4, ...visiblePoints.map((point) => point.addedBytes));
  const coordinates = visiblePoints.map((point, index) => {
    const x = padding.left + (index / Math.max(1, visiblePoints.length - 1)) * plotWidth;
    const y = padding.top + plotHeight - (point.addedBytes / max) * plotHeight;
    return [x, y] as const;
  });
  const path = coordinates
    .map(([x, y], index) => `${index === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`)
    .join(" ");

  return (
    <Card className="gap-5 p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <Typography type="h2" className="text-base font-semibold">
            {t("storage.growth")}
          </Typography>
          <Typography.Paragraph className="max-w-prose text-sm text-muted">
            {t("storage.growthDesc")}
          </Typography.Paragraph>
        </div>
        <fieldset aria-label={t("storage.range")} className="flex flex-wrap gap-1">
          {[7, 30, 60, 90].map((range) => (
            <Button
              key={range}
              size="sm"
              variant={days === range ? "primary" : "tertiary"}
              aria-pressed={days === range}
              onPress={() => {
                setDays(range);
                setSelectedDay(null);
              }}
            >
              {t("storage.days", { count: range })}
            </Button>
          ))}
        </fieldset>
      </div>
      {visiblePoints.length === 0 ? (
        <EmptyCopy>{t("storage.noHistory")}</EmptyCopy>
      ) : (
        <>
          <div
            className="flex flex-wrap items-baseline justify-between gap-2 text-sm"
            aria-live="polite"
            aria-atomic="true"
          >
            <span className="text-muted">
              {selected && formatDateOnly.format(new Date(`${selected.day}T00:00:00`))}
            </span>
            <span className="font-semibold">
              {t("storage.dailyAdded", { bytes: formatBytes(selected?.addedBytes ?? 0) })}
            </span>
          </div>
          <fieldset
            aria-label={t("storage.chartAria", { count: days })}
            className="min-w-0 overflow-x-auto rounded-xl border border-border bg-surface-secondary/40 p-3"
          >
            <svg
              viewBox={`0 0 ${width} ${height}`}
              className="h-auto min-w-[600px] w-full text-muted"
            >
              <title>{t("storage.chartAria", { count: days })}</title>
              {[0, 0.25, 0.5, 0.75, 1].map((ratio) => (
                <g key={ratio}>
                  <line
                    x1={padding.left}
                    x2={width - padding.right}
                    y1={padding.top + plotHeight * (1 - ratio)}
                    y2={padding.top + plotHeight * (1 - ratio)}
                    className="stroke-border"
                    strokeWidth="1"
                  />
                  <text
                    x={padding.left - 12}
                    y={padding.top + plotHeight * (1 - ratio)}
                    textAnchor="end"
                    dominantBaseline="middle"
                    fill="currentColor"
                    fontSize="12"
                  >
                    {formatBytes(Math.round(max * ratio))}
                  </text>
                </g>
              ))}
              <path
                d={path}
                fill="none"
                className="stroke-accent"
                strokeWidth="3"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
              {visiblePoints.map((point, index) => {
                const [x, y] = coordinates[index];
                const label = `${formatDateOnly.format(new Date(`${point.day}T00:00:00`))}: ${t("storage.dailyAdded", { bytes: formatBytes(point.addedBytes) })}`;
                return (
                  <g key={point.day}>
                    <circle
                      cx={x}
                      cy={y}
                      r={selected?.day === point.day ? 5 : 2}
                      className="fill-accent"
                    />
                    {/* biome-ignore lint/a11y/useSemanticElements: SVG hit targets cannot use HTML buttons. */}
                    <rect
                      x={x - plotWidth / Math.max(1, visiblePoints.length - 1) / 2}
                      y={padding.top}
                      width={Math.max(8, plotWidth / Math.max(1, visiblePoints.length - 1))}
                      height={plotHeight}
                      fill="transparent"
                      role="button"
                      tabIndex={0}
                      aria-label={label}
                      aria-pressed={selected?.day === point.day}
                      className="cursor-crosshair focus:stroke-accent focus:outline-none"
                      onPointerEnter={() => setSelectedDay(point.day)}
                      onFocus={() => setSelectedDay(point.day)}
                      onClick={() => setSelectedDay(point.day)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          setSelectedDay(point.day);
                        }
                      }}
                    >
                      <title>{label}</title>
                    </rect>
                  </g>
                );
              })}
              {[
                ...new Set([
                  0,
                  Math.floor((visiblePoints.length - 1) / 2),
                  visiblePoints.length - 1,
                ]),
              ].map((index) => (
                <text
                  key={index}
                  x={coordinates[index][0]}
                  y={height - 10}
                  fill="currentColor"
                  fontSize="12"
                  textAnchor={
                    index === 0 ? "start" : index === visiblePoints.length - 1 ? "end" : "middle"
                  }
                >
                  {formatDateOnly.format(new Date(`${visiblePoints[index].day}T00:00:00`))}
                </text>
              ))}
            </svg>
          </fieldset>
        </>
      )}
    </Card>
  );
}

function EmptyCopy({ children }: { children: React.ReactNode }) {
  return <div className="px-4 py-8 text-center text-sm text-muted">{children}</div>;
}

function formatRelative(
  t: (key: string, vars?: Record<string, string | number>) => string,
  formatDateOnly: Intl.DateTimeFormat,
  value: string,
) {
  const delta = Math.max(0, Date.now() - new Date(value).getTime());
  const minutes = Math.floor(delta / 60_000);
  if (minutes < 1) return t("tasks.time.now");
  if (minutes < 60) return t("tasks.time.min", { m: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t("tasks.time.hour", { h: hours });
  const days = Math.floor(hours / 24);
  return days < 7 ? t("tasks.time.day", { d: days }) : formatDateOnly.format(new Date(value));
}
