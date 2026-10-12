import { Button, Chip, Spinner } from "@heroui/react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { useCurrentUser } from "@/auth/use-current-user";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { getQueryClient } from "@/lib/queryClient";
import { useAppLocale } from "@/lib/locale";
import LogoutIcon from "~icons/gravity-ui/arrow-right-from-square";

export const Route = createFileRoute("/_settings/settings/")({
  component: AccountSettings,
  pendingComponent: () => (
    <div className="flex justify-center py-16">
      <Spinner size="lg" />
    </div>
  ),
});

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const power = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / 1024 ** power).toFixed(power === 0 ? 0 : 1)} ${units[power]}`;
}

function AccountSettings() {
  const navigate = useNavigate();
  const { t } = useAppLocale();
  const user = useCurrentUser();
  const stats = $api.useSuspenseQuery(
    "get",
    "/v1/files/statistics/drive",
    {},
    { staleTime: 30_000 },
  );
  const logout = $api.useMutation("post", "/v1/auth/cookie/logout");

  const logOut = async () => {
    try {
      await logout.mutateAsync({});
      getQueryClient().clear();
      await navigate({ to: "/login", search: { redirect: "/files" }, replace: true });
    } catch (error) {
      toast.error(t("account.logoutFailed"), { description: userMessage(error) });
    }
  };

  const displayName = user.data.displayName || user.data.username || `User ${user.data.userId}`;

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("account.title")}
        description={t("account.desc")}
        actions={
          <Button variant="danger" onPress={logOut} isDisabled={logout.isPending}>
            <LogoutIcon className="size-4" />
            {t("account.logout")}
          </Button>
        }
      />
      <SettingsSection title={t("account.profile")} description={t("account.profileDesc")}>
        <SettingsRow
          label={displayName}
          description={
            user.data.username
              ? `@${user.data.username}`
              : t("account.telegramUser", { id: user.data.userId })
          }
        >
          <div className="flex justify-end">
            <Chip
              variant="tertiary"
              color={
                user.data.role === "owner"
                  ? "accent"
                  : user.data.role === "admin"
                    ? "warning"
                    : "default"
              }
            >
              {t(
                user.data.role === "owner"
                  ? "account.roleOwner"
                  : user.data.role === "admin"
                    ? "account.roleAdmin"
                    : "account.roleUser",
              )}
            </Chip>
          </div>
        </SettingsRow>
        <SettingsRow label={t("account.created")} description={t("account.createdDesc")}>
          <p className="text-right text-sm text-muted">
            {new Date(user.data.createdAt).toLocaleString()}
          </p>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection title={t("account.stats")} description={t("account.statsDesc")}>
        <SettingsRow label={t("account.files")}>
          <p className="text-right font-mono text-sm">{stats.data.totalFiles.toLocaleString()}</p>
        </SettingsRow>
        <SettingsRow label={t("account.stored")}>
          <p className="text-right font-mono text-sm">{formatBytes(stats.data.totalBytes)}</p>
        </SettingsRow>
        <SettingsRow label={t("account.openUploads")}>
          <p className="text-right font-mono text-sm">{stats.data.openUploads.toLocaleString()}</p>
        </SettingsRow>
      </SettingsSection>
    </div>
  );
}
