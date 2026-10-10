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
          expiresAt: expirationDate(peopleExpirationMode, peopleDuration),
        },
      });
      setPeopleSearch("");
      await refreshGrants();
      toast.success("Acceso concedido");
    } catch (error) {
      toast.error("No se pudo conceder el acceso", {
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
          expiresAt: expirationDate(linkExpirationMode, linkDuration),
        },
      });
      const publicUrl = new URL(result.publicUrl, window.location.origin).toString();
      setCreatedUrl(publicUrl);
      await copyText(publicUrl);
      await refreshLinks();
      toast.success("Enlace público creado y copiado");
    } catch (error) {
      toast.error("No se pudo crear el enlace público", {
        description: userMessage(error),
      });
    }
  };

  return (
    <AppDialog
      open={Boolean(file)}
      onOpenChange={onOpenChange}
      title={file ? `Compartir ${file.name}` : "Compartir elemento"}
      description="Manage who can access this item and create public links."
      size="lg"
      className="sm:w-[min(92vw,46rem)]"
      bodyClassName="py-4"
      footer={
        <Button variant="secondary" onPress={() => onOpenChange(false)}>
          Cerrar
        </Button>
      }
    >
      <div className="grid gap-5">
        <section className="grid gap-4 rounded-2xl border border-border bg-default/10 p-4">
          <div>
            <h3 className="text-sm font-semibold">Personas con acceso</h3>
            <p className="mt-1 text-xs text-muted">
              El acceso se aplica a este elemento y su contenido.
            </p>
          </div>

          <TextField value={peopleSearch} onChange={setPeopleSearch}>
            <Label>Add a user</Label>
            <Input placeholder="Buscar por nombre, usuario o ID de Telegram" />
          </TextField>

          <div className="grid gap-3 sm:grid-cols-2">
            <ControlField label="Permiso">
              <PermissionPicker value={peoplePermission} onChange={setPeoplePermission} />
            </ControlField>
            <ControlField label="Vencimiento">
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
                        {user.displayName?.trim() || user.username?.trim() || `Usuario ${user.userId}`}
                      </span>
                      <span className="block truncate text-xs text-muted">
                        {user.username ? `@${user.username} · ` : ""}Telegram ID {user.userId}
                      </span>
                    </span>
                  </Button>
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted">No se encontraron usuarios.</p>
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
                        `Usuario ${grant.granteeUserId}`}
                    </p>
                    <p className="truncate text-xs text-muted">
                      {grant.granteeUsername ? `@${grant.granteeUsername} · ` : ""}
                      Telegram ID {grant.granteeUserId}
                    </p>
                    <p className="mt-0.5 truncate text-xs text-muted">
                      {grant.expiresAt
                        ? `Vence ${new Date(grant.expiresAt).toLocaleString()}`
                        : "Sin vencimiento"}
                    </p>
                  </div>
                  <Chip variant="tertiary">
                    {grant.permission === "read" ? "Lector" : "Editor"}
                  </Chip>
                  <Dropdown>
                    <Button isIconOnly size="sm" variant="ghost" aria-label="Acciones de acceso">
                      <span className="text-lg leading-none">⋯</span>
                    </Button>
                    <Dropdown.Popover className="min-w-48">
                      <Dropdown.Menu
                        aria-label="Acciones de acceso"
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
                                toast.error("No se pudo cambiar el permiso", {
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
                                toast.success("Acceso eliminado");
                              } catch (error) {
                                toast.error("No se pudo eliminar el acceso", {
                                  description: userMessage(error),
                                });
                              }
                            })();
                          }
                        }}
                      >
                        <Dropdown.Item id="permission" textValue="Cambiar permiso">
                          <Label>Make {grant.permission === "read" ? "editor" : "viewer"}</Label>
                        </Dropdown.Item>
                        <Dropdown.Item id="remove" textValue="Eliminar acceso">
                          <Label>Eliminar acceso</Label>
                        </Dropdown.Item>
                      </Dropdown.Menu>
                    </Dropdown.Popover>
                  </Dropdown>
                </div>
              ))
            ) : (
              <p className="flex min-h-12 items-center text-xs text-muted">
                No Teldrive users have access yet.
              </p>
            )}
          </div>
        </section>

        <section className="grid gap-4 rounded-2xl border border-border bg-default/10 p-4">
          <div>
            <h3 className="text-sm font-semibold">Enlaces públicos</h3>
            <p className="mt-1 text-xs text-muted">
              Cualquier persona con el enlace tendrá el permiso seleccionado. Puedes protegerlo con una contraseña.
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <ControlField label="Permiso">
              <PermissionPicker value={linkPermission} onChange={setLinkPermission} />
            </ControlField>
            <ControlField label="Vencimiento">
              <ExpirationPicker
                mode={linkExpirationMode}
                duration={linkDuration}
                onModeChange={setLinkExpirationMode}
                onDurationChange={setLinkDuration}
              />
            </ControlField>
          </div>

          <TextField value={linkPassword} onChange={setLinkPassword}>
            <Label>Contraseña</Label>
            <Input type="password" placeholder="Contraseña opcional" />
          </TextField>

          <div>
            <Button
              variant="primary"
              isDisabled={createLink.isPending}
              onPress={() => void createPublicLink()}
            >
              Crear enlace público
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
                    toast.success("Enlace copiado");
                  } catch (error) {
                    toast.error("No se pudo copiar el enlace", {
                      description: userMessage(error),
                    });
                  }
                }}
              >
                Copiar
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
                      Enlace público · {link.permission === "read" ? "Lector" : "Editor"}
                    </p>
                    <p className="truncate text-xs text-muted">
                      {link.passwordProtected ? "Protegido por contraseña · " : ""}
                      {link.downloadCount} descarga
                      {link.downloadCount === 1 ? "" : "s"}
                      {link.expiresAt
                        ? ` · Vence ${new Date(link.expiresAt).toLocaleString()}`
                        : " · Sin vencimiento"}
                    </p>
                  </div>
                  <Chip variant="tertiary">{link.permission === "read" ? "Lector" : "Editor"}</Chip>
                  <Dropdown>
                    <Button isIconOnly size="sm" variant="ghost" aria-label="Acciones del enlace público">
                      <span className="text-lg leading-none">⋯</span>
                    </Button>
                    <Dropdown.Popover className="min-w-40">
                      <Dropdown.Menu
                        aria-label="Acciones del enlace público"
                        onAction={(key) => {
                          if (key !== "revoke") return;
                          void (async () => {
                            try {
                              await revokeLink.mutateAsync({
                                params: { path: { shareId: link.id } },
                              });
                              await refreshLinks();
                              toast.success("Enlace público revocado");
                            } catch (error) {
                              toast.error("No se pudo revocar el enlace público", {
                                description: userMessage(error),
                              });
                            }
                          })();
                        }}
                      >
                        <Dropdown.Item id="revoke" textValue="Revocar enlace">
                          <Label>Revocar enlace</Label>
                        </Dropdown.Item>
                      </Dropdown.Menu>
                    </Dropdown.Popover>
                  </Dropdown>
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
  return (
    <Select
      aria-label="Permiso de acceso"
      className="w-full"
      selectedKey={value}
      onSelectionChange={(key) => onChange(String(key) as Permission)}
    >
      <Select.Trigger className="h-9 min-h-9 py-1.5">
        <Select.Value>{value === "read" ? "Lector" : "Editor"}</Select.Value>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          <ListBox.Item id="read" textValue="Lector">
            Lector
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
    <div className="grid gap-2">
      <Select
        aria-label="Vencimiento"
        className="w-full"
        selectedKey={mode}
        onSelectionChange={(key) => onModeChange(String(key) as ExpirationMode)}
      >
        <Select.Trigger className="h-9 min-h-9 py-1.5">
          <Select.Value>{expirationLabel(mode)}</Select.Value>
          <Select.Indicator />
        </Select.Trigger>
        <Select.Popover>
          <ListBox>
            <ListBox.Item id="never" textValue="Sin vencimiento">
              Sin vencimiento
            </ListBox.Item>
            <ListBox.Item id="1h" textValue="1 hora">
              1 hora
            </ListBox.Item>
            <ListBox.Item id="1d" textValue="1 día">
              1 día
            </ListBox.Item>
            <ListBox.Item id="7d" textValue="7 días">
              7 días
            </ListBox.Item>
            <ListBox.Item id="30d" textValue="30 días">
              30 días
            </ListBox.Item>
            <ListBox.Item id="custom" textValue="Personalizado">
              Personalizado…
            </ListBox.Item>
          </ListBox>
        </Select.Popover>
      </Select>
      {mode === "custom" ? (
        <TextField value={duration} onChange={onDurationChange}>
          <Input aria-label="Duración personalizada" placeholder="1h, 1d12h, 1y" />
        </TextField>
      ) : null}
    </div>
  );
}

function expirationLabel(mode: ExpirationMode) {
  if (mode === "never") return "Sin vencimiento";
  if (mode === "1h") return "1 hora";
  if (mode === "1d") return "1 día";
  if (mode === "7d") return "7 días";
  if (mode === "30d") return "30 días";
  return "Personalizado";
}

function expirationDate(mode: ExpirationMode, duration: string): string | undefined {
  if (mode === "never") return undefined;
  const milliseconds = parseDuration(mode === "custom" ? duration : mode);
  const expiresAt = new Date(Date.now() + milliseconds);
  if (!Number.isFinite(expiresAt.getTime())) throw new Error("La duración del enlace es demasiado larga");
  return expiresAt.toISOString();
}

function parseDuration(input: string): number {
  const value = input.trim();
  if (!value) throw new Error("Introduce una duración como 1h, 1d o 1y");

  const pattern = /(\d+(?:\.\d+)?)(ms|s|m|h|d|w|M|y)/g;
  let total = 0;
  let offset = 0;
  for (const match of value.matchAll(pattern)) {
    if (match.index !== offset) throw new Error("Duración de vencimiento no válida");
    total += Number(match[1]) * DURATION_UNITS[match[2]];
    offset = match.index + match[0].length;
  }
  if (offset !== value.length || !Number.isFinite(total) || total <= 0) {
    throw new Error("Duración no válida. Prueba 1h, 1d, 1w o 1y");
  }
  return total;
}
