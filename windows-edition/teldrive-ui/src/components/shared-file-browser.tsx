import { useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import {
  FbActions,
  FileBrowser,
  type FileBrowserHandle,
  FileContextMenu,
  FileList,
  FileNavbar,
} from "@tw-material/file-browser";
import { memo, useEffect, useMemo, useRef } from "react";
import type { StateSnapshot, VirtuosoGridHandle, VirtuosoHandle } from "react-virtuoso";
import useBreakpoint from "use-breakpoint";

import { sharefileActions, useShareFileAction } from "@/hooks/use-file-action";
import { $api } from "@/utils/api";
import { chainSharedLinks } from "@/utils/common";
import { BREAKPOINTS, defaultViewId } from "@/utils/defaults";
import { fileBrowserLocale } from "@/utils/file-browser-locale";
import { shareQueries } from "@/utils/query-options";
import { useModalStore } from "@/utils/stores";
import { DriveToolbar } from "./drive-toolbar";
import PreviewModal from "./modals/preview";

let firstRender = true;

function isVirtuosoList(value: any): value is VirtuosoHandle {
  return (value as VirtuosoHandle).getState !== undefined;
}

const route = getRouteApi("/_share/share/$id");

const positions = new Map<string, StateSnapshot>();

const disabledActions = [
  FbActions.UploadFiles.id,
  FbActions.CreateFolder.id,
  FbActions.CutFiles.id,
  FbActions.SelectMode.id,
  FbActions.PasteFiles.id,
  FbActions.RenameFile.id,
  FbActions.DeleteFiles.id,
];

export const SharedFileBrowser = memo(() => {
  const { id } = route.useParams();

  const { path } = route.useSearch();

  const listRef = useRef<VirtuosoHandle | VirtuosoGridHandle>(null);
  const browserRef = useRef<FileBrowserHandle>(null);

  const { breakpoint } = useBreakpoint(BREAKPOINTS);

  const params = {
    id,
    path: path || "",
  };

  const {
    data: { name, type },
  } = $api.useSuspenseQuery("get", "/shares/{id}", {
    params: {
      path: {
        id,
      },
    },
  });

  const {
    data: files,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useSuspenseInfiniteQuery(shareQueries.list(params));

  const actionHandler = useShareFileAction(params);

  const folderChain = useMemo(() => {
    if (type === "file") {
      return [];
    }
    return chainSharedLinks(name, params.path!).map(([name, path], index) => ({
      id: index + name,
      name,
      path,
      isDir: true,
      chain: true,
    }));
  }, [params.path, name, type]);

  const modalOpen = useModalStore((state) => state.open);

  const modalOperation = useModalStore((state) => state.operation);

  useEffect(() => {
    if (firstRender) {
      firstRender = false;
      return;
    }

    setTimeout(() => {
      listRef.current?.scrollTo({
        top: positions.get(id + path)?.scrollTop ?? 0,
        left: 0,
      });
    }, 0);

    return () => {
      if (listRef.current && isVirtuosoList(listRef.current)) {
        listRef.current?.getState((state) => positions.set(id + path, state));
      }
    };
  }, [id, path]);

  return (
    <div className="size-full m-auto drive-files">
      <div className="pb-4"><h1 className="text-2xl truncate" title={name}>{name}</h1><p className="text-sm text-on-surface-variant mt-1">Compartido por enlace · Puedes ver y descargar archivos.</p></div>
      <FileBrowser
        i18n={fileBrowserLocale}
        ref={browserRef}
        files={files}
        folderChain={folderChain}
        onFileAction={actionHandler()}
        fileActions={sharefileActions}
        breakpoint={breakpoint}
        defaultFileViewActionId={defaultViewId}
        disableEssentailFileActions={disabledActions}
      >
        <FileNavbar breakpoint={breakpoint} />
        <DriveToolbar browserRef={browserRef} readOnly />
        <FileList
          hasNextPage={hasNextPage}
          isNextPageLoading={isFetchingNextPage}
          loadNextPage={fetchNextPage}
          ref={listRef}
        />
        <FileContextMenu />
      </FileBrowser>
      {modalOperation === FbActions.OpenFiles.id && modalOpen && (
        <PreviewModal shareId={params.id} files={files} view="shared" path="" />
      )}
    </div>
  );
});
