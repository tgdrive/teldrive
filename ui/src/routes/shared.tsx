import { createFileRoute } from "@tanstack/react-router";
import {
  SharedFileBrowser,
  SharedPageSpinner,
  sharedBrowserSearch,
} from "@/features/files/shared-file-browser";

export const Route = createFileRoute("/shared")({
  validateSearch: sharedBrowserSearch,
  component: SharedPage,
  pendingComponent: SharedPageSpinner,
});

function SharedPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();

  return (
    <SharedFileBrowser
      mode="shared"
      search={search}
      navigate={(next, replace) => void navigate({ search: next, replace })}
    />
  );
}
