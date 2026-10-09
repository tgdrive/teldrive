import { Chip } from "@heroui/react";

const ACTIVE_STATUSES = new Set(["pending", "scheduled", "available", "running", "retryable"]);

export function taskStatusLabel(status: string) {
  return (
    (
      {
        pending: "Pendiente",
        queued: "En cola",
        paused: "En pausa",
        failed: "Fallida",
        scheduled: "Programada",
        available: "Disponible",
        running: "En ejecución",
        retryable: "Reintentable",
        cancelled: "Cancelada",
        discarded: "Descartada",
        completed: "Completada",
      } as Record<string, string>
    )[status] ?? status
  );
}

export function TaskStatusChip({ status, animate = true }: { status: string; animate?: boolean }) {
  const active = ACTIVE_STATUSES.has(status);

  return (
    <Chip size="sm" variant="soft" color={taskStatusColor(status)}>
      <span className="flex items-center gap-1.5">
        <span className="relative flex size-2 shrink-0" aria-hidden="true">
          {active && animate ? (
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-current opacity-30 motion-reduce:animate-none" />
          ) : null}
          <span className="relative inline-flex size-2 rounded-full bg-current" />
        </span>
        {taskStatusLabel(status)}
      </span>
    </Chip>
  );
}

function taskStatusColor(status: string): "accent" | "success" | "warning" | "danger" | "default" {
  switch (status) {
    case "completed":
      return "success";
    case "running":
      return "accent";
    case "pending":
    case "scheduled":
    case "available":
    case "retryable":
      return "warning";
    case "discarded":
      return "danger";
    default:
      return "default";
  }
}
