import { Button, Chip, Input, Spinner, TextField, Label } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import CheckIcon from "~icons/gravity-ui/check";
import RefreshIcon from "~icons/gravity-ui/arrow-rotate-left";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { getQueryClient } from "@/lib/queryClient";

export const Route = createFileRoute("/_settings/settings/channels")({
  component: ChannelsSettings,
  pendingComponent: () => (
    <div className="flex justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
});

function ChannelsSettings() {
  const [name, setName] = useState("");
  const [deleteChannel, setDeleteChannel] = useState<{ id: number; name: string } | null>(null);
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/channels",
    { params: { query: { limit: 200 } } },
    { staleTime: 20_000 },
  );
  const create = $api.useMutation("post", "/v1/channels");
  const select = $api.useMutation("post", "/v1/channels/{channelId}/select");
  const sync = $api.useMutation("post", "/v1/channels/sync");
  const remove = $api.useMutation("delete", "/v1/channels/{channelId}", {
    onSuccess: () => {
      setDeleteChannel(null);
      void refresh();
      toast.success("Canal de almacenamiento eliminado");
    },
    onError: (error) => {
      toast.error("No se pudo eliminar el canal de almacenamiento", { description: userMessage(error) });
    },
  });
  const refresh = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/channels").queryKey,
    });

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Canales de almacenamiento"
        description="Canales de Telegram utilizados para almacenar fragmentos cifrados."
        actions={
          <Button
            variant="secondary"
            onPress={async () => {
              try {
                await sync.mutateAsync({
                  params: { header: { "Idempotency-Key": newIdempotencyKey() } },
                });
                await refresh();
                toast.success("Canales sincronizados");
              } catch (error) {
                toast.error("No se pudieron sincronizar los canales", { description: userMessage(error) });
              }
            }}
            isDisabled={sync.isPending}
          >
            <RefreshIcon className="size-4" />
            Buscar y sincronizar
          </Button>
        }
      />
      <SettingsSection
        title="Crear canal"
        description="Teldrive creará y configurará un canal de almacenamiento en Telegram."
      >
        <SettingsRow
          label="Nombre del canal"
          description="Usa un nombre que permita identificar este canal."
        >
          <div className="flex gap-2">
            <TextField className="min-w-0 flex-1">
              <Label className="sr-only">Nombre del canal</Label>
              <Input
                value={name}
                onChange={(event) => setName(event.currentTarget.value)}
                placeholder="Teldrive Storage"
              />
            </TextField>
            <Button
              onPress={async () => {
                if (!name.trim()) return;
                try {
                  await create.mutateAsync({
                    params: { header: { "Idempotency-Key": newIdempotencyKey() } },
                    body: { name: name.trim(), selected: false },
                  });
                  setName("");
                  await refresh();
                  toast.success("Canal de almacenamiento creado");
                } catch (error) {
                  toast.error("No se pudo crear el canal", { description: userMessage(error) });
                }
              }}
              isDisabled={!name.trim() || create.isPending}
            >
              Crear
            </Button>
          </div>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title="Canales configurados"
        description="Elige el canal activo o elimina los canales vacíos que no utilices."
      >
        {query.data.items.length ? (
          query.data.items.map((channel) => (
            <SettingsRow
              key={channel.id}
              label={channel.name}
              description={`Canal ${channel.id}`}
            >
              <div className="flex items-center justify-end gap-2">
                {channel.selected ? (
                  <Chip color="success" variant="tertiary">
                    <CheckIcon className="size-3" />
                    Seleccionado
                  </Chip>
                ) : (
                  <Button
                    size="sm"
                    variant="secondary"
                    onPress={async () => {
                      await select.mutateAsync({ params: { path: { channelId: channel.id } } });
                      await refresh();
                    }}
                  >
                    Usar canal
                  </Button>
                )}
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={`Eliminar ${channel.name}`}
                  isDisabled={remove.isPending && deleteChannel?.id === channel.id}
                  onPress={() => setDeleteChannel({ id: channel.id, name: channel.name })}
                >
                  <TrashIcon className="size-4" />
                </Button>
              </div>
            </SettingsRow>
          ))
        ) : (
          <div className="px-5 py-8 text-sm text-muted">No hay canales de almacenamiento configurados.</div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={deleteChannel !== null}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setDeleteChannel(null);
        }}
        title="¿Eliminar este canal de almacenamiento?"
        message={`“${deleteChannel?.name ?? ""}” solo se puede eliminar si no contiene archivos.`}
        confirmLabel="Eliminar canal"
        isPending={remove.isPending}
        onConfirm={() => {
          if (deleteChannel) {
            remove.mutate({ params: { path: { channelId: deleteChannel.id } } });
          }
        }}
      />
    </div>
  );
}
