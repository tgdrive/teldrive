import { Card, Typography } from "@heroui/react";
import type { ReactNode } from "react";

export function SettingsPageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-6 flex flex-col gap-4 border-border border-b pb-5 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        <Typography type="h1" className="text-xl font-semibold tracking-tight">
          {title}
        </Typography>
        <Typography.Paragraph className="mt-1 max-w-2xl text-sm text-muted">
          {description}
        </Typography.Paragraph>
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </header>
  );
}

export function SettingsSection({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <Card className="gap-0 overflow-hidden border border-border bg-surface shadow-none">
      <Card.Header className="flex-col items-start gap-1 border-border border-b px-5 py-4">
        <Card.Title className="text-sm font-semibold">{title}</Card.Title>
        {description ? (
          <Card.Description className="max-w-2xl text-xs leading-relaxed">
            {description}
          </Card.Description>
        ) : null}
      </Card.Header>
      <Card.Content className="p-0">{children}</Card.Content>
    </Card>
  );
}

export function SettingsRow({
  label,
  description,
  children,
  align = "center",
}: {
  label: string;
  description?: string;
  children: ReactNode;
  align?: "center" | "start";
}) {
  return (
    <div
      className={`grid gap-3 border-border border-b px-5 py-4 last:border-b-0 md:grid-cols-[minmax(0,1fr)_minmax(16rem,24rem)] ${
        align === "start" ? "md:items-start" : "md:items-center"
      }`}
    >
      <div className="min-w-0">
        <p className="text-sm font-medium text-foreground">{label}</p>
        {description ? (
          <p className="mt-1 text-xs leading-relaxed text-muted">{description}</p>
        ) : null}
      </div>
      <div className="min-w-0 md:justify-self-stretch">{children}</div>
    </div>
  );
}
