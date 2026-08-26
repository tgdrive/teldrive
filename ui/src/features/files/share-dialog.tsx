import { Button, Chip, Input, Label, ListBox, Select, Spinner, TextField } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { useDeferredValue, useEffect, useState } from "react";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { copyText } from "@/features/files/download";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { getQueryClient } from "@/lib/queryClient";

type Permission = "read" | "edit";
type ExpirationMode = "never" | "custom";

const DURATION_UNITS: Record<string, number> = {
  ms: 1,
  s: 1000,
  m: 60 * 1000,
  h: 60 * 60 * 1000,
  d: 24 * 60 * 60 * 1000,
  w: 7 * 24 * 60 * 60 * 1000,
  M: 30 * 24 * 60 * 60 * 1000,
  y: 365 * 24 * 60 * 60 * 1000,
};

export function ShareDialog({
  file,
  onOpenChange,
}: {
  file?: FileEntry;
  onOpenChange: (open: boolean) => void;
}) {
  const fileId = file?.id ?? "00000000-0000-0000-0000-000000000000";
  const [peopleSearch, setPeopleSearch] = useState("");
  const deferredPeopleSearch = useDeferredValue(peopleSearch.trim());
  const [peoplePermission, setPeoplePermission] = useState<Permission>("read");
  const [linkPermission, setLinkPermission] = useState<Permission>("read");
  const [linkPassword, setLinkPassword] = useState("");
  const [peopleExpirationMode, setPeopleExpirationMode] = useState<ExpirationMode>("never");
  const [peopleDuration, setPeopleDuration] = useState("1d");
  const [linkExpirationMode, setLinkExpirationMode] = useState<ExpirationMode>("never");
  const [linkDuration, setLinkDuration] = useState("1d");
  const [createdUrl, setCreatedUrl] = useState("");

  useEffect(() => {
    setPeopleSearch("");
    setPeoplePermission("read");
    setLinkPermission("read");
    setLinkPassword("");
    setPeopleExpirationMode("never");
    setPeopleDuration("1d");
    setLinkExpirationMode("never");
    setLinkDuration("1d");
    setCreatedUrl("");
  }, [file?.id]);

  const grantsQuery = useQuery({
    ...$api.queryOptions("get", "/v1/files/{fileId}/grants", { params: { path: { fileId } } }),
    enabled: Boolean(file),
  });
  const linksQuery = useQuery({
    ...$api.queryOptions("get", "/v1/files/{fileId}/shares", {
      params: { path: { fileId }, query: { limit: 200 } },
    }),
    enabled: Boolean(file),
  });
  const usersQuery = useQuery({
    ...$api.queryOptions("get", "/v1/users/search", {
      params: { query: { search: deferredPeopleSearch || "_" } },
    }),
    enabled: Boolean(file) && deferredPeopleSearch.length >= 2,
  });

  const createGrant = $api.useMutation("post", "/v1/files/{fileId}/grants");
  const updateGrant = $api.useMutation("patch", "/v1/grants/{grantId}");
  const revokeGrant = $api.useMutation("delete", "/v1/grants/{grantId}");
  const createLink = $api.useMutation("post", "/v1/files/{fileId}/shares");
  const revokeLink = $api.useMutation("delete", "/v1/shares/{shareId}");

  const refreshGrants = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/files/{fileId}/grants", {
        params: { path: { fileId } },
      }).queryKey,
    });
  const refreshLinks = () =>
    getQueryClient().invalidateQueries({
      queryKey: $api.queryOptions("get", "/v1/files/{fileId}/shares", {
        params: { path: { fileId } },
      }).queryKey,
    });

  const addPerson = async (userId: number) => {
    if (!file) return;
    try {
      await createGrant.mutateAsync({
        params: { path: { fileId: file.id } },
        body: {
          granteeUserId: userId,
          permission: peoplePermission,
          expiresAt: expirationDate(peopleExpirationMode, peopleDuration),
        },
      });
      setPeopleSearch("");
      await refreshGrants();
      toast.success("Access granted");
    } catch (error) {
      toast.error("Access could not be granted", { description: userMessage(error) });
    }
  };

  const createPublicLink = async () => {
    if (!file) return;
    try {
      const result = await createLink.mutateAsync({
        params: {
          path: { fileId: file.id },
          header: { "Idempotency-Key": newIdempotencyKey() },
        },
        body: {
          password: linkPassword.trim() || undefined,
          permission: linkPermission,
          expiresAt: expirationDate(linkExpirationMode, linkDuration),
        },
      });
      const publicUrl = new URL(result.publicUrl, window.location.origin).toString();
      setCreatedUrl(publicUrl);
      await copyText(publicUrl);
      await refreshLinks();
      toast.success("Public link created and copied");
    } catch (error) {
      toast.error("Public link could not be created", { description: userMessage(error) });
    }
  };

  return (
    <AppDialog
      open={Boolean(file)}
      onOpenChange={onOpenChange}
      title={file ? `Share ${file.name}` : "Share item"}
      description="Grant access to another Teldrive user or create a public link."
      size="lg"
      footer={
        <Button variant="secondary" onPress={() => onOpenChange(false)}>
          Close
        </Button>
      }
    >
      <div className="grid gap-6">
        <section className="grid gap-3">
          <div>
            <h3 className="text-sm font-semibold">People</h3>
            <p className="mt-1 text-xs text-muted">
              Access applies to this item and its descendants.
            </p>
          </div>
          <PermissionPicker value={peoplePermission} onChange={setPeoplePermission} />
          <ExpirationPicker
            mode={peopleExpirationMode}
            duration={peopleDuration}
            onModeChange={setPeopleExpirationMode}
            onDurationChange={setPeopleDuration}
          />
          <TextField value={peopleSearch} onChange={setPeopleSearch}>
            <Label>Add a user</Label>
            <Input placeholder="Search by name, username, or Telegram ID" />
          </TextField>
          {deferredPeopleSearch.length >= 2 ? (
            usersQuery.isPending ? (
              <div className="flex justify-center py-3">
                <Spinner size="sm" />
              </div>
            ) : usersQuery.data?.length ? (
              <div className="grid gap-1 rounded-xl border border-border p-1">
                {usersQuery.data.map((user) => (
                  <Button
                    key={user.userId}
                    variant="ghost"
                    className="h-auto justify-start px-3 py-2 text-left"
                    isDisabled={createGrant.isPending}
                    onPress={() => void addPerson(user.userId)}
                  >
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-medium">
                        {user.displayName?.trim() || user.username?.trim() || `User ${user.userId}`}
                      </span>
                      <span className="block truncate text-xs text-muted">
                        {user.username ? `@${user.username} · ` : ""}Telegram ID {user.userId}
                      </span>
                    </span>
                  </Button>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted">No users found.</p>
            )
          ) : null}

          <div className="grid min-h-12 content-start gap-2">
            {grantsQuery.isPending ? (
              <div className="flex justify-center py-3">
                <Spinner size="sm" />
              </div>
            ) : grantsQuery.data?.length ? (
              grantsQuery.data.map((grant) => (
                <div
                  key={grant.id}
                  className="flex items-center gap-3 rounded-xl border border-border px-3 py-2"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">
                      {grant.granteeDisplayName?.trim() ||
                        grant.granteeUsername?.trim() ||
                        `User ${grant.granteeUserId}`}
                    </p>
                    <p className="truncate text-xs text-muted">
                      {grant.granteeUsername ? `@${grant.granteeUsername} · ` : ""}Telegram ID{" "}
                      {grant.granteeUserId}
                      {grant.expiresAt ? ` · Expires ${new Date(grant.expiresAt).toLocaleString()}` : " · No expiration"}
                    </p>
                  </div>
                  <Chip variant="tertiary">{grant.permission}</Chip>
                  <Button
                    size="sm"
                    variant="ghost"
                    isDisabled={updateGrant.isPending}
                    onPress={async () => {
                      try {
                        await updateGrant.mutateAsync({
                          params: { path: { grantId: grant.id } },
                          body: {
                            permission: grant.permission === "read" ? "edit" : "read",
                            clearExpiresAt: false,
                          },
                        });
                        await refreshGrants();
                      } catch (error) {
                        toast.error("Permission could not be changed", {
                          description: userMessage(error),
                        });
                      }
                    }}
                  >
                    Make {grant.permission === "read" ? "editor" : "viewer"}
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    isDisabled={revokeGrant.isPending}
                    onPress={async () => {
                      try {
                        await revokeGrant.mutateAsync({ params: { path: { grantId: grant.id } } });
                        await refreshGrants();
                        toast.success("Access removed");
                      } catch (error) {
                        toast.error("Access could not be removed", {
                          description: userMessage(error),
                        });
                      }
                    }}
                  >
                    Remove
                  </Button>
                </div>
              ))
            ) : (
              <p className="flex min-h-12 items-center text-xs text-muted">
                No Teldrive users have access yet.
              </p>
            )}
          </div>
        </section>

        <section className="grid gap-3 border-t border-border pt-5">
          <div>
            <h3 className="text-sm font-semibold">Public links</h3>
            <p className="mt-1 text-xs text-muted">
              Anyone with the link can use the selected permission. Password protection is optional.
            </p>
          </div>
          <PermissionPicker value={linkPermission} onChange={setLinkPermission} />
          <ExpirationPicker
            mode={linkExpirationMode}
            duration={linkDuration}
            onModeChange={setLinkExpirationMode}
            onDurationChange={setLinkDuration}
          />
          <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
            <TextField value={linkPassword} onChange={setLinkPassword} className="min-w-0 flex-1">
              <Label>Optional password</Label>
              <Input type="password" placeholder="Leave empty for no password" />
            </TextField>
            <Button
              variant="primary"
              isDisabled={createLink.isPending}
              onPress={() => void createPublicLink()}
            >
              Create and copy link
            </Button>
          </div>
          {createdUrl ? (
            <div className="flex gap-2 rounded-xl border border-border bg-default/20 p-3">
              <Input readOnly value={createdUrl} className="min-w-0 flex-1" />
              <Button
                size="sm"
                variant="secondary"
                onPress={async () => {
                  try {
                    await copyText(createdUrl);
                    toast.success("Link copied");
                  } catch (error) {
                    toast.error("Link could not be copied", { description: userMessage(error) });
                  }
                }}
              >
                Copy
              </Button>
            </div>
          ) : null}
          <div className="grid min-h-12 content-start gap-2">
            {linksQuery.isPending ? (
              <div className="flex justify-center py-3">
                <Spinner size="sm" />
              </div>
            ) : linksQuery.data?.items.length ? (
              linksQuery.data.items.map((link) => (
                <div
                  key={link.id}
                  className="flex items-center gap-3 rounded-xl border border-border px-3 py-2"
                >
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium">Public {link.permission} link</p>
                    <p className="text-xs text-muted">
                      {link.passwordProtected ? "Password protected · " : ""}
                      {link.downloadCount} download{link.downloadCount === 1 ? "" : "s"}
                      {link.expiresAt ? ` · Expires ${new Date(link.expiresAt).toLocaleString()}` : " · No expiration"}
                    </p>
                  </div>
                  <Chip variant="tertiary">{link.permission}</Chip>
                  <Button
                    size="sm"
                    variant="danger"
                    isDisabled={revokeLink.isPending}
                    onPress={async () => {
                      try {
                        await revokeLink.mutateAsync({ params: { path: { shareId: link.id } } });
                        await refreshLinks();
                        toast.success("Public link revoked");
                      } catch (error) {
                        toast.error("Public link could not be revoked", {
                          description: userMessage(error),
                        });
                      }
                    }}
                  >
                    Revoke
                  </Button>
                </div>
              ))
            ) : (
              <p className="flex min-h-12 items-center text-xs text-muted">
                No public links exist for this item.
              </p>
            )}
          </div>
        </section>
      </div>
    </AppDialog>
  );
}

function PermissionPicker({
  value,
  onChange,
}: {
  value: Permission;
  onChange: (value: Permission) => void;
}) {
  return (
    <Select
      aria-label="Share permission"
      className="w-32"
      selectedKey={value}
      onSelectionChange={(key) => onChange(String(key) as Permission)}
    >
      <Select.Trigger className="h-9 min-h-9 py-1.5">
        <Select.Value>{value === "read" ? "Viewer" : "Editor"}</Select.Value>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          <ListBox.Item id="read" textValue="Viewer">
            Viewer
          </ListBox.Item>
          <ListBox.Item id="edit" textValue="Editor">
            Editor
          </ListBox.Item>
        </ListBox>
      </Select.Popover>
    </Select>
  );
}


function ExpirationPicker({
  mode,
  duration,
  onModeChange,
  onDurationChange,
}: {
  mode: ExpirationMode;
  duration: string;
  onModeChange: (value: ExpirationMode) => void;
  onDurationChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
      <Select
        aria-label="Expiration"
        className="w-40"
        selectedKey={mode}
        onSelectionChange={(key) => onModeChange(String(key) as ExpirationMode)}
      >
        <Select.Trigger className="h-9 min-h-9 py-1.5">
          <Select.Value>{mode === "never" ? "No expiration" : "Custom"}</Select.Value>
          <Select.Indicator />
        </Select.Trigger>
        <Select.Popover>
          <ListBox>
            <ListBox.Item id="never" textValue="No expiration">
              No expiration
            </ListBox.Item>
            <ListBox.Item id="custom" textValue="Custom">
              Custom
            </ListBox.Item>
          </ListBox>
        </Select.Popover>
      </Select>
      {mode === "custom" ? (
        <TextField value={duration} onChange={onDurationChange} className="min-w-0 flex-1">
          <Label>Expires after</Label>
          <Input placeholder="1h, 1d, 1w, 1y" />
        </TextField>
      ) : null}
    </div>
  );
}

function expirationDate(mode: ExpirationMode, duration: string): string | undefined {
  if (mode === "never") return undefined;
  const milliseconds = parseDuration(duration);
  const expiresAt = new Date(Date.now() + milliseconds);
  if (!Number.isFinite(expiresAt.getTime())) throw new Error("Expiration duration is too large");
  return expiresAt.toISOString();
}

function parseDuration(input: string): number {
  const value = input.trim();
  if (!value) throw new Error("Enter an expiration duration such as 1h, 1d, or 1y");

  const pattern = /(\d+(?:\.\d+)?)(ms|s|m|h|d|w|M|y)/g;
  let total = 0;
  let offset = 0;
  for (const match of value.matchAll(pattern)) {
    if (match.index !== offset) throw new Error("Invalid expiration duration");
    total += Number(match[1]) * DURATION_UNITS[match[2]];
    offset = match.index + match[0].length;
  }
  if (offset !== value.length || !Number.isFinite(total) || total <= 0) {
    throw new Error("Invalid expiration duration. Try 1h, 1d, 1w, or 1y");
  }
  return total;
}
