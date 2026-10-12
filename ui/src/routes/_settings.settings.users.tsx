import { Button, Chip, Input, Label, Spinner, TextField } from "@heroui/react";
import { keepPreviousData } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { getQueryClient } from "@/lib/queryClient";
import { useAppLocale } from "@/lib/locale";
import { useDebouncedValue } from "@/lib/use-debounced-value";
import RefreshIcon from "~icons/gravity-ui/arrow-rotate-left";

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
  const { t } = useAppLocale();
  const debouncedSearch = useDebouncedValue(search.trim(), 300);
  const query = $api.useQuery(
    "get",
    "/v1/admin/users",
    { params: { query: { search: debouncedSearch || undefined } } },
    { staleTime: 10_000, placeholderData: keepPreviousData, throwOnError: true },
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
      toast.success(t("users.updated"));
    } catch (error) {
      toast.error(t("users.updateFailed"), { description: userMessage(error) });
    }
  };

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("users.title")}
        description={t("users.desc")}
        actions={
          <Button variant="secondary" onPress={() => void refresh()}>
            <RefreshIcon className="size-4" />
            {t("users.refresh")}
          </Button>
        }
      />

      <SettingsSection title={t("users.users")} description={t("users.usersDesc")}>
        <div className="border-b border-border p-4">
          <TextField value={search} onChange={setSearch} className="max-w-md">
            <Label>{t("users.search")}</Label>
            <Input placeholder={t("users.searchPh")} />
          </TextField>
        </div>
        {query.isPending ? (
          <div className="flex justify-center p-6" role="status" aria-label={t("users.loading")}>
            <Spinner />
          </div>
        ) : query.data?.length ? (
          query.data.map((user) => {
            const displayName =
              user.displayName?.trim() || user.username?.trim() || `User ${user.userId}`;
            const owner = user.role === "owner";
            return (
              <SettingsRow
                key={user.userId}
                label={displayName}
                description={`${user.username ? `@${user.username} · ` : ""}${t("users.telegramId", { id: user.userId })}`}
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
                      {t("users.disabled")}
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
                      {t(user.role === "admin" ? "users.makeUser" : "users.makeAdmin")}
                    </Button>
                  ) : null}
                  {!owner ? (
                    <Button
                      size="sm"
                      variant={user.disabled ? "secondary" : "danger"}
                      isDisabled={updateUser.isPending}
                      onPress={() => void update(user.userId, { disabled: !user.disabled })}
                    >
                      {t(user.disabled ? "users.enable" : "users.disable")}
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
                          toast.success(t("users.revoked"));
                        } catch (error) {
                          toast.error(t("users.revokeFailed"), {
                            description: userMessage(error),
                          });
                        }
                      }}
                    >
                      {t("users.revoke")}
                    </Button>
                  ) : null}
                </div>
              </SettingsRow>
            );
          })
        ) : (
          <p className="p-6 text-sm text-muted">{t("users.empty")}</p>
        )}
      </SettingsSection>
    </div>
  );
}
