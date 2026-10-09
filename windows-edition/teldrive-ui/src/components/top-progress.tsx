import { useIsFetching } from "@tanstack/react-query";
import { useRouterState } from "@tanstack/react-router";
import { memo, useEffect, useRef } from "react";
import LoadingBar, { type LoadingBarRef } from "react-top-loading-bar";

export const TopLoader = memo(() => {
  const ref = useRef<LoadingBarRef>(null);
  const completed = useRef(true);

  const [status, pathname] = useRouterState({ select: (s) => [s.status, s.location.pathname] });

  const loadingQueriesCount = useIsFetching({
    predicate: (query) => !query.state.dataUpdatedAt,
    fetchStatus: "fetching",
  });

  useEffect(() => {
    if (pathname?.startsWith("/search") || pathname?.startsWith("/settings")) return;

    if (status === "pending" && loadingQueriesCount > 0 && completed.current) {
      ref?.current?.continuousStart();
      completed.current = false;
    }
    if (status === "idle" && !completed.current) {
      ref?.current?.complete();
      completed.current = true;
    }
  }, [loadingQueriesCount, status, pathname]);

  return <LoadingBar shadow={false} className="!bg-primary" ref={ref} waitingTime={200} />;
});
