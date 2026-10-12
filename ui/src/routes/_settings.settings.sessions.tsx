import { Button, Chip, Spinner } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { getQueryClient } from "@/lib/queryClient";
import { useAppLocale } from "@/lib/locale";

export const Route = createFileRoute("/_settings/settings/sessions")({
  component: SessionsSettings,
  pendingComponent: () => (
    <div className="flex justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
});

function SessionsSettings() {
  const [revokeSessionId, setRevokeSessionId] = useState<string | null>(null);
  const { t } = useAppLocale();
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/sessions",
    { params: { query: { limit: 200 } } },
    { staleTime: 20_000 },
  );
  const refresh = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/sessions").queryKey,
    });
  const revoke = $api.useMutation("delete", "/v1/sessions/{sessionId}", {
    onSuccess: () => {
      setRevokeSessionId(null);
      void refresh();
      toast.success(t("sessions.revoked"));
    },
    onError: (error) => {
      toast.error(t("sessions.revokeFailed"), { description: userMessage(error) });
    },
  });

  return (
    <div className="space-y-6">
      <SettingsPageHeader title={t("sessions.title")} description={t("sessions.desc")} />
      <SettingsSection title={t("sessions.active")} description={t("sessions.activeDesc")}>
        {query.data.items.length ? (
          query.data.items.map((session) => (
            <SettingsRow
              key={session.id}
              label={t(session.current ? "sessions.current" : "sessions.other")}
              description={t("sessions.createdLine", {
                created: new Date(session.createdAt).toLocaleString(),
                expires: new Date(session.expiresAt).toLocaleString(),
              })}
            >
              <div className="flex items-center justify-end gap-2">
                {session.current ? (
                  <Chip color="success" variant="tertiary">
                    {t("sessions.currentChip")}
                  </Chip>
                ) : (
                  <Button
                    isIconOnly
                    size="sm"
                    variant="ghost"
                    aria-label={t("sessions.revokeAria")}
                    isDisabled={revoke.isPending && revokeSessionId === session.id}
                    onPress={() => setRevokeSessionId(session.id)}
                  >
                    <TrashIcon className="size-4" />
                  </Button>
                )}
              </div>
            </SettingsRow>
          ))
        ) : (
          <div className="px-5 py-8 text-sm text-muted">{t("sessions.empty")}</div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={revokeSessionId !== null}
        onOpenChange={(open) => {
          if (!open && !revoke.isPending) setRevokeSessionId(null);
        }}
        title={t("sessions.dialogTitle")}
        message={t("sessions.dialogMsg")}
        confirmLabel={t("sessions.dialogConfirm")}
        isPending={revoke.isPending}
        onConfirm={() => {
          if (revokeSessionId) {
            revoke.mutate({ params: { path: { sessionId: revokeSessionId } } });
          }
        }}
      />
    </div>
  );
}
