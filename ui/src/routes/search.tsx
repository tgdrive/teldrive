import { createFileRoute } from "@tanstack/react-router";
import { FileManagerPage } from "@/features/files/file-manager";
import { validateDriveSearch } from "@/features/files/search-state";

export const Route = createFileRoute("/search")({
  validateSearch: validateDriveSearch,
  component: SearchPage,
});

function SearchPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <FileManagerPage
      location={{ path: "/", query: search.q ?? "", view: search.view ?? "list" }}
      onLocationChange={(location, replace) =>
        void navigate({ search: { ...search, view: location.view }, replace })
      }
      searchMode={{
        criteria: search,
        onChange: (next, replace) => void navigate({ search: next, replace }),
      }}
    />
  );
}
