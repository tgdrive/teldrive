import { Button, Card, Chip, Spinner } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { $api, fetchClient } from "@/api/client";
import { userMessage } from "@/api/errors";
import type { FileCategory, FileEntry } from "@/api/types";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EmptyState, Page, PageContent, PageHeader } from "@/components/page";
import { FileTypeIcon } from "@/features/files/file-type-icon";
import { useFileActions } from "@/features/files/mutations";
import { useInfiniteFilePages } from "@/features/files/queries";
import RefreshIcon from "~icons/gravity-ui/arrow-rotate-left";
import RestoreIcon from "~icons/gravity-ui/arrow-rotate-left";
import FolderIcon from "~icons/gravity-ui/folder";
import TrashIcon from "~icons/gravity-ui/trash-bin";

export const Route = createFileRoute("/trash")({
  component: () => <QuarantinePage />,
  pendingComponent: () => (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  ),
});

export function QuarantinePage({ spam = false }: { spam?: boolean }) {
  const title = spam ? "Spam" : "Papelera";
  const [category, setCategory] = useState<FileCategory | "folder" | "">("");
  const [after, setAfter] = useState("");
  const [before, setBefore] = useState("");
  const [view, setView] = useState<"list" | "grid">(() =>
    localStorage.getItem("teldrive-v2-trash-view") === "grid" ? "grid" : "list",
  );
  const validRange = !after || !before || after <= before;
  const query = useInfiniteFilePages(
    {
      path: "/",
      sort: "updatedAt",
      order: "desc",
      view: "list",
      category: category && category !== "folder" ? category : undefined,
      kind: category === "folder" ? "folder" : undefined,
      updatedAfter: after ? new Date(`${after}T00:00:00`).toISOString() : undefined,
      updatedBefore: before ? new Date(`${before}T23:59:59.999`).toISOString() : undefined,
    },
    spam ? "spam" : "trashed",
    validRange,
  );
  const incoming = $api.useQuery("get", "/v1/shared/spam", {}, { enabled: spam });
  const incomingIds = new Set(incoming.data?.map((file) => file.id) ?? []);
  const incomingFiles = (incoming.data ?? []).filter((file) => {
    if (!validRange) return false;
    if (category === "folder" && file.kind !== "folder") return false;
    if (category && category !== "folder" && incomingCategory(file) !== category) return false;
    const modified = new Date(file.updatedAt).getTime();
    return (
      (!after || modified >= new Date(`${after}T00:00:00`).getTime()) &&
      (!before || modified <= new Date(`${before}T23:59:59.999`).getTime())
    );
  });
  const files = [
    ...(query.data?.pages.flatMap((page) => page.items) ?? []),
    ...(spam ? incomingFiles : []),
  ];
  const actions = useFileActions();
  const [incomingPending, setIncomingPending] = useState(false);
  const pending = actions.pending || incomingPending;
  const refresh = () => Promise.all([query.refetch(), ...(spam ? [incoming.refetch()] : [])]);
  const [purging, setPurging] = useState<FileEntry>();
  const [cleaningTrash, setCleaningTrash] = useState(false);

  const restore = async (file: FileEntry) => {
    if (pending) return;
    setIncomingPending(true);
    try {
      if (incomingIds.has(file.id)) {
        await fetchClient.DELETE("/v1/shared/spam/{fileId}", {
          params: { path: { fileId: file.id } },
        });
        await incoming.refetch();
      } else await actions.restore(file.id);
      toast.success(`${file.name} restaurado`);
    } catch (error) {
      toast.error("No se pudo restaurar el archivo", { description: userMessage(error) });
    } finally {
      setIncomingPending(false);
    }
  };

  const purge = async (file: FileEntry) => {
    if (pending) return;
    setIncomingPending(true);
    try {
      if (incomingIds.has(file.id)) {
        await fetchClient.DELETE("/v1/shared/spam/{fileId}/dismiss", {
          params: { path: { fileId: file.id } },
        });
        await incoming.refetch();
      } else await actions.purge(file.id);
      toast.success(
        incomingIds.has(file.id)
          ? `Acceso a ${file.name} retirado`
          : `${file.name} eliminado definitivamente`,
      );
    } catch (error) {
      toast.error("No se pudo eliminar el archivo definitivamente", {
        description: userMessage(error),
      });
    } finally {
      setIncomingPending(false);
    }
  };

  const cleanTrash = async () => {
    try {
      await actions.cleanTrash();
      toast.success("Papelera vaciada");
    } catch (error) {
      toast.error("No se pudo vaciar la papelera", { description: userMessage(error) });
    }
  };

  return (
    <Page>
      <PageHeader
        title={title}
        description={
          spam
            ? "Elementos puestos en cuarentena durante 30 días. En los recibidos se retira tu acceso sin borrar el original del propietario. En los propios se retiran los enlaces; restaurarlos no vuelve a compartirlos."
            : "Restaura los elementos eliminados o elimínalos definitivamente de Teldrive."
        }
        actions={
          <div className="flex items-center gap-2">
            {!spam ? (
              <Button
                size="sm"
                variant="danger"
                isDisabled={pending}
                onPress={() => setCleaningTrash(true)}
              >
                <TrashIcon className="size-4" /> Vaciar papelera
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="tertiary"
              isDisabled={pending}
              onPress={() => void refresh()}
            >
              <RefreshIcon className="size-4" /> Actualizar
            </Button>
          </div>
        }
      />
      <PageContent>
        <div className="mb-4 flex flex-wrap items-end gap-3">
          <label className="space-y-1 text-sm">
            <span className="block">Tipo</span>
            <select
              aria-label="Tipo de archivo"
              className="rounded-lg border border-border bg-surface p-2"
              value={category}
              onChange={(event) => setCategory(event.target.value as typeof category)}
            >
              <option value="">Todos los tipos</option>
              <option value="folder">Carpetas</option>
              <option value="image">Imágenes</option>
              <option value="video">Vídeos</option>
              <option value="audio">Audio</option>
              <option value="document">Documentos</option>
              <option value="archive">Archivos comprimidos</option>
              <option value="other">Otros</option>
            </select>
          </label>
          <label className="space-y-1 text-sm">
            <span className="block">Modificado desde</span>
            <input
              aria-label="Modificado desde"
              className="rounded-lg border border-border bg-surface p-2"
              type="date"
              value={after}
              onChange={(event) => setAfter(event.target.value)}
            />
          </label>
          <label className="space-y-1 text-sm">
            <span className="block">Modificado hasta</span>
            <input
              aria-label="Modificado hasta"
              className="rounded-lg border border-border bg-surface p-2"
              type="date"
              value={before}
              onChange={(event) => setBefore(event.target.value)}
            />
          </label>
          <Button
            variant="tertiary"
            onPress={() => {
              setCategory("");
              setAfter("");
              setBefore("");
            }}
          >
            Borrar filtros
          </Button>
          <div className="ml-auto flex gap-2">
            {(["list", "grid"] as const).map((choice, index) => (
              <Button
                key={choice}
                aria-label={["Vista de lista", "Vista de cuadrícula"][index]}
                aria-pressed={view === choice}
                variant={view === choice ? "primary" : "secondary"}
                onPress={() => {
                  setView(choice);
                  localStorage.setItem("teldrive-v2-trash-view", choice);
                }}
              >
                {["Lista", "Cuadrícula"][index]}
              </Button>
            ))}
          </div>
        </div>
        {!validRange ? (
          <p role="alert" className="mb-4 text-danger">
            La fecha inicial debe ser anterior a la final.
          </p>
        ) : null}
        {query.isError || (spam && incoming.isError) ? (
          <p role="alert" className="mb-4 text-danger">
            No se pudo consultar {title}.{" "}
            <Button variant="secondary" onPress={() => void refresh()}>
              Reintentar
            </Button>
          </p>
        ) : null}
        {(query.isPending || (spam && incoming.isPending)) && validRange ? (
          <Spinner aria-label={`Cargando ${title}`} />
        ) : files.length === 0 && !query.isError && !(spam && incoming.isError) && validRange ? (
          <EmptyState
            title={spam ? "No hay elementos en Spam" : "La papelera está vacía"}
            description={
              category || after || before
                ? "No hay elementos que coincidan con los filtros."
                : spam
                  ? "Los elementos marcados como spam aparecerán aquí."
                  : "Los archivos y carpetas eliminados aparecerán aquí."
            }
          />
        ) : (
          <Card
            className={
              view === "grid"
                ? "grid grid-cols-1 gap-3 border border-border bg-surface/80 p-3 sm:grid-cols-2 xl:grid-cols-3"
                : "gap-0 overflow-hidden border border-border bg-surface/80 shadow-sm"
            }
          >
            {files.map((file) => {
              return (
                <div
                  key={file.id}
                  className={
                    view === "grid"
                      ? "grid gap-3 rounded-xl border border-border p-4"
                      : "grid min-h-16 gap-3 border-b border-border px-4 py-3 last:border-b-0 sm:grid-cols-[minmax(0,1fr)_5rem_auto] sm:items-center"
                  }
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-default/20 text-muted">
                      {file.kind === "folder" ? (
                        <FolderIcon className="size-4" />
                      ) : (
                        <FileTypeIcon file={file} className="size-4" />
                      )}
                    </div>
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{file.name}</p>
                      <p className="mt-0.5 text-xs text-muted">
                        {spam ? "Modificado" : "Eliminado"}{" "}
                        {new Date(file.updatedAt).toLocaleString("es-ES")}
                      </p>
                    </div>
                  </div>
                  <Chip size="sm" variant="tertiary" className="w-fit capitalize">
                    {file.kind === "folder" ? "Carpeta" : "Archivo"}
                  </Chip>
                  <div className="flex justify-end gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      isDisabled={pending}
                      onPress={() => void restore(file)}
                    >
                      <RestoreIcon className="size-4" /> {spam ? "No es spam" : "Restaurar"}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      isDisabled={pending}
                      onPress={() => setPurging(file)}
                    >
                      <TrashIcon className="size-4" />{" "}
                      {incomingIds.has(file.id) ? "Retirar acceso" : "Eliminar definitivamente"}
                    </Button>
                  </div>
                </div>
              );
            })}
          </Card>
        )}
        {query.hasNextPage ? (
          <div className="mt-4 flex justify-center">
            <Button
              isDisabled={query.isFetchingNextPage}
              onPress={() => void query.fetchNextPage()}
            >
              {query.isFetchingNextPage ? "Cargando…" : "Mostrar más"}
            </Button>
          </div>
        ) : null}
      </PageContent>

      <ConfirmDialog
        open={Boolean(purging)}
        onOpenChange={(open) => {
          if (!open) setPurging(undefined);
        }}
        title={
          purging && incomingIds.has(purging.id)
            ? "¿Retirar tu acceso a este elemento?"
            : "¿Eliminar este elemento definitivamente?"
        }
        message="Elimina los elementos propios y sus datos en Telegram. En un elemento recibido retira solo tu acceso y conserva el original del propietario. Esta acción no se puede deshacer."
        confirmLabel={
          purging && incomingIds.has(purging.id) ? "Retirar acceso" : "Eliminar definitivamente"
        }
        isPending={pending}
        onConfirm={() => {
          if (!purging) return;
          void purge(purging).finally(() => setPurging(undefined));
        }}
      />

      <ConfirmDialog
        open={cleaningTrash}
        onOpenChange={setCleaningTrash}
        title="¿Vaciar toda la papelera?"
        message="Elimina definitivamente todos los elementos de la papelera, incluidos los ocultos por filtros. Esta acción no se puede deshacer."
        confirmLabel="Vaciar papelera"
        isPending={pending}
        onConfirm={() => {
          void cleanTrash().finally(() => setCleaningTrash(false));
        }}
      />
    </Page>
  );
}

function incomingCategory(file: FileEntry): FileCategory {
  const mime = (file.mimeType ?? "").toLowerCase();
  for (const kind of ["image", "audio", "video"] as const) {
    if (mime.startsWith(`${kind}/`)) return kind;
  }
  if (
    mime.startsWith("text/") ||
    /pdf|json|xml|msword|rtf|ms-excel|ms-powerpoint|officedocument/.test(mime)
  )
    return "document";
  if (/zip|rar|tar|gzip|bzip|xz/.test(mime) || /\.(zip|rar|7z|tar|gz|tgz|bz2|xz)$/i.test(file.name))
    return "archive";
  return "other";
}
