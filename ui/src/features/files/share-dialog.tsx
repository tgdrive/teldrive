import { Button, Chip, Input, Label, Spinner, TextField } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { useDeferredValue, useEffect, useState } from "react";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { getQueryClient } from "@/lib/queryClient";

type Permission = "read" | "edit";

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
  const [createdUrl, setCreatedUrl] = useState("");

  useEffect(() => {
    setPeopleSearch("");
    setPeoplePermission("read");
    setLinkPermission("read");
    setLinkPassword("");
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
        body: { granteeUserId: userId, permission: peoplePermission },
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
        },
      });
      const publicUrl = new URL(result.publicUrl, window.location.origin).toString();
      setCreatedUrl(publicUrl);
      await navigator.clipboard.writeText(publicUrl);
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

          <div className="grid gap-2">
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
              <p className="text-xs text-muted">No Teldrive users have access yet.</p>
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
                onPress={() => {
                  void navigator.clipboard.writeText(createdUrl);
                  toast.success("Link copied");
                }}
              >
                Copy
              </Button>
            </div>
          ) : null}
          <div className="grid gap-2">
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
              <p className="text-xs text-muted">No public links exist for this item.</p>
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
    <fieldset className="flex w-fit rounded-lg border border-border p-1" aria-label="Share permission">
      <Button
        size="sm"
        variant={value === "read" ? "secondary" : "ghost"}
        onPress={() => onChange("read")}
      >
        Viewer
      </Button>
      <Button
        size="sm"
        variant={value === "edit" ? "secondary" : "ghost"}
        onPress={() => onChange("edit")}
      >
        Editor
      </Button>
    </fieldset>
  );
}
