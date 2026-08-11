import { useSuspenseQuery } from "@tanstack/react-query";
import { currentUserQueryOptions } from "./queries";

export function useCurrentUser() {
  return useSuspenseQuery(currentUserQueryOptions());
}
