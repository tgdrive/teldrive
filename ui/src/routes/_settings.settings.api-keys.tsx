import { Button, Input, Label, Spinner, TextField } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import CopyIcon from "~icons/gravity-ui/copy";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { ApiKeyCreated } from "@/api/types";
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
  return value ? new Date(value).toLocaleString() : "never";
}

function ApiKeysSettings() {
  const [name, setName] = useState("");
  const [created, setCreated] = useState<ApiKeyCreated>();
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/api-keys",
    { params: { query: { limit: 200 } } },
    { staleTime: 20_000 },
  );
  const create = $api.useMutation("post", "/v1/api-keys");
  const revoke = $api.useMutation("delete", "/v1/api-keys/{apiKeyId}");
  const refresh = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/api-keys").queryKey,
    });

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="API keys"
        description="Credentials for rclone and external API clients. They cannot sign in to this browser UI."
      />
      <SettingsSection
        title="Create API key"
        description="The secret is shown once. Store it in a password manager."
      >
        <SettingsRow
          label="Key name"
          description="Use a name that identifies the client or machine."
        >
          <div className="flex gap-2">
            <TextField className="min-w-0 flex-1">
              <Label className="sr-only">Key name</Label>
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
                  toast.error("API key could not be created", { description: userMessage(error) });
                }
              }}
              isDisabled={!name.trim() || create.isPending}
            >
              Create
            </Button>
          </div>
        </SettingsRow>
        {created ? (
          <SettingsRow
            label="New API key secret"
            description="Copy this value now. It cannot be retrieved later."
          >
            <div className="flex gap-2">
              <Input readOnly value={created.secret} className="min-w-0 flex-1 font-mono" />
              <Button
                isIconOnly
                variant="secondary"
                aria-label="Copy API key"
                onPress={() => {
                  void navigator.clipboard.writeText(created.secret);
                  toast.success("API key copied");
                }}
              >
                <CopyIcon className="size-4" />
              </Button>
            </div>
          </SettingsRow>
        ) : null}
      </SettingsSection>
      <SettingsSection
        title="Existing API keys"
        description="Revoke credentials that are no longer in use."
      >
        {query.data.items.length ? (
          query.data.items.map((item) => (
            <SettingsRow
              key={item.id}
              label={item.name}
              description={`Created ${formatDate(item.createdAt)} · last used ${formatDate(item.lastUsedAt)}`}
            >
              <div className="flex justify-end">
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={`Revoke ${item.name}`}
                  isDisabled={revoke.isPending}
                  onPress={async () => {
                    if (
                      !window.confirm(
                        "Revoke this API key? Applications using it will lose access immediately.",
                      )
                    )
                      return;
                    await revoke.mutateAsync({ params: { path: { apiKeyId: item.id } } });
                    await refresh();
                    toast.success("API key revoked");
                  }}
                >
                  <TrashIcon className="size-4" />
                </Button>
              </div>
            </SettingsRow>
          ))
        ) : (
          <div className="px-5 py-8 text-sm text-muted">No API keys created.</div>
        )}
      </SettingsSection>
    </div>
  );
}
