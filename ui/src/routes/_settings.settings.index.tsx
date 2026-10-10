import { Button, Chip, Spinner } from "@heroui/react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { useCurrentUser } from "@/auth/use-current-user";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { getQueryClient } from "@/lib/queryClient";
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
      toast.error("Unable to log out", { description: userMessage(error) });
    }
  };

  const displayName = user.data.displayName || user.data.username || `User ${user.data.userId}`;

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Account"
        description="Your authenticated Teldrive profile, storage usage, and current session."
        actions={
          <Button variant="danger" onPress={logOut} isDisabled={logout.isPending}>
            <LogoutIcon className="size-4" />
            Log out
          </Button>
        }
      />
      <SettingsSection
        title="Profile"
        description="This identity comes from your authenticated Telegram account."
      >
        <SettingsRow
          label={displayName}
          description={
            user.data.username ? `@${user.data.username}` : `Telegram user ${user.data.userId}`
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
              {user.data.role === "owner" ? "Owner" : user.data.role === "admin" ? "Admin" : "User"}
            </Chip>
          </div>
        </SettingsRow>
        <SettingsRow
          label="Account created"
          description="When this Teldrive profile was first created."
        >
          <p className="text-right text-sm text-muted">
            {new Date(user.data.createdAt).toLocaleString()}
          </p>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title="Drive statistics"
        description="Current storage totals for this account."
      >
        <SettingsRow label="Files">
          <p className="text-right font-mono text-sm">{stats.data.totalFiles.toLocaleString()}</p>
        </SettingsRow>
        <SettingsRow label="Stored data">
          <p className="text-right font-mono text-sm">{formatBytes(stats.data.totalBytes)}</p>
        </SettingsRow>
        <SettingsRow label="Open uploads">
          <p className="text-right font-mono text-sm">{stats.data.openUploads.toLocaleString()}</p>
        </SettingsRow>
      </SettingsSection>
    </div>
  );
}
