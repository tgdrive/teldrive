import { t } from "@/lib/localization";
import { Button, Input, Label, Spinner, TextField } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import CopyIcon from "~icons/gravity-ui/copy";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { ApiKeyCreated } from "@/api/types";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { getQueryClient } from "@/lib/queryClient";

export const Route = createFileRoute("/_settings/settings/api-keys")({
  component: ApiKeysSettings,
  pendingComponent: () => (
    <div className="flex justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
});

function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleString("es") : t("Sin vencimiento");
}

function ApiKeysSettings() {
  const [name, setName] = useState("");
  const [created, setCreated] = useState<ApiKeyCreated>();
  const [revokeKey, setRevokeKey] = useState<{ id: string; name: string } | null>(null);
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/api-keys",
    { params: { query: { limit: 200 } } },
    { staleTime: 20_000 },
  );
  const create = $api.useMutation("post", "/v1/api-keys");
  const revoke = $api.useMutation("delete", "/v1/api-keys/{apiKeyId}", {
    onSuccess: () => {
      setRevokeKey(null);
      void refresh();
      toast.success(t("Clave de API revocada"));
    },
    onError: (error) => {
      toast.error(t("No se pudo revocar la clave de API"), { description: userMessage(error) });
    },
  });
  const refresh = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/api-keys").queryKey,
    });

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("Claves de API")}
        description={t(
          "Credenciales para rclone y clientes externos. El acceso a esta interfaz utiliza tu sesión de Telegram.",
        )}
      />
      <SettingsSection
        title={t("Crear clave de API")}
        description={t("La clave se muestra una sola vez. Guárdala en tu gestor de contraseñas.")}
      >
        <SettingsRow
          label={t("Nombre de la clave")}
          description={t("Elige un nombre que identifique el cliente o equipo.")}
        >
          <div className="flex gap-2">
            <TextField className="min-w-0 flex-1">
              <Label className="sr-only">{t("Nombre de la clave")}</Label>
              <Input
                value={name}
                onChange={(event) => setName(event.currentTarget.value)}
                placeholder="rclone laptop"
              />
            </TextField>
            <Button
              onPress={async () => {
                if (!name.trim()) return;
                try {
                  const result = await create.mutateAsync({
                    params: { header: { "Idempotency-Key": newIdempotencyKey() } },
                    body: { name: name.trim() },
                  });
                  setCreated(result);
                  setName("");
                  await refresh();
                } catch (error) {
                  toast.error(t("No se pudo crear la clave de API"), {
                    description: userMessage(error),
                  });
                }
              }}
              isDisabled={!name.trim() || create.isPending}
            >
              {t("Crear")}
            </Button>
          </div>
        </SettingsRow>
        {created ? (
          <SettingsRow
            label={t("New API key secret")}
            description={t("Copia esta clave ahora. No podrás consultarla de nuevo.")}
          >
            <div className="flex gap-2">
              <Input readOnly value={created.secret} className="min-w-0 flex-1 font-mono" />
              <Button
                isIconOnly
                variant="secondary"
                aria-label={t("Copiar clave de API")}
                onPress={() => {
                  void navigator.clipboard.writeText(created.secret);
                  toast.success(t("Clave de API copiada"));
                }}
              >
                <CopyIcon className="size-4" />
              </Button>
            </div>
          </SettingsRow>
        ) : null}
      </SettingsSection>
      <SettingsSection
        title={t("Claves de API existentes")}
        description={t("Revoca las credenciales que ya no utilices.")}
      >
        {query.data.items.length ? (
          query.data.items.map((item) => (
            <SettingsRow
              key={item.id}
              label={item.name}
              description={t("Creada {0} · last used {1}", [
                formatDate(item.createdAt),
                formatDate(item.lastUsedAt),
              ])}
            >
              <div className="flex justify-end">
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t("Revoke {0}", [item.name])}
                  isDisabled={revoke.isPending && revokeKey?.id === item.id}
                  onPress={() => setRevokeKey({ id: item.id, name: item.name })}
                >
                  <TrashIcon className="size-4" />
                </Button>
              </div>
            </SettingsRow>
          ))
        ) : (
          <div className="px-5 py-8 text-sm text-muted">{t("No API keys created.")}</div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={revokeKey !== null}
        onOpenChange={(open) => {
          if (!open && !revoke.isPending) setRevokeKey(null);
        }}
        title={t("¿Revocar la clave de API?")}
        message={t("Applications using “{0}” will lose access immediately.", [
          revokeKey?.name ?? "",
        ])}
        confirmLabel={t("Revocar clave")}
        isPending={revoke.isPending}
        onConfirm={() => {
          if (revokeKey) {
            revoke.mutate({ params: { path: { apiKeyId: revokeKey.id } } });
          }
        }}
      />
    </div>
  );
}
