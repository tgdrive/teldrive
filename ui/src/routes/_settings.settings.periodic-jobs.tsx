import { createFileRoute } from "@tanstack/react-router";
import {
  Button,
  Card,
  Chip,
  Label,
  ListBox,
  NumberField,
  Select,
  Typography,
  toast,
} from "@heroui/react";
import { useStore } from "@tanstack/react-form";
import { useEffect, useMemo, useState } from "react";
import AddIcon from "~icons/gravity-ui/plus";
import EditIcon from "~icons/gravity-ui/pencil";
import PlayIcon from "~icons/gravity-ui/play";
import TrashBinIcon from "~icons/gravity-ui/trash-bin";
import { AppDialog } from "../components/dialogs/app-dialog";
import { ConfirmDialog } from "../components/dialogs/confirm-dialog";
import { SettingsPageHeader } from "../components/settings-layout";
import type { components } from "@/api/schema";
import { $api as api } from "@/api/client";
import { queryClient } from "@/api/query-client";
import { useAppForm } from "../forms/app-form";

type PeriodicJob = components["schemas"]["PeriodicJob"];
type PeriodicJobTemplate = components["schemas"]["PeriodicJobTemplate"];
type PeriodicJobCreateRequest = components["schemas"]["PeriodicJobCreate"];
type PeriodicJobUpdateRequest = components["schemas"]["PeriodicJobUpdate"];

type JobFormValues = {
  id: string;
  kind: string;
  argsText: string;
  queue: string;
  priority: number;
  maxAttempts: number;
  tagsText: string;
  cronExpression: string;
  cronTimezone: string;
  paused: boolean;
};

const EMPTY_JOB: JobFormValues = {
  id: "",
  kind: "",
  argsText: "{}",
  queue: "cron",
  priority: 1,
  maxAttempts: 25,
  tagsText: "",
  cronExpression: "0 0 * * *",
  cronTimezone: "UTC",
  paused: false,
};

const CRON_PRESETS = [
  { label: "Hourly", value: "0 * * * *" },
  { label: "Every 2 hours", value: "0 */2 * * *" },
  { label: "Every 6 hours", value: "0 */6 * * *" },
  { label: "Daily", value: "0 0 * * *" },
  { label: "Weekly", value: "0 0 * * 0" },
] as const;

export const Route = createFileRoute("/_settings/settings/periodic-jobs")({
  loader: async () => {
    await Promise.all([
      queryClient.ensureQueryData(api.queryOptions("get", "/v1/periodic-jobs")),
      queryClient.ensureQueryData(api.queryOptions("get", "/v1/periodic-jobs/catalog")),
    ]);
  },
  component: PeriodicJobsPage,
});

function PeriodicJobsPage() {
  const { data: jobsResponse } = api.useSuspenseQuery("get", "/v1/periodic-jobs");
  const { data: catalogResponse } = api.useSuspenseQuery("get", "/v1/periodic-jobs/catalog");
  const jobs = jobsResponse.jobs ?? [];
  const templates = catalogResponse.templates ?? [];

  const [editorOpen, setEditorOpen] = useState(false);
  const [editingJob, setEditingJob] = useState<PeriodicJob | null>(null);
  const [deleteJob, setDeleteJob] = useState<PeriodicJob | null>(null);

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: api.queryOptions("get", "/v1/periodic-jobs").queryKey,
    });

  const createJob = api.useMutation("post", "/v1/periodic-jobs", {
    onSuccess: () => {
      toast.success("Periodic job created");
      setEditorOpen(false);
      void invalidate();
    },
    onError: () => toast.danger("Failed to create periodic job"),
  });
  const updateJob = api.useMutation("put", "/v1/periodic-jobs/{periodicJobId}", {
    onSuccess: () => {
      toast.success("Periodic job updated");
      setEditorOpen(false);
      void invalidate();
    },
    onError: () => toast.danger("Failed to update periodic job"),
  });
  const deleteMutation = api.useMutation("delete", "/v1/periodic-jobs/{periodicJobId}", {
    onSuccess: () => {
      toast.success("Periodic job deleted");
      setDeleteJob(null);
      void invalidate();
    },
    onError: () => toast.danger("Failed to delete periodic job"),
  });
  const pauseMutation = api.useMutation("post", "/v1/periodic-jobs/{periodicJobId}/pause", {
    onSuccess: () => {
      toast.success("Periodic job paused");
      void invalidate();
    },
    onError: () => toast.danger("Failed to pause periodic job"),
  });
  const resumeMutation = api.useMutation("post", "/v1/periodic-jobs/{periodicJobId}/resume", {
    onSuccess: () => {
      toast.success("Periodic job resumed");
      void invalidate();
    },
    onError: () => toast.danger("Failed to resume periodic job"),
  });

  const openCreate = () => {
    setEditingJob(null);
    setEditorOpen(true);
  };
  const openEdit = (job: PeriodicJob) => {
    setEditingJob(job);
    setEditorOpen(true);
  };

  const submitJob = async (values: JobFormValues) => {
    const args = parseArguments(values.argsText);
    const common = {
      kind: values.kind.trim(),
      args,
      queue: values.queue.trim(),
      priority: values.priority,
      maxAttempts: values.maxAttempts,
      tags: values.tagsText
        .split(",")
        .map((tag) => tag.trim())
        .filter(Boolean),
      cronExpression: values.cronExpression.trim(),
      cronTimezone: values.cronTimezone.trim() || "UTC",
    } satisfies PeriodicJobUpdateRequest;

    if (editingJob) {
      await updateJob.mutateAsync({
        params: { path: { periodicJobId: editingJob.id ?? values.id } },
        body: common,
      });
      return;
    }
    const body: PeriodicJobCreateRequest = {
      ...common,
      id: values.id.trim(),
      paused: values.paused,
    };
    await createJob.mutateAsync({ body });
  };

  return (
    <div className="flex flex-col gap-5">
      <SettingsPageHeader
        title="Periodic Jobs"
        description="Manage durable schedules stored in PostgreSQL and shared by every worker instance."
        actions={
          <Button variant="primary" onPress={openCreate}>
            <AddIcon className="size-4" /> Add periodic job
          </Button>
        }
      />

      {jobs.length === 0 ? (
        <Card className="flex min-h-44 items-center justify-center border border-border bg-surface p-6 text-center shadow-none">
          <div className="max-w-md">
            <Typography type="h3" className="text-sm font-semibold">
              No periodic jobs configured
            </Typography>
            <Typography.Paragraph className="mt-2 text-sm text-muted">
              Add a schedule when ready. Worker startup will not recreate deleted jobs.
            </Typography.Paragraph>
            <Button className="mt-4" size="sm" variant="primary" onPress={openCreate}>
              <AddIcon className="size-4" /> Create first job
            </Button>
          </div>
        </Card>
      ) : (
        <div className="grid gap-3">
          {jobs.map((job) => (
            <PeriodicJobCard
              key={job.id}
              job={job}
              isToggling={pauseMutation.isPending || resumeMutation.isPending}
              onEdit={() => openEdit(job)}
              onDelete={() => setDeleteJob(job)}
              onToggle={() => {
                const id = job.id ?? "";
                if (job.paused) resumeMutation.mutate({ params: { path: { periodicJobId: id } } });
                else pauseMutation.mutate({ params: { path: { periodicJobId: id } } });
              }}
            />
          ))}
        </div>
      )}

      <PeriodicJobEditor
        open={editorOpen}
        editingJob={editingJob}
        templates={templates}
        onOpenChange={setEditorOpen}
        onSubmit={submitJob}
      />

      <ConfirmDialog
        open={Boolean(deleteJob)}
        onOpenChange={(open) => {
          if (!open) setDeleteJob(null);
        }}
        title="Delete periodic job?"
        message={`The schedule “${deleteJob?.id ?? ""}” will be removed permanently.`}
        confirmLabel="Delete job"
        isPending={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteJob?.id)
            deleteMutation.mutate({ params: { path: { periodicJobId: deleteJob.id } } });
        }}
      />
    </div>
  );
}

function PeriodicJobCard({
  job,
  isToggling,
  onEdit,
  onDelete,
  onToggle,
}: {
  job: PeriodicJob;
  isToggling: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onToggle: () => void;
}) {
  return (
    <Card className="border border-border bg-surface p-4 shadow-none">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Typography type="h3" className="truncate text-sm font-semibold">
              {job.id}
            </Typography>
            <Chip size="sm" variant="tertiary" color={job.paused ? "warning" : "success"}>
              {job.paused ? "Paused" : "Active"}
            </Chip>
            <Chip size="sm" variant="tertiary">
              {job.kind}
            </Chip>
          </div>
          <div className="mt-3 grid gap-2 text-xs sm:grid-cols-2 lg:grid-cols-4">
            <JobDetail
              label="Schedule"
              value={`${job.cronExpression ?? "—"} (${job.cronTimezone ?? "UTC"})`}
              mono
            />
            <JobDetail
              label="Next run"
              value={job.paused ? "Paused" : formatDateTime(job.nextRunAt)}
            />
            <JobDetail label="Queue" value={job.queue ?? "default"} />
            <JobDetail label="Attempts" value={String(job.maxAttempts ?? 25)} />
          </div>
          <div className="mt-3 truncate font-mono text-[11px] text-muted">
            {JSON.stringify(job.args ?? {})}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2 lg:justify-end">
          <Button size="sm" variant="tertiary" isPending={isToggling} onPress={onToggle}>
            {job.paused ? <PlayIcon className="size-4" /> : null}
            {job.paused ? "Resume" : "Pause"}
          </Button>
          <Button size="sm" variant="tertiary" onPress={onEdit}>
            <EditIcon className="size-4" /> Edit
          </Button>
          <Button size="sm" variant="danger-soft" onPress={onDelete}>
            <TrashBinIcon className="size-4" /> Delete
          </Button>
        </div>
      </div>
    </Card>
  );
}

function JobDetail({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div>
      <div className="text-muted">{label}</div>
      <div
        className={
          mono ? "mt-0.5 truncate font-mono text-foreground" : "mt-0.5 truncate text-foreground"
        }
      >
        {value}
      </div>
    </div>
  );
}

function PeriodicJobEditor({
  open,
  editingJob,
  templates,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  editingJob: PeriodicJob | null;
  templates: PeriodicJobTemplate[];
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: JobFormValues) => Promise<void>;
}) {
  const editing = Boolean(editingJob);
  const initialValues = useMemo(
    () =>
      editingJob
        ? formFromJob(editingJob)
        : templates[0]
          ? formFromTemplate(templates[0])
          : EMPTY_JOB,
    [editingJob, templates],
  );
  const form = useAppForm({
    defaultValues: initialValues,
    validators: {
      onSubmit: ({ value }) => {
        const fields: Partial<Record<keyof JobFormValues, string>> = {};
        if (!editing && !value.id.trim()) fields.id = "Job ID is required";
        if (!value.kind.trim()) fields.kind = "Worker kind is required";
        if (!value.cronExpression.trim()) fields.cronExpression = "Cron expression is required";
        try {
          parseArguments(value.argsText);
        } catch (error) {
          fields.argsText = error instanceof Error ? error.message : "Invalid JSON";
        }
        return Object.keys(fields).length ? { fields } : undefined;
      },
    },
    onSubmit: async ({ value }) => onSubmit(value),
  });
  useEffect(() => {
    if (open) form.reset(initialValues);
  }, [form, initialValues, open]);

  const values = useStore(form.store, (state) => state.values);
  const isSubmitting = useStore(form.store, (state) => state.isSubmitting);
  const cronExpression = values.cronExpression;
  const scheduleDescription = useMemo(() => describeCron(cronExpression), [cronExpression]);

  const chooseTemplate = (kind: string) => {
    const template = templates.find((item) => item.kind === kind);
    if (!template) {
      form.setFieldValue("kind", kind);
      return;
    }
    const current = values;
    form.reset({
      ...formFromTemplate(template),
      id: editing ? current.id : (template.defaultId ?? current.id),
      paused: current.paused,
    });
  };

  return (
    <form.AppForm>
      <AppDialog
        className="sm:w-[min(92vw,50rem)]"
        open={open}
        onOpenChange={onOpenChange}
        title={editing ? "Edit periodic job" : "Add periodic job"}
        description="Configure a durable River job definition and cron schedule."
        bodyClassName="overflow-x-hidden px-2"
        isDismissable={!isSubmitting}
        footer={
          <>
            <Button
              type="button"
              variant="tertiary"
              isDisabled={form.state.isSubmitting}
              onPress={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <form.SubmitButton form="periodic-job-form" variant="primary">
              {editing ? "Save changes" : "Create job"}
            </form.SubmitButton>
          </>
        }
      >
        <form
          id="periodic-job-form"
          className="grid min-w-0 gap-5 py-1"
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <Select
              selectedKey={values.kind}
              onSelectionChange={(key) => chooseTemplate(String(key))}
            >
              <Label>Job template</Label>
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {templates.map((template) => (
                    <ListBox.Item
                      key={template.kind}
                      id={template.kind ?? ""}
                      textValue={template.label ?? template.kind}
                    >
                      <div>
                        <div className="text-sm font-medium">{template.label ?? template.kind}</div>
                        <div className="text-xs text-muted">{template.description}</div>
                      </div>
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
            <form.AppField name="id">
              {(field) => (
                <field.TextField
                  label="Job ID"
                  isRequired
                  isDisabled={editing}
                  description="Stable unique identifier for this schedule."
                />
              )}
            </form.AppField>
          </div>

          <form.AppField name="kind">
            {(field) => (
              <field.TextField
                label="Worker kind"
                isRequired
                description="Must match a registered River worker kind."
              />
            )}
          </form.AppField>

          <Card className="gap-4 bg-surface-secondary p-4">
            <div>
              <Typography type="h3" className="text-sm font-semibold">
                Schedule
              </Typography>
              <Typography.Paragraph className="text-xs text-muted">
                {scheduleDescription}
              </Typography.Paragraph>
            </div>
            <div className="flex flex-wrap gap-2">
              {CRON_PRESETS.map((preset) => (
                <Button
                  key={preset.value}
                  type="button"
                  size="sm"
                  variant={cronExpression === preset.value ? "primary" : "tertiary"}
                  onPress={() => form.setFieldValue("cronExpression", preset.value)}
                >
                  {preset.label}
                </Button>
              ))}
            </div>
            <div className="grid gap-4">
              <form.AppField name="cronExpression">
                {(field) => (
                  <field.TextField
                    label="Cron expression"
                    isRequired
                    className="font-mono"
                    description="Five-field cron: minute, hour, day, month, weekday."
                  />
                )}
              </form.AppField>
              <form.AppField name="cronTimezone">
                {(field) => (
                  <field.TextField
                    label="Timezone"
                    description="IANA name such as UTC or Asia/Kolkata."
                  />
                )}
              </form.AppField>
            </div>
          </Card>

          <div className="grid gap-4 sm:grid-cols-3">
            <form.AppField name="queue">
              {(field) => <field.TextField label="Queue" />}
            </form.AppField>
            <form.AppField name="priority">
              {(field) => (
                <NumberField
                  value={field.state.value}
                  minValue={1}
                  maxValue={4}
                  onChange={(value) => field.handleChange(value ?? 1)}
                >
                  <Label>Priority</Label>
                  <NumberField.Group>
                    <NumberField.DecrementButton />
                    <NumberField.Input />
                    <NumberField.IncrementButton />
                  </NumberField.Group>
                </NumberField>
              )}
            </form.AppField>
            <form.AppField name="maxAttempts">
              {(field) => (
                <NumberField
                  value={field.state.value}
                  minValue={1}
                  onChange={(value) => field.handleChange(value ?? 1)}
                >
                  <Label>Max attempts</Label>
                  <NumberField.Group>
                    <NumberField.DecrementButton />
                    <NumberField.Input />
                    <NumberField.IncrementButton />
                  </NumberField.Group>
                </NumberField>
              )}
            </form.AppField>
          </div>

          <form.AppField name="tagsText">
            {(field) => (
              <field.TextField
                label="Tags"
                placeholder="metadata, sync"
                description="Optional comma-separated River tags."
              />
            )}
          </form.AppField>
          <form.AppField name="argsText">
            {(field) => (
              <field.TextAreaField
                label="Job arguments"
                className="min-h-52 resize-y font-mono text-sm"
                spellCheck={false}
                description="JSON object passed to the selected River worker."
              />
            )}
          </form.AppField>
          {!editing ? (
            <form.AppField name="paused">
              {(field) => (
                <field.SwitchField
                  label="Create paused"
                  description="Save without enqueueing until resumed."
                />
              )}
            </form.AppField>
          ) : null}
        </form>
      </AppDialog>
    </form.AppForm>
  );
}

function formFromTemplate(template: PeriodicJobTemplate): JobFormValues {
  return {
    ...EMPTY_JOB,
    id: template.defaultId ?? "",
    kind: template.kind ?? "",
    argsText: JSON.stringify(template.defaultArgs ?? {}, null, 2),
    queue: template.defaultQueue ?? "cron",
    cronExpression: template.recommendedCron ?? "0 0 * * *",
  };
}

function formFromJob(job: PeriodicJob): JobFormValues {
  return {
    id: job.id ?? "",
    kind: job.kind ?? "",
    argsText: JSON.stringify(job.args ?? {}, null, 2),
    queue: job.queue ?? "default",
    priority: job.priority ?? 1,
    maxAttempts: job.maxAttempts ?? 25,
    tagsText: (job.tags ?? []).join(", "),
    cronExpression: job.cronExpression ?? "",
    cronTimezone: job.cronTimezone ?? "UTC",
    paused: job.paused ?? false,
  };
}

function parseArguments(value: string): Record<string, unknown> {
  const parsed: unknown = JSON.parse(value || "{}");
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object")
    throw new Error("Arguments must be a JSON object");
  return parsed as Record<string, unknown>;
}

function describeCron(expression: string): string {
  const preset = CRON_PRESETS.find((item) => item.value === expression.trim());
  if (preset) return `${preset.label} in the selected timezone.`;
  if (!expression.trim()) return "Enter a cron expression.";
  return `Custom schedule: ${expression.trim()}`;
}

function formatDateTime(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    date,
  );
}
