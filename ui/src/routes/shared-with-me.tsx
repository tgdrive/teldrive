import { Spinner } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useDeferredValue, useEffect, useState } from "react";
import { $api } from "@/api/client";
import type { FileEntry } from "@/api/types";
import { FilePreviewDialog, isPreviewable } from "@/components/file-preview-dialog";
import { Page, PageContent } from "@/components/page";
import { FileBrowser } from "@/features/files/file-browser";

type SharedSearch = {
  path: string;
  parentId?: string;
  query: string;
  view: "list" | "grid";
};

export const Route = createFileRoute("/shared-with-me")({
  validateSearch: (search: Record<string, unknown>): SharedSearch => ({
    path: typeof search.path === "string" && search.path ? search.path : "/",
    parentId: typeof search.parentId === "string" ? search.parentId : undefined,
    query: typeof search.query === "string" ? search.query : "",
    view: search.view === "grid" ? "grid" : "list",
  }),
  component: SharedWithMePage,
  pendingComponent: () => (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  ),
});

function SharedWithMePage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [queryDraft, setQueryDraft] = useState(search.query);
  const deferredQuery = useDeferredValue(queryDraft.trim());
  const [previewFile, setPreviewFile] = useState<FileEntry>();
  const atRoot = !search.parentId;

  const rootsQuery = useQuery({
    ...$api.queryOptions("get", "/v1/shared-with-me"),
    enabled: atRoot,
  });
  const childrenQuery = useQuery({
    ...$api.queryOptions("get", "/v1/files", {
      params: {
        query: {
          parentId: search.parentId,
          search: search.query || undefined,
          sort: "name",
          order: "asc",
          status: "active",
          limit: 200,
        },
      },
    }),
    enabled: !atRoot,
  });

  useEffect(() => setQueryDraft(search.query), [search.query]);
  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (deferredQuery !== search.query) {
        void navigate({ search: { ...search, query: deferredQuery }, replace: true });
      }
    }, 250);
    return () => window.clearTimeout(timer);
  }, [deferredQuery, navigate, search]);

  const rootFiles = (rootsQuery.data ?? [])
    .map((entry) => entry.file)
    .filter(
      (file) =>
        !search.query || file.name.toLocaleLowerCase().includes(search.query.toLocaleLowerCase()),
    );
  const files = atRoot ? rootFiles : (childrenQuery.data?.items ?? []);
  const loading = atRoot ? rootsQuery.isPending : childrenQuery.isPending;

  const openFile = (file: FileEntry) => {
    if (file.kind === "folder") {
      const path = joinPath(search.path, file.name);
      void navigate({ search: { path, parentId: file.id, query: "", view: search.view } });
      return;
    }
    if (isPreviewable(file)) {
      setPreviewFile(file);
      return;
    }
    const anchor = document.createElement("a");
    anchor.href = `/api/v1/files/${encodeURIComponent(file.id)}/content`;
    anchor.download = file.name;
    anchor.click();
  };

  return (
    <Page className="h-full min-h-0 gap-0 overflow-x-hidden">
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <FileBrowser
          files={files}
          path={search.path}
          rootLabel="Shared with me"
          view={search.view}
          query={queryDraft}
          loading={loading}
          onQueryChange={setQueryDraft}
          onNavigatePath={(path) => {
            if (path === "/") {
              void navigate({ search: { path: "/", query: "", view: search.view }, replace: true });
              return;
            }
            const parts = path.split("/").filter(Boolean);
            if (parts.length < search.path.split("/").filter(Boolean).length) {
              void navigate({ search: { path: "/", query: "", view: search.view }, replace: true });
            }
          }}
          onViewChange={(view) => void navigate({ search: { ...search, view }, replace: true })}
          onOpen={openFile}
          emptyHint={
            atRoot
              ? "Files and folders shared directly with you appear here."
              : "This shared folder is empty."
          }
        />
      </PageContent>
      <FilePreviewDialog
        file={previewFile}
        onOpenChange={(open) => {
          if (!open) setPreviewFile(undefined);
        }}
      />
    </Page>
  );
}

function joinPath(parent: string, name: string) {
  return `${parent === "/" ? "" : parent}/${name}`.replace(/\/+/g, "/") || "/";
}
