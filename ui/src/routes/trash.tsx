import { Button, Card, Chip, Spinner } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import FolderIcon from "~icons/gravity-ui/folder";
import RefreshIcon from "~icons/gravity-ui/arrow-rotate-left";
import RestoreIcon from "~icons/gravity-ui/arrow-rotate-left";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { userMessage } from "@/api/errors";
import type { FileEntry, FileCategory } from "@/api/types";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EmptyState, Page, PageContent, PageHeader } from "@/components/page";
import { FileTypeIcon } from "@/features/files/file-type-icon";
import { useFileActions } from "@/features/files/mutations";
import { useInfiniteFilePages } from "@/features/files/queries";

export const Route = createFileRoute("/trash")({
  component: TrashPage,
  pendingComponent: () => (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  ),
});

function TrashPage() {
  const [category, setCategory] = useState<FileCategory | "folder" | "">("");
  const [after, setAfter] = useState("");
  const [before, setBefore] = useState("");
  const [view, setView] = useState<"list" | "grid">(() => localStorage.getItem("teldrive-v2-trash-view") === "grid" ? "grid" : "list");
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
    "trashed",
    validRange,
  );
  const files = query.data?.pages.flatMap(page => page.items) ?? [];
  const actions = useFileActions();
  const [purging, setPurging] = useState<FileEntry>();
  const [cleaningTrash, setCleaningTrash] = useState(false);

  const restore = async (file: FileEntry) => {
    try {
      await actions.restore(file.id);
      toast.success(`${file.name} restaurado`);
    } catch (error) {
      toast.error("No se pudo restaurar el archivo", { description: userMessage(error) });
    }
  };

  const purge = async (file: FileEntry) => {
    try {
      await actions.purge(file.id);
      toast.success(`${file.name} eliminado definitivamente`);
    } catch (error) {
      toast.error("No se pudo eliminar el archivo definitivamente", { description: userMessage(error) });
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
        title="Papelera"
        description="Restaura los elementos eliminados o elimínalos definitivamente de Teldrive."
        actions={
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="danger"
              isDisabled={actions.pending}
              onPress={() => setCleaningTrash(true)}
            >
              <TrashIcon className="size-4" /> Vaciar papelera
            </Button>
            <Button
              size="sm"
              variant="tertiary"
              isDisabled={actions.pending}
              onPress={() => void query.refetch()}
            >
              <RefreshIcon className="size-4" /> Actualizar
            </Button>
          </div>
        }
      />
      <PageContent>
        <div className="mb-4 flex flex-wrap items-end gap-3">
          <label className="space-y-1 text-sm"><span className="block">Tipo</span><select aria-label="Tipo de archivo" className="rounded-lg border border-border bg-surface p-2" value={category} onChange={event => setCategory(event.target.value as typeof category)}><option value="">Todos los tipos</option><option value="folder">Carpetas</option><option value="image">Imágenes</option><option value="video">Vídeos</option><option value="audio">Audio</option><option value="document">Documentos</option><option value="archive">Archivos comprimidos</option><option value="other">Otros</option></select></label>
          <label className="space-y-1 text-sm"><span className="block">Modificado desde</span><input aria-label="Modificado desde" className="rounded-lg border border-border bg-surface p-2" type="date" value={after} onChange={event => setAfter(event.target.value)} /></label>
          <label className="space-y-1 text-sm"><span className="block">Modificado hasta</span><input aria-label="Modificado hasta" className="rounded-lg border border-border bg-surface p-2" type="date" value={before} onChange={event => setBefore(event.target.value)} /></label>
          <Button variant="tertiary" onPress={() => { setCategory(""); setAfter(""); setBefore(""); }}>Borrar filtros</Button>
          <div className="ml-auto flex gap-2">{(["list", "grid"] as const).map((choice, index) => <Button key={choice} aria-label={["Vista de lista", "Vista de cuadrícula"][index]} aria-pressed={view === choice} variant={view === choice ? "primary" : "secondary"} onPress={() => {setView(choice); localStorage.setItem("teldrive-v2-trash-view", choice);}}>{["Lista", "Cuadrícula"][index]}</Button>)}</div>
        </div>
        {!validRange ? <p role="alert" className="mb-4 text-danger">La fecha inicial debe ser anterior a la final.</p> : null}
        {query.isError ? <p role="alert" className="mb-4 text-danger">No se pudo consultar la papelera. <Button variant="secondary" onPress={() => void query.refetch()}>Reintentar</Button></p> : null}
        {query.isPending && validRange ? <Spinner aria-label="Cargando papelera" /> : files.length === 0 ? (
          <EmptyState
            title="La papelera está vacía"
            description="Los archivos y carpetas eliminados aparecerán aquí."
          />
        ) : (
          <Card className={view === "grid" ? "grid grid-cols-1 gap-3 border border-border bg-surface/80 p-3 sm:grid-cols-2 xl:grid-cols-3" : "gap-0 overflow-hidden border border-border bg-surface/80 shadow-sm"}>
            {files.map((file) => {
              return (
                <div
                  key={file.id}
                  className={view === "grid" ? "grid gap-3 rounded-xl border border-border p-4" : "grid min-h-16 gap-3 border-b border-border px-4 py-3 last:border-b-0 sm:grid-cols-[minmax(0,1fr)_5rem_auto] sm:items-center"}
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
                        Eliminada {new Date(file.updatedAt).toLocaleString()}
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
                      isDisabled={actions.pending}
                      onPress={() => void restore(file)}
                    >
                      <RestoreIcon className="size-4" /> Restaurar
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      isDisabled={actions.pending}
                      onPress={() => setPurging(file)}
                    >
                      <TrashIcon className="size-4" /> Eliminar definitivamente
                    </Button>
                  </div>
                </div>
              );
            })}
          </Card>
        )}
        {query.hasNextPage ? <div className="mt-4 flex justify-center"><Button isDisabled={query.isFetchingNextPage} onPress={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? "Cargando…" : "Mostrar más"}</Button></div> : null}
      </PageContent>

      <ConfirmDialog
        open={Boolean(purging)}
        onOpenChange={(open) => {
          if (!open) setPurging(undefined);
        }}
        title="¿Eliminar este elemento definitivamente?"
        message="Elimina el archivo y programa el borrado de sus datos en Telegram. Esta acción no se puede deshacer."
        confirmLabel="Eliminar definitivamente"
        isPending={actions.pending}
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
        isPending={actions.pending}
        onConfirm={() => {
          void cleanTrash().finally(() => setCleaningTrash(false));
        }}
      />
    </Page>
  );
}
