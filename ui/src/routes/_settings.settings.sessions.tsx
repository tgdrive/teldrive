import { Button, Chip, Spinner } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
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
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/sessions",
    { params: { query: { limit: 200 } } },
    { staleTime: 20_000 },
  );
  const revoke = $api.useMutation("delete", "/v1/sessions/{sessionId}");
  const refresh = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/sessions").queryKey,
    });

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Sessions"
        description="Browser and client sessions currently authorized for this account."
      />
      <SettingsSection
        title="Active sessions"
        description="Revoke any session you no longer recognize or use."
      >
        {query.data.items.length ? (
          query.data.items.map((session) => (
            <SettingsRow
              key={session.id}
              label={session.current ? "Current browser session" : "Teldrive session"}
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
                    isDisabled={revoke.isPending}
                    onPress={async () => {
                      if (
                        !window.confirm(
                          "Revoke this session? The client will need to sign in again.",
                        )
                      )
                        return;
                      await revoke.mutateAsync({ params: { path: { sessionId: session.id } } });
                      await refresh();
                    }}
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
    </div>
  );
}
