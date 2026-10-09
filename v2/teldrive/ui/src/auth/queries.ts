import { queryOptions } from "@tanstack/react-query";
import { fetchClient } from "@/api/client";
import { unwrap } from "@/api/errors";

const currentUserQueryKey = ["auth", "current-user"] as const;

export function currentUserQueryOptions() {
  return queryOptions({
    queryKey: currentUserQueryKey,
    queryFn: () => unwrap(fetchClient.GET("/v1/me")),
    staleTime: 30_000,
    gcTime: 10 * 60_000,
    retry: false,
  });
}
