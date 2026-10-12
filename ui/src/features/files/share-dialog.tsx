import {
  Button,
  Chip,
  Dropdown,
  Input,
  Label,
  ListBox,
  Select,
  Spinner,
  TextField,
} from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { useDeferredValue, useEffect, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { copyText } from "@/features/files/download";
import { newIdempotencyKey } from "@/features/shared/idempotency";
import { getQueryClient } from "@/lib/queryClient";
import { useAppLocale } from "@/lib/locale";

type Permission = "read" | "edit";
type ExpirationMode = "never" | "1h" | "1d" | "7d" | "30d" | "custom";

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
  const { t } = useAppLocale();

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
    ...$api.queryOptions("get", "/v1/files/{fileId}/grants", {
      params: { path: { fileId } },
    }),
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
          expiresAt: expirationDate(peopleExpirationMode, peopleDuration, t),
        },
      });
      setPeopleSearch("");
      await refreshGrants();
      toast.success(t("share.granted"));
    } catch (error) {
      toast.error(t("share.grantFailed"), {
        description: userMessage(error),
      });
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
          expiresAt: expirationDate(linkExpirationMode, linkDuration, t),
        },
      });
      const publicUrl = new URL(result.publicUrl, window.location.origin).toString();
      setCreatedUrl(publicUrl);
      await copyText(publicUrl);
      await refreshLinks();
      toast.success(t("share.linkCreated"));
    } catch (error) {
      toast.error(t("share.linkCreateFailed"), {
        description: userMessage(error),
      });
    }
  };

  return (
    <AppDialog
      open={Boolean(file)}
      onOpenChange={onOpenChange}
      title={file ? t("share.titleName", { name: file.name }) : t("share.titleItem")}
      description={t("share.description")}
      size="lg"
      className="sm:w-[min(92vw,46rem)]"
      bodyClassName="py-4"
      footer={
        <Button variant="secondary" onPress={() => onOpenChange(false)}>
          {t("share.close")}
        </Button>
      }
    >
      <div className="grid gap-5">
        <section className="grid gap-4 rounded-2xl border border-border bg-default/10 p-4">
          <div>
            <h3 className="text-sm font-semibold">{t("share.people")}</h3>
            <p className="mt-1 text-xs text-muted">{t("share.peopleHint")}</p>
          </div>

          <TextField value={peopleSearch} onChange={setPeopleSearch}>
            <Label>{t("share.addUser")}</Label>
            <Input placeholder={t("share.searchPlaceholder")} />
          </TextField>

          <div className="grid gap-3 sm:grid-cols-2">
            <ControlField label={t("share.permission")}>
              <PermissionPicker value={peoplePermission} onChange={setPeoplePermission} />
            </ControlField>
            <ControlField label={t("share.expiration")}>
              <ExpirationPicker
                mode={peopleExpirationMode}
                duration={peopleDuration}
                onModeChange={setPeopleExpirationMode}
                onDurationChange={setPeopleDuration}
              />
            </ControlField>
          </div>

          {deferredPeopleSearch.length >= 2 ? (
            usersQuery.isPending ? (
              <div className="flex justify-center py-3">
                <Spinner size="sm" />
              </div>
            ) : usersQuery.data?.length ? (
              <div className="grid gap-1 rounded-xl border border-border bg-surface p-1">
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
                        {user.displayName?.trim() ||
                          user.username?.trim() ||
                          t("share.userFallback", { id: user.userId })}
                      </span>
                      <span className="block truncate text-xs text-muted">
                        {user.username ? `@${user.username} · ` : ""}
                        {t("share.telegramId", { id: user.userId })}
                      </span>
                    </span>
                  </Button>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted">{t("share.noUsers")}</p>
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
                  className="flex items-center gap-3 rounded-xl border border-border bg-surface px-3 py-2.5"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">
                      {grant.granteeDisplayName?.trim() ||
                        grant.granteeUsername?.trim() ||
                        t("share.userFallback", { id: grant.granteeUserId })}
                    </p>
                    <p className="truncate text-xs text-muted">
                      {grant.granteeUsername ? `@${grant.granteeUsername} · ` : ""}
                      {t("share.telegramId", { id: grant.granteeUserId })}
                    </p>
                    <p className="mt-0.5 truncate text-xs text-muted">
                      {grant.expiresAt
                        ? t("share.expiresOn", {
                            date: new Date(grant.expiresAt).toLocaleString(),
                          })
                        : t("share.noExpiration")}
                    </p>
                  </div>
                  <Chip variant="tertiary">
                    {t(grant.permission === "read" ? "share.viewer" : "share.editor")}
                  </Chip>
                  <Dropdown>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="ghost"
                      aria-label={t("share.grantActions")}
                    >
                      <span className="text-lg leading-none">⋯</span>
                    </Button>
                    <Dropdown.Popover className="min-w-48">
                      <Dropdown.Menu
                        aria-label={t("share.grantActions")}
                        onAction={(key) => {
                          if (key === "permission") {
                            void (async () => {
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
                                toast.error(t("share.permFailed"), {
                                  description: userMessage(error),
                                });
                              }
                            })();
                          }
                          if (key === "remove") {
                            void (async () => {
                              try {
                                await revokeGrant.mutateAsync({
                                  params: { path: { grantId: grant.id } },
                                });
                                await refreshGrants();
                                toast.success(t("share.removed"));
                              } catch (error) {
                                toast.error(t("share.removeFailed"), {
                                  description: userMessage(error),
                                });
                              }
                            })();
                          }
                        }}
                      >
                        <Dropdown.Item id="permission" textValue={t("share.changePermission")}>
                          <Label>
                            {t(
                              grant.permission === "read" ? "share.makeEditor" : "share.makeViewer",
                            )}
                          </Label>
                        </Dropdown.Item>
                        <Dropdown.Item id="remove" textValue={t("share.removeAccess")}>
                          <Label>{t("share.removeAccess")}</Label>
                        </Dropdown.Item>
                      </Dropdown.Menu>
                    </Dropdown.Popover>
                  </Dropdown>
                </div>
              ))
            ) : (
              <p className="flex min-h-12 items-center text-xs text-muted">{t("share.noGrants")}</p>
            )}
          </div>
        </section>

        <section className="grid gap-4 rounded-2xl border border-border bg-default/10 p-4">
          <div>
            <h3 className="text-sm font-semibold">{t("share.links")}</h3>
            <p className="mt-1 text-xs text-muted">{t("share.linksHint")}</p>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <ControlField label={t("share.permission")}>
              <PermissionPicker value={linkPermission} onChange={setLinkPermission} />
            </ControlField>
            <ControlField label={t("share.expiration")}>
              <ExpirationPicker
                mode={linkExpirationMode}
                duration={linkDuration}
                onModeChange={setLinkExpirationMode}
                onDurationChange={setLinkDuration}
              />
            </ControlField>
          </div>

          <TextField value={linkPassword} onChange={setLinkPassword}>
            <Label>{t("share.password")}</Label>
            <Input type="password" placeholder={t("share.passwordPlaceholder")} />
          </TextField>

          <div>
            <Button
              variant="primary"
              isDisabled={createLink.isPending}
              onPress={() => void createPublicLink()}
            >
              {t("share.createLink")}
            </Button>
          </div>

          {createdUrl ? (
            <div className="flex gap-2 rounded-xl border border-border bg-surface p-3">
              <Input readOnly value={createdUrl} className="min-w-0 flex-1" />
              <Button
                size="sm"
                variant="secondary"
                onPress={async () => {
                  try {
                    await copyText(createdUrl);
                    toast.success(t("share.linkCopied"));
                  } catch (error) {
                    toast.error(t("share.linkCopyFailed"), {
                      description: userMessage(error),
                    });
                  }
                }}
              >
                {t("share.copy")}
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
                  className="flex items-center gap-3 rounded-xl border border-border bg-surface px-3 py-2.5"
                >
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium">
                      {t("share.linkTitle", {
                        role: t(link.permission === "read" ? "share.viewer" : "share.editor"),
                      })}
                    </p>
                    <p className="truncate text-xs text-muted">
                      {link.passwordProtected ? `${t("share.protected")} · ` : ""}
                      {t(link.downloadCount === 1 ? "share.downloadsOne" : "share.downloadsMany", {
                        count: link.downloadCount,
                      })}
                      {link.expiresAt
                        ? ` · ${t("share.expiresOn", { date: new Date(link.expiresAt).toLocaleString() })}`
                        : ` · ${t("share.noExpiration")}`}
                    </p>
                  </div>
                  <Chip variant="tertiary">
                    {t(link.permission === "read" ? "share.viewer" : "share.editor")}
                  </Chip>
                  <Dropdown>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="ghost"
                      aria-label={t("share.linkActions")}
                    >
                      <span className="text-lg leading-none">⋯</span>
                    </Button>
                    <Dropdown.Popover className="min-w-40">
                      <Dropdown.Menu
                        aria-label={t("share.linkActions")}
                        onAction={(key) => {
                          if (key !== "revoke") return;
                          void (async () => {
                            try {
                              await revokeLink.mutateAsync({
                                params: { path: { shareId: link.id } },
                              });
                              await refreshLinks();
                              toast.success(t("share.linkRevoked"));
                            } catch (error) {
                              toast.error(t("share.linkRevokeFailed"), {
                                description: userMessage(error),
                              });
                            }
                          })();
                        }}
                      >
                        <Dropdown.Item id="revoke" textValue={t("share.revokeLink")}>
                          <Label>{t("share.revokeLink")}</Label>
                        </Dropdown.Item>
                      </Dropdown.Menu>
                    </Dropdown.Popover>
                  </Dropdown>
                </div>
              ))
            ) : (
              <p className="flex min-h-12 items-center text-xs text-muted">{t("share.noLinks")}</p>
            )}
          </div>
        </section>
      </div>
    </AppDialog>
  );
}

function ControlField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <span className="text-xs font-medium text-muted">{label}</span>
      {children}
    </div>
  );
}

function PermissionPicker({
  value,
  onChange,
}: {
  value: Permission;
  onChange: (value: Permission) => void;
}) {
  const { t } = useAppLocale();
  return (
    <Select
      aria-label={t("share.permissionAria")}
      className="w-full"
      selectedKey={value}
      onSelectionChange={(key) => onChange(String(key) as Permission)}
    >
      <Select.Trigger className="h-9 min-h-9 py-1.5">
        <Select.Value>{t(value === "read" ? "share.viewer" : "share.editor")}</Select.Value>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          <ListBox.Item id="read" textValue={t("share.viewer")}>
            {t("share.viewer")}
          </ListBox.Item>
          <ListBox.Item id="edit" textValue={t("share.editor")}>
            {t("share.editor")}
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
  const { t } = useAppLocale();
  return (
    <div className="grid gap-2">
      <Select
        aria-label={t("share.expirationAria")}
        className="w-full"
        selectedKey={mode}
        onSelectionChange={(key) => onModeChange(String(key) as ExpirationMode)}
      >
        <Select.Trigger className="h-9 min-h-9 py-1.5">
          <Select.Value>{expirationLabel(mode, t)}</Select.Value>
          <Select.Indicator />
        </Select.Trigger>
        <Select.Popover>
          <ListBox>
            <ListBox.Item id="never" textValue={t("share.noExpiration")}>
              {t("share.noExpiration")}
            </ListBox.Item>
            <ListBox.Item id="1h" textValue={t("share.expHour")}>
              {t("share.expHour")}
            </ListBox.Item>
            <ListBox.Item id="1d" textValue={t("share.expDay")}>
              {t("share.expDay")}
            </ListBox.Item>
            <ListBox.Item id="7d" textValue={t("share.exp7")}>
              {t("share.exp7")}
            </ListBox.Item>
            <ListBox.Item id="30d" textValue={t("share.exp30")}>
              {t("share.exp30")}
            </ListBox.Item>
            <ListBox.Item id="custom" textValue={t("share.expCustom")}>
              {t("share.expCustomMore")}
            </ListBox.Item>
          </ListBox>
        </Select.Popover>
      </Select>
      {mode === "custom" ? (
        <TextField value={duration} onChange={onDurationChange}>
          <Input aria-label={t("share.customAria")} placeholder={t("share.customPlaceholder")} />
        </TextField>
      ) : null}
    </div>
  );
}

function expirationLabel(mode: ExpirationMode, t: (key: string) => string) {
  if (mode === "never") return t("share.noExpiration");
  if (mode === "1h") return t("share.expHour");
  if (mode === "1d") return t("share.expDay");
  if (mode === "7d") return t("share.exp7");
  if (mode === "30d") return t("share.exp30");
  return t("share.expCustom");
}

function expirationDate(
  mode: ExpirationMode,
  duration: string,
  t: (key: string) => string,
): string | undefined {
  if (mode === "never") return undefined;
  const milliseconds = parseDuration(mode === "custom" ? duration : mode, t);
  const expiresAt = new Date(Date.now() + milliseconds);
  if (!Number.isFinite(expiresAt.getTime())) throw new Error(t("share.tooLarge"));
  return expiresAt.toISOString();
}

function parseDuration(input: string, t: (key: string) => string): number {
  const value = input.trim();
  if (!value) throw new Error(t("share.enterDuration"));

  const pattern = /(\d+(?:\.\d+)?)(ms|s|m|h|d|w|M|y)/g;
  let total = 0;
  let offset = 0;
  for (const match of value.matchAll(pattern)) {
    if (match.index !== offset) throw new Error(t("share.invalidDuration"));
    total += Number(match[1]) * DURATION_UNITS[match[2]];
    offset = match.index + match[0].length;
  }
  if (offset !== value.length || !Number.isFinite(total) || total <= 0) {
    throw new Error(t("share.invalidHint"));
  }
  return total;
}
