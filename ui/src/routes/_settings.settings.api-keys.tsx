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
import { useAppLocale } from "@/lib/locale";

export const Route = createFileRoute("/_settings/settings/api-keys")({
  component: ApiKeysSettings,
  pendingComponent: () => (
    <div className="flex justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
});

function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleString() : undefined;
}

function ApiKeysSettings() {
  const [name, setName] = useState("");
  const [created, setCreated] = useState<ApiKeyCreated>();
  const [revokeKey, setRevokeKey] = useState<{ id: string; name: string } | null>(null);
  const { t } = useAppLocale();
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
      toast.success(t("keys.revoked"));
    },
    onError: (error) => {
      toast.error(t("keys.revokeFailed"), { description: userMessage(error) });
    },
  });
  const refresh = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/api-keys").queryKey,
    });

  return (
    <div className="space-y-6">
      <SettingsPageHeader title={t("keys.title")} description={t("keys.desc")} />
      <SettingsSection title={t("keys.create")} description={t("keys.createDesc")}>
        <SettingsRow label={t("keys.name")} description={t("keys.nameDesc")}>
          <div className="flex gap-2">
            <TextField className="min-w-0 flex-1">
              <Label className="sr-only">{t("keys.name")}</Label>
              <Input
                value={name}
                onChange={(event) => setName(event.currentTarget.value)}
                placeholder={t("keys.namePh")}
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
                  toast.error(t("keys.createFailed"), { description: userMessage(error) });
                }
              }}
              isDisabled={!name.trim() || create.isPending}
            >
              {t("keys.createBtn")}
            </Button>
          </div>
        </SettingsRow>
        {created ? (
          <SettingsRow label={t("keys.secret")} description={t("keys.secretDesc")}>
            <div className="flex gap-2">
              <Input readOnly value={created.secret} className="min-w-0 flex-1 font-mono" />
              <Button
                isIconOnly
                variant="secondary"
                aria-label={t("keys.copyAria")}
                onPress={() => {
                  void navigator.clipboard.writeText(created.secret);
                  toast.success(t("keys.copied"));
                }}
              >
                <CopyIcon className="size-4" />
              </Button>
            </div>
          </SettingsRow>
        ) : null}
      </SettingsSection>
      <SettingsSection title={t("keys.existing")} description={t("keys.existingDesc")}>
        {query.data.items.length ? (
          query.data.items.map((item) => (
            <SettingsRow
              key={item.id}
              label={item.name}
              description={t("keys.createdLine", {
                created: formatDate(item.createdAt) ?? t("keys.never"),
                used: formatDate(item.lastUsedAt) ?? t("keys.never"),
              })}
            >
              <div className="flex justify-end">
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t("keys.revokeAria", { name: item.name })}
                  isDisabled={revoke.isPending && revokeKey?.id === item.id}
                  onPress={() => setRevokeKey({ id: item.id, name: item.name })}
                >
                  <TrashIcon className="size-4" />
                </Button>
              </div>
            </SettingsRow>
          ))
        ) : (
          <div className="px-5 py-8 text-sm text-muted">{t("keys.empty")}</div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={revokeKey !== null}
        onOpenChange={(open) => {
          if (!open && !revoke.isPending) setRevokeKey(null);
        }}
        title={t("keys.revokeTitle")}
        message={t("keys.revokeMsg", { name: revokeKey?.name ?? "" })}
        confirmLabel={t("keys.revokeConfirm")}
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
