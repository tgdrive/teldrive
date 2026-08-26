import { createFileRoute } from "@tanstack/react-router";
import {
  SharedFileBrowser,
  SharedPageSpinner,
  sharedBrowserSearch,
} from "@/features/files/shared-file-browser";

export const Route = createFileRoute("/shared-with-me")({
  validateSearch: sharedBrowserSearch,
  component: SharedWithMePage,
  pendingComponent: SharedPageSpinner,
});

function SharedWithMePage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();

  return (
    <SharedFileBrowser
      mode="with-me"
      search={search}
      navigate={(next, replace) => void navigate({ search: next, replace })}
    />
  );
}
