import type { BrowseView, FileListParams } from "@/types";
import { createFileRoute } from "@tanstack/react-router";

import { ErrorView } from "@/components/error-view";
import { fileQueries } from "@/utils/query-options";

const allowedTypes = ["my-drive", "recent", "search", "storage", "browse", "shared", "trash", "spam"];

export const Route = createFileRoute("/_authed/$view")({
  beforeLoad: ({ params }) => {
    if (!allowedTypes.includes(params.view)) {
      throw new Error("Ruta no válida");
    }
  },
  validateSearch: (search: Record<string, unknown>) => (search || {}) as FileListParams["params"],
  loaderDeps: ({ search }) => search,
  loader: async ({ context: { queryClient }, deps, params }) => {
    if (params.view === "trash" || params.view === "spam") return;
    await queryClient.ensureInfiniteQueryData(
      fileQueries.list({ view: params.view as BrowseView, params: deps }),
    );
  },
  wrapInSuspense: true,
  errorComponent: ({ error }) => {
    return <ErrorView message={error.message} />;
  },
});
