import { useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { getRouteApi, useNavigate } from "@tanstack/react-router";
import {
  FbActions,
  FileBrowser,
  type FileBrowserHandle,
  FileContextMenu,
  FileList,
  FileNavbar,
} from "@tw-material/file-browser";
import { Button } from "@tw-material/react";
import { memo, useEffect, useMemo, useRef } from "react";
import type {
  StateSnapshot,
  VirtuosoGridHandle,
  VirtuosoHandle,
} from "react-virtuoso";
import useBreakpoint from "use-breakpoint";

import {
  CustomActions,
  fileActions,
  useFileAction,
} from "@/hooks/use-file-action";
import { chainLinks } from "@/utils/common";
import {
  BREAKPOINTS,
  defaultSortState,
  defaultViewId,
  sortViewMap,
} from "@/utils/defaults";
import { fileQueries, useSession } from "@/utils/query-options";
import { useFileUploadStore, useModalStore } from "@/utils/stores";

import type { BrowseView, FileListParams } from "@/types";
import { fileBrowserLocale } from "@/utils/file-browser-locale";
import { DriveToolbar } from "./drive-toolbar";
import { FileOperationModal } from "./modals/file-operation";
import PreviewModal from "./modals/preview";
import { Upload } from "./upload";
import { UploadDropzone } from "./upload/drop-zone";

let firstRender = true;

function isVirtuosoList(value: any): value is VirtuosoHandle {
  return (value as VirtuosoHandle).getState !== undefined;
}

const modalFileActions = [
  FbActions.RenameFile.id,
  FbActions.CreateFolder.id,
  FbActions.DeleteFiles.id,
  CustomActions.ShareFiles.id,
];

const fileRoute = getRouteApi("/_authed/$view");

const positions = new Map<string, StateSnapshot>();

const quickFilters = [
  { id: "image", label: "Imágenes" },
  { id: "video", label: "Vídeos" },
  { id: "audio", label: "Audio" },
  { id: "document", label: "Documentos" },
] as const;

export const DriveFileBrowser = memo(() => {
  const { view } = fileRoute.useParams();
  const navigate = useNavigate();

  const search = fileRoute.useSearch();

  const listRef = useRef<VirtuosoHandle | VirtuosoGridHandle>(null);
  const browserRef = useRef<FileBrowserHandle>(null);

  const [session] = useSession();

  const queryParams: FileListParams = {
    view: view as BrowseView,
    params: search,
  };
  const queryOptions = fileQueries.list(queryParams, session?.hash);

  const modalOpen = useModalStore((state) => state.open);

  const modalOperation = useModalStore((state) => state.operation);

  const openUpload = useFileUploadStore((state) => state.uploadOpen);

  const { breakpoint } = useBreakpoint(BREAKPOINTS);

  const {
    data: files,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useSuspenseInfiniteQuery(queryOptions);

  const actionHandler = useFileAction(queryParams, session!);

  const folderChain = useMemo(() => {
    if (view === "my-drive") {
      return chainLinks(search?.path || "").map(([name, path], index) => ({
        id: index + name,
        name,
        path,
        isDir: true,
        chain: true,
      }));
    }

    return [];
  }, [search?.path, view]);

  useEffect(() => {
    if (firstRender) {
      firstRender = false;
      return;
    }

    setTimeout(() => {
      listRef.current?.scrollTo({
        top: positions.get(view + search?.path || "")?.scrollTop ?? 0,
        left: 0,
      });
    }, 0);

    return () => {
      if (listRef.current && isVirtuosoList(listRef.current)) {
        listRef.current?.getState((state) =>
          positions.set(view + search?.path || "", state),
        );
      }
    };
  }, [search?.path, view]);

  return (
    <div className="size-full m-auto relative drive-files flex flex-col">
      <div className="mb-4 shrink-0">
        <h1 className="text-2xl tracking-tight mb-1">{view === "my-drive" ? "Mi unidad" : view === "recent" ? "Recientes" : view === "shared" ? "Mis enlaces compartidos" : view === "search" ? "Resultados de búsqueda" : "Archivos"}</h1>
        <p className="text-sm text-on-surface-variant">{view === "shared" ? "Administra los archivos y carpetas que compartes por enlace." : view === "search" ? (search.query ? `Resultados para «${search.query}»` : "Filtra y encuentra tus archivos.") : "Selecciona un archivo para ver sus acciones. Haz doble clic para abrirlo."}</p>
        {(view === "my-drive" || view === "search") && <div className="flex flex-wrap gap-2 mt-4" aria-label="Filtrar por tipo de archivo">
          {quickFilters.map((filter) => {
            const active = search.category?.includes(filter.id) || false;
            return <Button key={filter.id} size="sm" variant={active ? "filledTonal" : "outlined"} className="rounded-lg" aria-pressed={active} onPress={() => void navigate({ to: "/$view", params: { view: "search" }, search: { ...search, category: active ? undefined : [filter.id], ...(view === "my-drive" ? { path: search.path || "/", deepSearch: true } : {}) } })}>{filter.label}</Button>;
          })}
          {view === "search" && <Button size="sm" variant="text" onPress={() => void navigate({ to: "/$view", params: { view: "my-drive" }, search: { path: "/" } })}>Limpiar filtros</Button>}
        </div>}
      </div>
      <UploadDropzone isDisabled={view !== "my-drive"}>
        <FileBrowser
          i18n={fileBrowserLocale}
          ref={browserRef}
          files={files}
          folderChain={folderChain}
          onFileAction={actionHandler()}
          fileActions={view === "my-drive" ? fileActions : fileActions.filter((action) => action.id !== CustomActions.UploadFolder.id)}
          disableEssentailFileActions={view !== "my-drive" ? [FbActions.UploadFiles.id, FbActions.CreateFolder.id, FbActions.CutFiles.id, FbActions.PasteFiles.id] : []}
          defaultFileViewActionId={defaultViewId}
          defaultSortActionId={
            view === "my-drive"
              ? defaultSortState.sortId
              : sortViewMap[view].sortId
          }
          defaultSortOrder={
            view === "my-drive"
              ? defaultSortState.order
              : sortViewMap[view].order
          }
          breakpoint={breakpoint}
        >
          {view === "my-drive" && <FileNavbar breakpoint={breakpoint} />}
          <DriveToolbar browserRef={browserRef} />
          <FileList
            hasNextPage={hasNextPage}
            isNextPageLoading={isFetchingNextPage}
            loadNextPage={fetchNextPage}
            ref={listRef}
          />
          <FileContextMenu />
        </FileBrowser>
      </UploadDropzone>

      {modalFileActions.find((val) => val === modalOperation) && modalOpen && (
        <FileOperationModal queryKey={queryOptions.queryKey} />
      )}

      {modalOperation === FbActions.OpenFiles.id && modalOpen && (
        <PreviewModal
          session={session!}
          files={files}
          path={search?.path || ""}
          view={view as BrowseView}
        />
      )}
      {openUpload && <Upload queryKey={queryOptions.queryKey} />}
    </div>
  );
});
