import { Button, Chip, Input, Label, Spinner, TextField } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import RefreshIcon from "~icons/gravity-ui/arrow-rotate-left";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { getQueryClient } from "@/lib/queryClient";

export const Route = createFileRoute("/_settings/settings/users")({
  component: UsersSettings,
  pendingComponent: () => (
    <div className="flex justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
});

function UsersSettings() {
  const [search, setSearch] = useState("");
  const query = $api.useSuspenseQuery(
    "get",
    "/v1/admin/users",
    { params: { query: { search: search.trim() || undefined } } },
    { staleTime: 10_000 },
  );
  const updateUser = $api.useMutation("patch", "/v1/admin/users/{userId}");
  const revokeAccess = $api.useMutation("post", "/v1/admin/users/{userId}/revoke-access");
  const refresh = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/admin/users").queryKey,
    });

  const update = async (userId: number, body: { role?: "admin" | "user"; disabled?: boolean }) => {
    try {
      await updateUser.mutateAsync({ params: { path: { userId } }, body });
      await refresh();
      toast.success("User updated");
    } catch (error) {
      toast.error("User could not be updated", { description: userMessage(error) });
    }
  };

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Users & roles"
        description="Manage who can use this Teldrive instance and which users can administer system-wide features."
        actions={
          <Button variant="secondary" onPress={() => void refresh()}>
            <RefreshIcon className="size-4" />
            Refresh
          </Button>
        }
      />

      <SettingsSection
        title="Users"
        description="The first account is the instance owner and cannot be demoted or disabled."
      >
        <div className="border-b border-border p-4">
          <TextField value={search} onChange={setSearch} className="max-w-md">
            <Label>Search users</Label>
            <Input placeholder="Name, username, or Telegram user ID" />
          </TextField>
        </div>
        {query.data.length ? (
          query.data.map((user) => {
            const displayName =
              user.displayName?.trim() || user.username?.trim() || `User ${user.userId}`;
            const owner = user.role === "owner";
            return (
              <SettingsRow
                key={user.userId}
                label={displayName}
                description={`${user.username ? `@${user.username} · ` : ""}Telegram ID ${user.userId}`}
              >
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Chip
                    variant="tertiary"
                    color={owner ? "accent" : user.role === "admin" ? "warning" : "default"}
                  >
                    {user.role}
                  </Chip>
                  {user.disabled ? (
                    <Chip variant="tertiary" color="danger">
                      Disabled
                    </Chip>
                  ) : null}
                  {!owner ? (
                    <Button
                      size="sm"
                      variant="secondary"
                      isDisabled={updateUser.isPending}
                      onPress={() =>
                        void update(user.userId, { role: user.role === "admin" ? "user" : "admin" })
                      }
                    >
                      {user.role === "admin" ? "Make user" : "Make admin"}
                    </Button>
                  ) : null}
                  {!owner ? (
                    <Button
                      size="sm"
                      variant={user.disabled ? "secondary" : "danger"}
                      isDisabled={updateUser.isPending}
                      onPress={() => void update(user.userId, { disabled: !user.disabled })}
                    >
                      {user.disabled ? "Enable" : "Disable"}
                    </Button>
                  ) : null}
                  {!owner ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      isDisabled={revokeAccess.isPending}
                      onPress={async () => {
                        try {
                          await revokeAccess.mutateAsync({
                            params: { path: { userId: user.userId } },
                          });
                          toast.success("Sessions and API keys revoked");
                        } catch (error) {
                          toast.error("Access could not be revoked", {
                            description: userMessage(error),
                          });
                        }
                      }}
                    >
                      Revoke access
                    </Button>
                  ) : null}
                </div>
              </SettingsRow>
            );
          })
        ) : (
          <p className="p-6 text-sm text-muted">No users match this search.</p>
        )}
      </SettingsSection>
    </div>
  );
}
