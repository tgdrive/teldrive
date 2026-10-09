import type { ShareListParams } from "@/types";
import { createFileRoute } from "@tanstack/react-router";
import { shareQueries } from "@/utils/query-options";
import { ErrorView } from "@/components/error-view";
import { $api } from "@/utils/api";

export const Route = createFileRoute("/_share/share/$id")({
  validateSearch: (search: Record<string, unknown>) =>
    search as {
      path?: string;
    },
  loaderDeps: ({ search }) => search,
  loader: async ({ context: { queryClient }, params: { id }, deps }) => {
    const res = await queryClient.ensureQueryData(
      $api.queryOptions("get", "/shares/{id}", {
        params: {
          path: {
            id,
          },
        },
      }),
    );
    const password = JSON.parse(sessionStorage.getItem("password") || "null");
    const queryParams = {
      id,
      password: password || "",
      path: deps.path || "",
    } as ShareListParams;

    if (res.protected && !password) {
      return;
    }
    await queryClient.ensureInfiniteQueryData(shareQueries.list(queryParams));
  },
  wrapInSuspense: true,
  errorComponent: ({ error }) => {
    return <ErrorView message={error.message} />;
  },
});
