import { createFileRoute } from "@tanstack/react-router";

import { SettingsTabView } from "@/components/settings/settings-tab-view";
import { $api } from "@/utils/api";
import { Spinner } from "@tw-material/react";
import { center } from "@/utils/classes";

export const Route = createFileRoute("/_authed/settings/$tabId")({
  component: SettingsTabView,
  wrapInSuspense: true,
  pendingComponent: () => <Spinner className={center} />,
  loader: async ({ context: { queryClient }, params }) => {
    if (params.tabId === "account") {
      await Promise.all([
        queryClient.ensureQueryData($api.queryOptions("get", "/users/sessions")),
        queryClient.ensureQueryData($api.queryOptions("get", "/users/config")),
        queryClient.ensureQueryData($api.queryOptions("get", "/users/channels")),
      ]);
    }
  },
});
