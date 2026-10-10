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
      toast.success("Session revoked");
    },
    onError: (error) => {
      toast.error("Session could not be revoked", { description: userMessage(error) });
    },
  });

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Sessions"
        description="Sessions currently authorized for this account."
      />
      <SettingsSection
        title="Active sessions"
        description="Revoke any session you no longer recognize or use."
      >
        {query.data.items.length ? (
          query.data.items.map((session) => (
            <SettingsRow
              key={session.id}
              label={session.current ? "Current session" : "Teldrive session"}
              description={`Created ${new Date(session.createdAt).toLocaleString()} · expires ${new Date(session.expiresAt).toLocaleString()}`}
            >
              <div className="flex items-center justify-end gap-2">
                {session.current ? (
                  <Chip color="success" variant="tertiary">
                    Current
                  </Chip>
                ) : (
                  <Button
                    isIconOnly
                    size="sm"
                    variant="ghost"
                    aria-label="Revoke session"
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
          <div className="px-5 py-8 text-sm text-muted">No sessions found.</div>
        )}
      </SettingsSection>
      <ConfirmDialog
        open={revokeSessionId !== null}
        onOpenChange={(open) => {
          if (!open && !revoke.isPending) setRevokeSessionId(null);
        }}
        title="Revoke session?"
        message="This client will need to sign in again."
        confirmLabel="Revoke session"
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
