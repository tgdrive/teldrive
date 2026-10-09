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
  archive: "Archivos comprimidos",
  audio: "Audio",
  document: "Documentos",
  image: "Imágenes",
  video: "Vídeo",
  other: "Otros",
};

const ACTIVITY_LABELS: Record<string, string> = {
  "file.created": "Archivo añadido",
  "file.trashed": "Archivo enviado a la papelera",
  "file.restored": "Archivo restaurado",
  "file.purged": "Archivo eliminado definitivamente",
  "upload.completed": "Subida completada",
  "upload.aborted": "Subida cancelada",
  "upload.expired": "Subida vencida",
  "share.created": "Enlace compartido creado",
  "share.deleted": "Enlace compartido eliminado",
  "channel.created": "Canal de almacenamiento añadido",
  "channel.updated": "Canal de almacenamiento actualizado",
  "channel.deleted": "Canal de almacenamiento retirado",
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
  const selectedChannels = data.channels.filter((channel) => channel.selected).length;
  const totalChannelParts = data.channels.reduce((total, channel) => total + channel.partCount, 0);

  return (
    <Page>
      <PageHeader
        title="Almacenamiento"
        description="Uso, crecimiento, distribución, limpieza y actividad del almacenamiento en Telegram."
      />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Total almacenado"
          value={formatBytes(summary.logicalBytes)}
          detail={`${formatBytes(data.growth.at(-1)?.addedBytes ?? 0)} añadidos hoy`}
        />
        <StatCard
          label="Archivos activos"
          value={summary.activeFiles.toLocaleString()}
          detail={`${summary.activeFolders.toLocaleString()} carpetas`}
        />
        <StatCard
          label="Papelera"
          value={formatBytes(summary.trashBytes)}
          detail={`${summary.trashedFiles.toLocaleString()} archivos`}
        />
        <StatCard
          label="Canales"
          value={configuredChannels.toLocaleString()}
          detail={`${selectedChannels.toLocaleString()} seleccionados`}
        />
        <StatCard
          label="Recuperable"
          value={formatBytes(data.cleanup.totalReclaimableBytes)}
          detail={`${data.cleanup.staleUploads.toLocaleString()} subidas abandonadas`}
        />
      </div>

      <Card className="gap-5 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              Crecimiento del almacenamiento
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              Bytes lógicos almacenados durante los últimos 30 días.
            </Typography.Paragraph>
          </div>
          <Chip variant="tertiary">30 días</Chip>
        </div>
        <StorageGrowthChart points={data.growth} />
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card className="gap-5 p-5">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              Composición del almacenamiento
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              Espacio de archivos activos agrupado por tipo.
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
                      archivos
                    </span>
                  </div>
                  <ProgressTrack value={percent} label={`${category.category} storage`} />
                </div>
              );
            })}
            {data.categories.length === 0 && (
              <EmptyCopy>Todavía no hay archivos activos almacenados.</EmptyCopy>
            )}
          </div>
        </Card>

        <Card className="gap-5 p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Typography type="h2" className="text-base font-semibold">
                Distribución entre canales de Telegram
              </Typography>
              <Typography.Paragraph className="text-sm text-muted">
                Fragmentos almacenados en los canales de Telegram configurados.
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
                            Seleccionado
                          </Chip>
                        )}
                      </div>
                      <div className="mt-1 text-xs text-muted">
                        {channel.partCount.toLocaleString()} fragmentos · {percent.toFixed(1)}%
                      </div>
                    </div>
                  </div>
                  <ProgressTrack
                    value={percent}
                    label={`${channel.name}: distribución de almacenamiento`}
                  />
                </div>
              );
            })}
            {data.channels.length === 0 && (
              <EmptyCopy>No hay canales de almacenamiento configurados.</EmptyCopy>
            )}
          </div>
        </Card>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card className="gap-5 p-5">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Typography type="h2" className="text-base font-semibold">
                Opciones para liberar espacio
              </Typography>
              <Typography.Paragraph className="text-sm text-muted">
                Almacenamiento que puedes revisar para eliminar definitivamente.
              </Typography.Paragraph>
            </div>
            <CleanupIcon className="size-5 text-muted" />
          </div>
          <div className="divide-y divide-border rounded-xl border border-border">
            <MetricRow label="Papelera" value={formatBytes(data.cleanup.trashBytes)} />
            <MetricRow label="Spam propio" value={formatBytes(data.summary.spamBytes ?? 0)} />
            <MetricRow
              label="Subidas por fragmentos abandonadas"
              value={formatBytes(data.cleanup.staleUploadBytes)}
              detail={`${data.cleanup.staleUploads.toLocaleString()} sesiones`}
            />
            <MetricRow
              label="Total que puedes liberar"
              value={formatBytes(data.cleanup.totalReclaimableBytes)}
              strong
            />
          </div>
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs text-muted">Este panel no elimina archivos automáticamente.</p>
            <LinkButton to="/trash" size="sm" variant="primary">
              Revisar limpieza
            </LinkButton>
          </div>
        </Card>

        <Card className="gap-5 p-5">
          <div>
            <Typography type="h2" className="text-base font-semibold">
              Actividad reciente del almacenamiento
            </Typography>
            <Typography.Paragraph className="text-sm text-muted">
              Registro de archivos, subidas, enlaces compartidos y canales.
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
  if (points.length === 0) return <EmptyCopy>No hay historial de almacenamiento.</EmptyCopy>;
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
          aria-label="Crecimiento de almacenamiento durante 30 días"
          className="h-56 w-full"
        >
          <title>Crecimiento de almacenamiento durante 30 días</title>
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
  if (minutes < 1) return "ahora";
  if (minutes < 60) return `hace ${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `hace ${hours} h`;
  const days = Math.floor(hours / 24);
  return days < 7 ? `hace ${days} días` : new Date(value).toLocaleDateString();
}
