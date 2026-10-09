import { Button, Card, Label, ListBox, NumberField, Select, Typography } from "@heroui/react";
import { useStore } from "@tanstack/react-form";
import { toast } from "sonner";
import CloseIcon from "~icons/gravity-ui/xmark";
import type { components } from "@/api/schema";
import { fetchClient } from "@/api/client";
import { useAppForm } from "../forms/app-form";

type JobCreate = components["schemas"]["JobCreate"];
type TaskType = "teldrive_upload_cleanup" | "teldrive_pending_file_purge";

type TaskFormValues = {
  taskType: TaskType;
  batchSize: number;
  queue: string;
  priority: number;
  maxAttempts: number;
};

const GROUPS: {
  label: string;
  items: { key: TaskType; label: string; description: string; defaultQueue: string }[];
}[] = [
  {
    label: "Mantenimiento",
    items: [
      {
        key: "teldrive_upload_cleanup",
        label: "Limpiar subidas abandonadas",
        description: "Finaliza o elimina las sesiones de subida por fragmentos abandonadas.",
        defaultQueue: "maintenance",
      },
      {
        key: "teldrive_pending_file_purge",
        label: "Purgar archivos pendientes",
        description: "Elimina los archivos pendientes vencidos y sus datos en Telegram.",
        defaultQueue: "maintenance",
      },
    ],
  },
];

const DEFAULT_VALUES: TaskFormValues = {
  taskType: "teldrive_upload_cleanup",
  batchSize: 100,
  queue: "maintenance",
  priority: 2,
  maxAttempts: 10,
};

export function TaskLauncher({ onQueued, onClose }: { onQueued: () => void; onClose: () => void }) {
  const form = useAppForm({
    defaultValues: DEFAULT_VALUES,
    validators: {
      onSubmit: ({ value }) => {
        if (value.batchSize < 1) return "El tamaño del lote debe ser al menos 1";
        return undefined;
      },
    },
    onSubmit: async ({ value }) => {
      const body: JobCreate = {
        type: value.taskType,
        args: { batchSize: value.batchSize },
        queue: value.queue.trim() || "maintenance",
        priority: value.priority,
        maxAttempts: value.maxAttempts,
        tags: ["teldrive", "maintenance"],
      };
      const { error } = await fetchClient.POST("/v1/jobs", { body });
      if (error) {
        toast.error("No se pudo programar la tarea");
        throw new Error("No se pudo programar la tarea");
      }
      toast.success("Tarea programada");
      onQueued();
    },
  });
  const values = useStore(form.store, (state) => state.values);
  const submitting = useStore(form.store, (state) => state.isSubmitting);
  const selected = GROUPS.flatMap((group) => group.items).find(
    (item) => item.key === values.taskType,
  )!;

  const chooseTask = (taskType: TaskType) => {
    const task = GROUPS.flatMap((group) => group.items).find((item) => item.key === taskType)!;
    form.setFieldValue("taskType", taskType);
    form.setFieldValue("queue", task.defaultQueue);
    form.setFieldValue("priority", taskType === "teldrive_upload_cleanup" ? 2 : 1);
    form.setFieldValue("maxAttempts", taskType === "teldrive_upload_cleanup" ? 10 : 25);
  };

  return (
    <form.AppForm>
      <div className="fixed inset-0 z-50 flex justify-end">
        <Button
          type="button"
          variant="ghost"
          aria-label="Cerrar panel de tareas"
          className="absolute inset-0 h-full w-full rounded-none bg-black/40 backdrop-blur-[1px]"
          onPress={onClose}
        />
        <form
          className="relative z-10 flex h-full w-full max-w-[52rem] flex-col border-border border-l bg-background shadow-2xl"
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <div className="flex items-start justify-between border-border border-b px-5 py-4 sm:px-6">
            <div>
              <h2 className="text-lg font-semibold">Nueva tarea</h2>
              <p className="mt-0.5 text-xs text-muted">
                Elige una tarea y configura sus parámetros.
              </p>
            </div>
            <Button
              isIconOnly
              size="sm"
              variant="tertiary"
              aria-label="Cerrar panel de tareas"
              onPress={onClose}
            >
              <CloseIcon className="size-4" />
            </Button>
          </div>

          <div className="min-h-0 flex-1 overflow-hidden">
            <div className="border-border border-b p-4 md:hidden">
              <Select
                aria-label="Tipo de tarea"
                selectedKey={values.taskType}
                onSelectionChange={(key) => chooseTask(String(key) as TaskType)}
              >
                <Label>Tipo de tarea</Label>
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {GROUPS.flatMap((group) => group.items).map((item) => (
                      <ListBox.Item key={item.key} id={item.key} textValue={item.label}>
                        <div className="min-w-0">
                          <div className="text-sm font-medium">{item.label}</div>
                          <div className="truncate text-xs text-muted">{item.description}</div>
                        </div>
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            </div>

            <div className="grid h-full min-h-0 md:grid-cols-[12rem_minmax(0,1fr)]">
              <nav
                className="hidden overflow-y-auto border-border border-r px-3 py-4 md:block"
                aria-label="Tipo de tarea"
              >
                {GROUPS.map((group) => (
                  <div key={group.label} className="mb-5 last:mb-0">
                    <div className="mb-1.5 px-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
                      {group.label}
                    </div>
                    <div className="grid gap-0.5">
                      {group.items.map((item) => {
                        const active = values.taskType === item.key;
                        return (
                          <Button
                            key={item.key}
                            type="button"
                            size="sm"
                            variant="ghost"
                            aria-current={active ? "page" : undefined}
                            className={`relative h-9 w-full justify-start rounded-md px-2.5 text-left ${
                              active
                                ? "bg-accent/12 text-accent"
                                : "text-muted hover:text-foreground"
                            }`}
                            onPress={() => chooseTask(item.key)}
                          >
                            {item.label}
                          </Button>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </nav>

              <div className="min-h-0 overflow-y-auto p-5 sm:p-6">
                <div className="grid gap-5">
                  <div>
                    <Typography type="h3" className="text-base font-semibold">
                      {selected.label}
                    </Typography>
                    <Typography.Paragraph className="mt-1 text-sm text-muted">
                      {selected.description}
                    </Typography.Paragraph>
                  </div>

                  <Card className="grid gap-4 bg-surface-secondary p-4">
                    <NumberField
                      value={values.batchSize}
                      minValue={1}
                      maxValue={1000}
                      onChange={(value) => form.setFieldValue("batchSize", value ?? 1)}
                    >
                      <Label>Tamaño del lote</Label>
                      <NumberField.Group>
                        <NumberField.DecrementButton />
                        <NumberField.Input />
                        <NumberField.IncrementButton />
                      </NumberField.Group>
                    </NumberField>
                    <form.AppField name="queue">
                      {(field) => (
                        <field.TextField
                          label="Cola"
                          description="Cola de River utilizada para esta tarea."
                        />
                      )}
                    </form.AppField>
                    <div className="grid gap-4 sm:grid-cols-2">
                      <NumberField
                        value={values.priority}
                        minValue={1}
                        maxValue={4}
                        onChange={(value) => form.setFieldValue("priority", value ?? 1)}
                      >
                        <Label>Prioridad</Label>
                        <NumberField.Group>
                          <NumberField.DecrementButton />
                          <NumberField.Input />
                          <NumberField.IncrementButton />
                        </NumberField.Group>
                      </NumberField>
                      <NumberField
                        value={values.maxAttempts}
                        minValue={1}
                        onChange={(value) => form.setFieldValue("maxAttempts", value ?? 1)}
                      >
                        <Label>Intentos máximos</Label>
                        <NumberField.Group>
                          <NumberField.DecrementButton />
                          <NumberField.Input />
                          <NumberField.IncrementButton />
                        </NumberField.Group>
                      </NumberField>
                    </div>
                  </Card>
                </div>
              </div>
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 border-border border-t px-5 py-4 sm:px-6">
            <Button type="button" variant="tertiary" isDisabled={submitting} onPress={onClose}>
              Cancelar
            </Button>
            <form.SubmitButton variant="primary">
              Programar {selected.label.toLowerCase()}
            </form.SubmitButton>
          </div>
        </form>
      </div>
    </form.AppForm>
  );
}
