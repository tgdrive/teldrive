import { QueryClient } from "@tanstack/react-query";
import { NetworkError } from "./fetch-throw";

const HTTP_STATUS_TO_NOT_RETRY = [400, 401, 403, 404];

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      gcTime: 10 * (60 * 1000),
      staleTime: 5 * (60 * 1000),
      retry(count, error) {
        if (error instanceof NetworkError && HTTP_STATUS_TO_NOT_RETRY.includes(error.status!)) {
          return false;
        }

        return count < 4;
      },
    },
  },
});
