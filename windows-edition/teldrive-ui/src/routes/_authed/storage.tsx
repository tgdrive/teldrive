import { createFileRoute } from "@tanstack/react-router";

import { $api } from "@/utils/api";

export const Route = createFileRoute("/_authed/storage")({
  wrapInSuspense: true,
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([
      queryClient.ensureQueryData(
        $api.queryOptions("get", "/uploads/stats", {
          params: {
            query: {
              days: 60,
            },
          },
        }),
      ),
      queryClient.ensureQueryData($api.queryOptions("get", "/files/categories")),
    ]);
  },
});
