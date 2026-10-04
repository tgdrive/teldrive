import { Spinner } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { FileManagerPage, type FilesLocation } from "@/features/files/file-manager";

export const Route = createFileRoute("/files")({
  validateSearch: (search: Record<string, unknown>): FilesLocation => ({
    path: typeof search.path === "string" && search.path ? search.path : "/",
    parentId: typeof search.parentId === "string" ? search.parentId : undefined,
    query: typeof search.query === "string" ? search.query : "",
    view: search.view === "grid" ? "grid" : "list",
    split: search.split === true || search.split === "true",
    secondaryPath:
      typeof search.secondaryPath === "string" && search.secondaryPath
        ? search.secondaryPath
        : undefined,
    secondaryParentId:
      typeof search.secondaryParentId === "string" ? search.secondaryParentId : undefined,
    secondaryQuery: typeof search.secondaryQuery === "string" ? search.secondaryQuery : undefined,
    secondaryView:
      search.secondaryView === "grid" || search.secondaryView === "list"
        ? search.secondaryView
        : undefined,
  }),
  component: FilesPage,
  pendingComponent: () => (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  ),
});

function FilesPage() {
  const location = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <FileManagerPage
      location={location}
      onLocationChange={(search, replace) => void navigate({ search, replace })}
    />
  );
}
