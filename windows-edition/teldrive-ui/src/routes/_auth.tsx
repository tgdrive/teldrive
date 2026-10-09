import { createFileRoute, redirect } from "@tanstack/react-router";

import { NonAuthLayout } from "@/layouts/non-auth-layout";
import { $api } from "@/utils/api";
import { sessionOptions } from "@/utils/query-options";

export const Route = createFileRoute("/_auth")({
  component: NonAuthLayout,
  beforeLoad: async ({ context: { queryClient } }) => {
    const session = await queryClient.ensureQueryData(sessionOptions);
    if (session) {
      redirect({
        to: "/$view",
        params: { view: "my-drive" },
        search: {
          path: "/",
        },
        throw: true,
      });
    }
  },
});
