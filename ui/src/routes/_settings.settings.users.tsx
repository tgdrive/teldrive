import { t } from "@/lib/localization";
import { Button, Chip, Input, Label, Spinner, TextField } from "@heroui/react";
import { keepPreviousData } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { getQueryClient } from "@/lib/queryClient";
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
      toast.success(t("Usuario actualizado"));
    } catch (error) {
      toast.error(t("No se pudo actualizar el usuario"), { description: userMessage(error) });
    }
  };

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("Usuarios y permisos")}
        description={t(
          "Administra quién puede usar este servidor y quién puede gestionar sus funciones de sistema.",
        )}
        actions={
          <Button variant="secondary" onPress={() => void refresh()}>
            <RefreshIcon className="size-4" />
            {t("Actualizar")}
          </Button>
        }
      />

      <SettingsSection
        title={t("Usuarios")}
        description={t(
          "La primera cuenta es la propietaria del servidor y no se puede degradar ni desactivar.",
        )}
      >
        <div className="border-b border-border p-4">
          <TextField value={search} onChange={setSearch} className="max-w-md">
            <Label>{t("Buscar usuarios")}</Label>
            <Input placeholder={t("Nombre, usuario o ID de Telegram")} />
          </TextField>
        </div>
        {query.isPending ? (
          <div
            className="flex justify-center p-6"
            role="status"
            aria-label={t("Cargando usuarios")}
          >
            <Spinner />
          </div>
        ) : query.data?.length ? (
          query.data.map((user) => {
            const displayName =
              user.displayName?.trim() ||
              user.username?.trim() ||
              t("User {id}", { id: user.userId });
            const owner = user.role === "owner";
            return (
              <SettingsRow
                key={user.userId}
                label={displayName}
                description={`${user.username ? `@${user.username} · ` : ""}${t("Telegram ID {id}", { id: user.userId })}`}
              >
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Chip
                    variant="tertiary"
                    color={owner ? "accent" : user.role === "admin" ? "warning" : "default"}
                  >
                    {user.role === "owner"
                      ? t("Propietario")
                      : user.role === "admin"
                        ? t("Administrador")
                        : t("Usuario")}
                  </Chip>
                  {user.disabled ? (
                    <Chip variant="tertiary" color="danger">
                      {t("Desactivado")}
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
                          toast.success(t("Sesiones y claves de API revocadas"));
                        } catch (error) {
                          toast.error(t("No se pudo revocar el acceso"), {
                            description: userMessage(error),
                          });
                        }
                      }}
                    >
                      {t("Revocar acceso")}
                    </Button>
                  ) : null}
                </div>
              </SettingsRow>
            );
          })
        ) : (
          <p className="p-6 text-sm text-muted">{t("Ningún usuario coincide con la búsqueda.")}</p>
        )}
      </SettingsSection>
    </div>
  );
}
