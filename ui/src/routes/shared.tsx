import { Spinner, Tabs } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useDeferredValue, useEffect, useState } from "react";
import { $api } from "@/api/client";
import type { FileEntry } from "@/api/types";
import { FilePreviewDialog, isPreviewable } from "@/components/file-preview-dialog";
import { Page, PageContent } from "@/components/page";
import { startFileDownload } from "@/features/files/download";
import { FileBrowser } from "@/features/files/file-browser";

type SharedSearch = {
  path: string;
  parentId?: string;
  query: string;
  view: "list" | "grid";
  tab: "shared" | "with-me";
};

export const Route = createFileRoute("/shared")({
  validateSearch: (search: Record<string, unknown>): SharedSearch => ({
    path: typeof search.path === "string" && search.path ? search.path : "/",
    parentId: typeof search.parentId === "string" ? search.parentId : undefined,
    query: typeof search.query === "string" ? search.query : "",
    view: search.view === "grid" ? "grid" : "list",
    tab: search.tab === "with-me" ? "with-me" : "shared",
  }),
  component: SharedPage,
  pendingComponent: () => (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  ),
});

function SharedPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [queryDraft, setQueryDraft] = useState(search.query);
  const deferredQuery = useDeferredValue(queryDraft.trim());
  const [previewFile, setPreviewFile] = useState<FileEntry>();
  const atRoot = !search.parentId;

  const sharedQuery = useQuery({
    ...$api.queryOptions("get", "/v1/shared"),
    enabled: atRoot && search.tab === "shared",
  });
  const sharedWithMeQuery = useQuery({
    ...$api.queryOptions("get", "/v1/shared/with-me"),
    enabled: atRoot && search.tab === "with-me",
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

  const rootsQuery = search.tab === "with-me" ? sharedWithMeQuery : sharedQuery;
  const rootFiles = (rootsQuery.data ?? []).filter(
    (file) =>
      !search.query || file.name.toLocaleLowerCase().includes(search.query.toLocaleLowerCase()),
  );
  const files = atRoot ? rootFiles : (childrenQuery.data?.items ?? []);
  const loading = atRoot ? rootsQuery.isPending : childrenQuery.isPending;

  const openFile = (file: FileEntry) => {
    if (file.kind === "folder") {
      const path = joinPath(search.path, file.name);
      void navigate({ search: { path, parentId: file.id, query: "", view: search.view, tab: search.tab } });
      return;
    }
    if (isPreviewable(file)) {
      setPreviewFile(file);
      return;
    }
    startFileDownload(file);
  };

  return (
    <Page className="h-full min-h-0 gap-0 overflow-x-hidden">
      <PageContent className="flex min-h-0 flex-1 overflow-x-hidden">
        <Tabs
          selectedKey={search.tab}
          onSelectionChange={(key) => {
            const tab = key === "with-me" ? "with-me" : "shared";
            void navigate({ search: { path: "/", query: "", view: search.view, tab }, replace: true });
          }}
          className="flex min-h-0 flex-1 flex-col"
        >
          <Tabs.ListContainer className="px-4 pt-3">
            <Tabs.List aria-label="Shared files">
              <Tabs.Tab id="shared">Shared</Tabs.Tab>
              <Tabs.Tab id="with-me">Shared with me</Tabs.Tab>
            </Tabs.List>
          </Tabs.ListContainer>
          <Tabs.Panel id={search.tab} className="flex min-h-0 flex-1 pt-2">
            <FileBrowser
              files={files}
              path={search.path}
              rootLabel={search.tab === "with-me" ? "Shared with me" : "Shared"}
              view={search.view}
              query={queryDraft}
              loading={loading}
              onQueryChange={setQueryDraft}
              onNavigatePath={(path) => {
                if (path === "/") {
                  void navigate({ search: { path: "/", query: "", view: search.view, tab: search.tab }, replace: true });
                  return;
                }
                const parts = path.split("/").filter(Boolean);
                if (parts.length < search.path.split("/").filter(Boolean).length) {
                  void navigate({ search: { path: "/", query: "", view: search.view, tab: search.tab }, replace: true });
                }
              }}
              onViewChange={(view) => void navigate({ search: { ...search, view }, replace: true })}
              onOpen={openFile}
              emptyHint={
                atRoot
                  ? search.tab === "with-me"
                    ? "Files and folders shared with you appear here."
                    : "Files and folders you shared appear here."
                  : "This shared folder is empty."
              }
            />
          </Tabs.Panel>
        </Tabs>
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
