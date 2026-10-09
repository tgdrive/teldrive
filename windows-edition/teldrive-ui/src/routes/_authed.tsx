import type { QueryClient } from "@tanstack/react-query";
import { createFileRoute, type ParsedLocation, redirect } from "@tanstack/react-router";

import { AuthLayout } from "@/layouts/auth-layout";
import { sessionOptions } from "@/utils/query-options";

const checkAuth = async (queryClient: QueryClient, location: ParsedLocation, preload: boolean) => {
  if (preload) {
    return;
  }
  const session = await queryClient.ensureQueryData(sessionOptions);
  if (!session) {
    redirect({
      to: "/login",
      search: {
        redirect: location.href,
      },
      throw: true,
    });
  }
};

export const Route = createFileRoute("/_authed")({
  component: AuthLayout,
  beforeLoad: ({ location, context: { queryClient }, preload }) =>
    checkAuth(queryClient, location, preload),
});
