import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./errors";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 20_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
      retry(failures, error) {
        if (error instanceof ApiError && !error.retryable) return false;
        return failures < 2;
      },
      retryDelay(attempt, error) {
        if (error instanceof ApiError && error.retryAfterSeconds)
          return error.retryAfterSeconds * 1000;
        return Math.min(1000 * 2 ** attempt, 8000);
      },
    },
    mutations: { retry: false },
  },
});
