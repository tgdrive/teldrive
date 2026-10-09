import { Button, Input, Spinner } from "@heroui/react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { desktopRequest } from "@/lib/desktop";
import { copyText } from "@/features/files/download";
import { AppDialog } from "./dialogs/app-dialog";
import { ConfirmDialog } from "./dialogs/confirm-dialog";
import { SettingsSection } from "./settings-layout";

type Entry = { Name: string; Path: string; Size: number; ModTime: string; IsDir: boolean };
type Local = { name: string; path: string; directory: boolean; size: number; modified: string };
type LocalFolder = { path: string; name: string; items: Local[] };
type Job = {
  id: string;
  label: string;
  mount: boolean;
  status: "running" | "completed" | "cancelled" | "failed";
  exitCode?: number;
  log: string;
};
type Info = {
  configured: boolean;
  encrypted: boolean;
  locked: boolean;
  driverInstalled: boolean;
  version: string;
};

export function RcloneBrowser({ connectionVersion }: { connectionVersion: number }) {
  const [tab, setTab] = useState<"files" | "jobs" | "mount">("files");
  const [info, setInfo] = useState<Info>();
  const [path, setPath] = useState("/");
  const [entries, setEntries] = useState<Entry[]>([]);
  const [selected, setSelected] = useState<Entry>();
  const [local, setLocal] = useState<LocalFolder>();
  const [jobs, setJobs] = useState<Job[]>([]);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [drive, setDrive] = useState("T:");
  const [editor, setEditor] = useState<"mkdir" | "rename">();
  const [name, setName] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [password, setPassword] = useState("");
  const [repeatPassword, setRepeatPassword] = useState("");
  const target = selected ? join(path, selected.Name) : path;
  const execute = async <T,>(method: string, params: unknown = {}): Promise<T | undefined> => {
    setPending(true);
    setError("");
    try {
      return await desktopRequest<T>(method, params);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "La operación de rclone falló.";
      setError(message);
      toast.error(message);
      return undefined;
    } finally {
      setPending(false);
    }
  };
  const browse = useCallback(async () => {
    setPending(true);
    setError("");
    try {
      const data = await desktopRequest<Entry[] | null>("rclone.browse", { path });
      setEntries(data ?? []);
      setSelected(undefined);
      setPage(0);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo abrir la carpeta.");
    } finally {
      setPending(false);
    }
  }, [path]);
  useEffect(() => {
    let active = true;
    desktopRequest<Info>("rclone.info")
      .then((value) => {
        if (active) setInfo(value);
      })
      .catch((cause) => {
        if (active) setError(String(cause.message));
      });
    return () => {
      active = false;
    };
  }, [connectionVersion]);
  useEffect(() => {
    if (info?.configured && !info.locked) void browse();
    else {
      setEntries([]);
      setSelected(undefined);
    }
  }, [browse, info?.configured, info?.locked, connectionVersion]);
  useEffect(() => {
    let active = true;
    const refresh = () =>
      desktopRequest<Job[]>("rclone.jobs")
        .then((value) => {
          if (active) setJobs(value);
        })
        .catch(() => {});
    void refresh();
    const interval = setInterval(refresh, 3000);
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, []);
  const filtered = entries
    .filter((entry) => entry.Name.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
    .sort((a, b) => Number(b.IsDir) - Number(a.IsDir) || a.Name.localeCompare(b.Name, "es"));
  const edit = async () => {
    const clean = name.trim();
    if (!clean || /[\\/\0]/.test(clean) || clean === "." || clean === "..") {
      setError("Introduce un nombre de archivo o carpeta válido.");
      return;
    }
    const result =
      editor === "mkdir"
        ? await execute("rclone.mkdir", { path: join(path, clean) })
        : await execute("rclone.rename", { source: target, target: join(path, clean) });
    if (result) {
      setEditor(undefined);
      setName("");
      await browse();
    }
  };
  const transfer = async (method: "rclone.upload" | "rclone.download") => {
    const id = await execute<string | null>(method, {
      path: method === "rclone.upload" ? path : target,
    });
    if (id) {
      toast.success("Transferencia iniciada");
      setTab("jobs");
    }
  };
  return (
    <SettingsSection
      title="Explorador de rclone"
      description="Explora tu conexión, reproduce multimedia y gestiona transferencias y montajes desde esta ventana."
    >
      <div className="space-y-4 p-4 sm:p-5">
        <fieldset className="flex flex-wrap gap-2" aria-label="Secciones de rclone">
          {(["files", "jobs", "mount"] as const).map((value, index) => (
            <Button
              key={value}
              variant={tab === value ? "primary" : "secondary"}
              onPress={() => setTab(value)}
            >
              {["Archivos", "Transferencias", "Montajes"][index]}
            </Button>
          ))}
        </fieldset>
        {error ? (
          <p role="alert" className="rounded-lg border border-danger/40 p-3 text-sm text-danger">
            {error}
          </p>
        ) : null}
        {!info?.configured ? (
          <p className="text-sm text-muted">
            Introduce tu clave de API y pulsa «Guardar conexión en este equipo» para empezar.
          </p>
        ) : null}
        {info?.configured ? (
          <div className="space-y-3 rounded-xl border border-border p-3">
            <h3 className="font-semibold">Protección de la conexión</h3>
            <p className="text-sm text-muted">
              {info.encrypted
                ? info.locked
                  ? "La configuración está cifrada. Desbloquéala para usar rclone."
                  : "Configuración cifrada y desbloqueada para esta sesión."
                : "La clave de API se guarda en la configuración local. Puedes cifrarla con una contraseña de rclone."}
            </p>
            {!info.encrypted || info.locked ? (
              <div className="flex flex-wrap items-center gap-2">
                <Input
                  type="password"
                  aria-label="Contraseña de rclone"
                  autoComplete="off"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="max-w-xs"
                />
                {!info.encrypted ? (
                  <Input
                    type="password"
                    aria-label="Repetir contraseña de rclone"
                    autoComplete="off"
                    value={repeatPassword}
                    onChange={(event) => setRepeatPassword(event.target.value)}
                    className="max-w-xs"
                  />
                ) : null}
                <Button
                  isDisabled={
                    pending ||
                    !password ||
                    (!info.encrypted &&
                      (password.trim().length < 12 || password !== repeatPassword))
                  }
                  onPress={async () => {
                    const value = await execute<Info>(
                      info.encrypted ? "rclone.unlock" : "rclone.protect",
                      { password },
                    );
                    setPassword("");
                    setRepeatPassword("");
                    if (value) setInfo(value);
                  }}
                >
                  {info.encrypted ? "Desbloquear rclone" : "Cifrar configuración"}
                </Button>
              </div>
            ) : (
              <Button
                variant="secondary"
                isDisabled={pending}
                onPress={async () => {
                  const value = await execute<Info>("rclone.lock");
                  if (value) setInfo(value);
                }}
              >
                Bloquear y detener rclone
              </Button>
            )}
          </div>
        ) : null}
        {tab === "files" && info?.configured && !info.locked ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="secondary"
                isDisabled={pending || path === "/"}
                onPress={() => setPath(parent(path))}
              >
                Subir carpeta
              </Button>
              <Button
                variant="secondary"
                isDisabled={pending}
                onPress={() => {
                  setPath("/");
                  if (path === "/") void browse();
                }}
              >
                Raíz
              </Button>
              <span className="min-w-0 break-all text-sm">teldrive:{path}</span>
              <Button variant="secondary" isDisabled={pending} onPress={() => void browse()}>
                Actualizar
              </Button>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button isDisabled={pending} onPress={() => void transfer("rclone.upload")}>
                Subir archivo local
              </Button>
              <Button
                variant="secondary"
                isDisabled={pending}
                onPress={async () => {
                  const id = await execute<string | null>("rclone.upload-folder", { path });
                  if (id) setTab("jobs");
                }}
              >
                Subir carpeta local
              </Button>
              <Button
                variant="secondary"
                isDisabled={pending}
                onPress={() => void transfer("rclone.download")}
              >
                {selected ? "Descargar selección" : "Descargar carpeta"}
              </Button>
              <Button
                variant="secondary"
                isDisabled={pending}
                onPress={() => {
                  setName("");
                  setEditor("mkdir");
                }}
              >
                Nueva carpeta
              </Button>
              <Button
                variant="secondary"
                isDisabled={pending || !selected}
                onPress={() => {
                  setName(selected?.Name ?? "");
                  setEditor("rename");
                }}
              >
                Renombrar
              </Button>
              <Button
                variant="danger"
                isDisabled={pending || !selected}
                onPress={() => setDeleting(true)}
              >
                Eliminar
              </Button>
            </div>
            <Button
              variant="secondary"
              isDisabled={
                pending ||
                !selected ||
                selected.IsDir ||
                !/\.(mp3|mp4|flac|m4a|mkv|avi|wmv|flv|ogg|opus|wav|webm|mov|aac|aiff|ape|wma|ogv|m4v|m2ts|ts|3gp|mpeg|mpg|vob|mp2|alac)$/i.test(
                  selected.Name,
                )
              }
              onPress={() => void execute("rclone.play", { path: target })}
            >
              Reproducir con mpv integrado
            </Button>
            <div className="grid min-w-0 gap-4 xl:grid-cols-2">
              <div className="min-w-0 rounded-xl border border-border p-3">
                <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                  <h3 className="font-semibold">Teldrive remoto</h3>
                  <Input
                    aria-label="Buscar en rclone"
                    placeholder="Buscar en esta carpeta"
                    value={query}
                    onChange={(event) => {
                      setQuery(event.target.value);
                      setPage(0);
                    }}
                    className="max-w-xs"
                  />
                </div>
                {pending ? <Spinner aria-label="Consultando rclone" /> : null}
                <ul aria-label="Archivos de rclone" className="max-h-96 overflow-y-auto">
                  {filtered.slice(page * 100, (page + 1) * 100).map((entry) => (
                    <li
                      key={entry.Path}
                      className={`flex items-center gap-2 border-b border-border py-2 ${selected?.Path === entry.Path ? "bg-accent/10" : ""}`}
                    >
                      <label className="flex size-11 shrink-0 cursor-pointer items-center justify-center">
                        <input
                          type="radio"
                          name="rclone-selection"
                          aria-label={`Seleccionar ${entry.Name}`}
                          checked={selected?.Path === entry.Path}
                          onChange={() => setSelected(entry)}
                          className="size-5"
                        />
                      </label>
                      <Button
                        variant="ghost"
                        className="h-auto min-w-0 flex-1 justify-start whitespace-normal text-left"
                        isDisabled={pending}
                        onPress={() =>
                          entry.IsDir ? setPath(join(path, entry.Name)) : setSelected(entry)
                        }
                      >
                        <span aria-hidden="true">{entry.IsDir ? "📁" : "📄"}</span>
                        <span className="min-w-0 break-all">
                          {entry.Name}
                          <small className="block text-xs text-muted">
                            {entry.IsDir ? "Carpeta" : bytes(entry.Size)} ·{" "}
                            {new Date(entry.ModTime).toLocaleDateString("es")}
                          </small>
                        </span>
                      </Button>
                    </li>
                  ))}
                </ul>
                {!pending && !filtered.length ? (
                  <p className="py-5 text-sm text-muted">No hay elementos que mostrar.</p>
                ) : null}
                <div className="mt-3 flex items-center justify-between gap-2">
                  <Button
                    variant="secondary"
                    isDisabled={page === 0}
                    onPress={() => setPage(page - 1)}
                  >
                    Anterior
                  </Button>
                  <span className="text-xs">{filtered.length} elementos</span>
                  <Button
                    variant="secondary"
                    isDisabled={(page + 1) * 100 >= filtered.length}
                    onPress={() => setPage(page + 1)}
                  >
                    Siguiente
                  </Button>
                </div>
              </div>
              <div className="min-w-0 rounded-xl border border-border p-3">
                <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                  <h3 className="font-semibold">Archivos de Windows</h3>
                  <Button
                    variant="secondary"
                    isDisabled={pending}
                    onPress={async () => {
                      const value = await execute<LocalFolder | null>("rclone.local-folder");
                      if (value) setLocal(value);
                    }}
                  >
                    Elegir carpeta local
                  </Button>
                </div>
                {local ? (
                  <>
                    <div className="mb-3 flex flex-wrap items-center gap-2">
                      <Button
                        variant="secondary"
                        isDisabled={pending || !local.path}
                        onPress={async () => {
                          const value = await execute<LocalFolder>("rclone.local-browse", {
                            path: local.path.split(/[\\/]/).slice(0, -1).join("/"),
                          });
                          if (value) setLocal(value);
                        }}
                      >
                        Subir carpeta local
                      </Button>
                      <span className="min-w-0 break-all text-xs">{local.name}</span>
                    </div>
                    <div className="max-h-96 overflow-y-auto">
                      {local.items.map((entry) => (
                        <div
                          key={entry.path}
                          className="flex items-center gap-2 border-b border-border py-2"
                        >
                          <span aria-hidden="true">{entry.directory ? "📁" : "📄"}</span>
                          <span className="min-w-0 flex-1 break-all text-sm">
                            {entry.name}
                            <small className="block text-xs text-muted">
                              {entry.directory ? "Carpeta" : bytes(entry.size)}
                            </small>
                          </span>
                          <Button
                            variant="secondary"
                            isDisabled={pending}
                            onPress={async () => {
                              if (entry.directory) {
                                const value = await execute<LocalFolder>("rclone.local-browse", {
                                  path: entry.path,
                                });
                                if (value) setLocal(value);
                              } else {
                                const id = await execute<string>("rclone.upload-local", {
                                  path: entry.path,
                                  destination: path,
                                });
                                if (id) setTab("jobs");
                              }
                            }}
                          >
                            {entry.directory ? "Abrir" : "Subir"}
                          </Button>
                          {entry.directory ? (
                            <Button
                              variant="secondary"
                              isDisabled={pending}
                              onPress={async () => {
                                const id = await execute<string>("rclone.upload-local", {
                                  path: entry.path,
                                  destination: path,
                                });
                                if (id) setTab("jobs");
                              }}
                            >
                              Subir carpeta
                            </Button>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  </>
                ) : (
                  <p className="py-5 text-sm text-muted">
                    El selector de Windows limita el acceso a la carpeta que elijas.
                  </p>
                )}
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="secondary"
                isDisabled={pending}
                onPress={async () => {
                  const value = await execute<{ count: number; bytes: number }>("rclone.size", {
                    path: target,
                  });
                  if (value) toast.info(`${value.count} archivos · ${bytes(value.bytes)}`);
                }}
              >
                Calcular tamaño
              </Button>
              <Button
                variant="secondary"
                isDisabled={pending || !selected}
                onPress={async () => {
                  const value = await execute<string>("rclone.link", { path: target });
                  if (value) {
                    try {
                      await copyText(value);
                      toast.success("Enlace público creado y copiado");
                    } catch (cause) {
                      const message =
                        cause instanceof Error ? cause.message : "No se pudo copiar el enlace.";
                      setError(message);
                      toast.error(message);
                    }
                  }
                }}
              >
                Crear enlace público
              </Button>
              <Button
                variant="secondary"
                onPress={() => {
                  const url = URL.createObjectURL(
                    new Blob([JSON.stringify(entries, null, 2)], { type: "application/json" }),
                  );
                  const anchor = document.createElement("a");
                  anchor.href = url;
                  anchor.download = "rclone-archivos.json";
                  anchor.click();
                  setTimeout(() => URL.revokeObjectURL(url), 1000);
                }}
              >
                Exportar lista
              </Button>
            </div>
          </>
        ) : null}
        {tab === "jobs" ? (
          <div className="space-y-3">
            {jobs.length ? (
              jobs.map((job) => (
                <div key={job.id} className="rounded-xl border border-border p-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <h3 className="font-medium">{job.label}</h3>
                      <p className="text-xs text-muted">
                        {
                          {
                            running: "En curso",
                            completed: "Completada",
                            cancelled: "Detenida",
                            failed: "Fallida",
                          }[job.status]
                        }
                      </p>
                    </div>
                    {job.status !== "running" && !job.mount ? (
                      <Button
                        variant="secondary"
                        isDisabled={pending || info?.locked}
                        onPress={() => void execute("rclone.retry", { id: job.id })}
                      >
                        Reintentar
                      </Button>
                    ) : null}
                    {job.status === "running" ? (
                      <Button
                        variant="danger"
                        isDisabled={pending}
                        onPress={() => void execute("rclone.stop", { id: job.id })}
                      >
                        Detener
                      </Button>
                    ) : null}
                  </div>
                  <details className="mt-3">
                    <summary className="cursor-pointer py-2 text-sm">Progreso y registro</summary>
                    <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-default/30 p-3 text-xs">
                      {job.log || "Esperando la respuesta de rclone…"}
                    </pre>
                  </details>
                </div>
              ))
            ) : (
              <p className="text-sm text-muted">
                Las transferencias y los montajes aparecerán aquí.
              </p>
            )}
          </div>
        ) : null}
        {tab === "mount" ? (
          <div className="space-y-4">
            <p className="text-sm text-muted">
              Monta Teldrive como una unidad de Windows. La caché usa un máximo de 10 GiB; los
              archivos permanecen en Telegram.
            </p>
            <div className="flex flex-wrap items-center gap-3">
              <Input
                aria-label="Letra de unidad"
                value={drive}
                onChange={(event) => setDrive(event.target.value)}
                className="max-w-32"
              />
              <Button
                isDisabled={pending || !info?.configured || info.locked}
                onPress={async () => {
                  const id = await execute<string>("rclone.mount", { drive });
                  if (id) setTab("jobs");
                }}
              >
                Montar unidad
              </Button>
              <Button
                variant="secondary"
                isDisabled={pending}
                onPress={async () => {
                  const value = await execute<Info>("rclone.winfsp");
                  if (value) setInfo(value);
                }}
              >
                Preparar WinFsp integrado
              </Button>
            </div>
            <p className="text-sm">
              {info?.driverInstalled
                ? "WinFsp está preparado."
                : "El controlador requiere la confirmación de Windows al instalarlo."}
            </p>
            <p className="text-sm text-muted">
              Para desmontar, abre Transferencias y detén la operación «Unidad».
            </p>
          </div>
        ) : null}
      </div>
      <AppDialog
        open={Boolean(editor)}
        onOpenChange={(open) => {
          if (!open && !pending) setEditor(undefined);
        }}
        title={editor === "mkdir" ? "Nueva carpeta de rclone" : "Renombrar en rclone"}
        isCloseDisabled={pending}
        footer={
          <Button isDisabled={pending} onPress={() => void edit()}>
            Guardar
          </Button>
        }
      >
        <Input
          aria-label="Nombre en rclone"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </AppDialog>
      <ConfirmDialog
        open={deleting}
        onOpenChange={setDeleting}
        title="¿Eliminar este elemento mediante rclone?"
        message={`Se eliminará «${selected?.Name ?? ""}» y, si es una carpeta, su contenido. Comprueba el elemento antes de continuar.`}
        isPending={pending}
        confirmLabel="Eliminar"
        onConfirm={async () => {
          const value = await execute("rclone.trash", {
            path: target,
            directory: selected?.IsDir ?? false,
          });
          if (value) {
            setDeleting(false);
            await browse();
          }
        }}
      />
    </SettingsSection>
  );
}
function join(path: string, name: string) {
  return `${path.replace(/\/$/, "")}/${name}`;
}
function parent(path: string) {
  return path.split("/").filter(Boolean).slice(0, -1).join("/")
    ? `/${path.split("/").filter(Boolean).slice(0, -1).join("/")}`
    : "/";
}
function bytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const power = Math.min(4, Math.floor(Math.log2(value) / 10));
  return `${(value / 1024 ** power).toLocaleString("es", { maximumFractionDigits: 1 })} ${units[power]}`;
}
