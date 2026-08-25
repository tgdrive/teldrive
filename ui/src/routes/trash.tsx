import { Button, Card, Chip, Spinner } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import RefreshIcon from "~icons/gravity-ui/arrow-rotate-left";
import RestoreIcon from "~icons/gravity-ui/arrow-rotate-left";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import { userMessage } from "@/api/errors";
import type { FileEntry } from "@/api/types";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EmptyState, Page, PageContent, PageHeader } from "@/components/page";
import { useFileActions } from "@/features/files/mutations";
import { useFilePage } from "@/features/files/queries";

export const Route = createFileRoute("/trash")({
  component: TrashPage,
  pendingComponent: () => (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Spinner size="lg" />
    </div>
  ),
});

function TrashPage() {
  const query = useFilePage(
    {
      path: "/",
      sort: "updatedAt",
      order: "desc",
      view: "list",
    },
    "trashed",
  );
  const actions = useFileActions();
  const [purging, setPurging] = useState<FileEntry>();
  const [cleaningTrash, setCleaningTrash] = useState(false);

  const restore = async (file: FileEntry) => {
    try {
      await actions.restore(file.id);
      toast.success(`${file.name} restored`);
    } catch (error) {
      toast.error("File could not be restored", { description: userMessage(error) });
    }
  };

  const purge = async (file: FileEntry) => {
    try {
      await actions.purge(file.id);
      toast.success(`${file.name} permanently deleted`);
    } catch (error) {
      toast.error("File could not be permanently deleted", { description: userMessage(error) });
    }
  };

  const cleanTrash = async () => {
    try {
      await actions.cleanTrash();
      toast.success("Trash cleaned");
    } catch (error) {
      toast.error("Trash could not be cleaned", { description: userMessage(error) });
    }
  };

  return (
    <Page>
      <PageHeader
        title="Trash"
        description="Restore deleted items or permanently remove them from Teldrive."
        actions={
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="danger"
              isDisabled={query.data.items.length === 0 || actions.pending}
              onPress={() => setCleaningTrash(true)}
            >
              <TrashIcon className="size-4" /> Clean trash
            </Button>
            <Button
              size="sm"
              variant="tertiary"
              isDisabled={actions.pending}
              onPress={() => void query.refetch()}
            >
              <RefreshIcon className="size-4" /> Refresh
            </Button>
          </div>
        }
      />
      <PageContent>
        {query.data.items.length === 0 ? (
          <EmptyState
            title="Trash is empty"
            description="Deleted files and folders will appear here."
          />
        ) : (
          <Card className="gap-0 overflow-hidden border border-border bg-surface/80 shadow-sm">
            {query.data.items.map((file) => {
              const Icon = file.kind === "folder" ? FolderIcon : FileIcon;
              return (
                <div
                  key={file.id}
                  className="grid min-h-16 gap-3 border-b border-border px-4 py-3 last:border-b-0 sm:grid-cols-[minmax(0,1fr)_8rem_auto] sm:items-center"
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-default/20 text-muted">
                      <Icon className="size-4" />
                    </div>
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{file.name}</p>
                      <p className="mt-0.5 text-xs text-muted">
                        Deleted {new Date(file.updatedAt).toLocaleString()}
                      </p>
                    </div>
                  </div>
                  <Chip size="sm" variant="tertiary" className="w-fit capitalize">
                    {file.kind}
                  </Chip>
                  <div className="flex justify-end gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      isDisabled={actions.pending}
                      onPress={() => void restore(file)}
                    >
                      <RestoreIcon className="size-4" /> Restore
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      isDisabled={actions.pending}
                      onPress={() => setPurging(file)}
                    >
                      <TrashIcon className="size-4" /> Delete forever
                    </Button>
                  </div>
                </div>
              );
            })}
          </Card>
        )}
      </PageContent>

      <ConfirmDialog
        open={Boolean(purging)}
        onOpenChange={(open) => {
          if (!open) setPurging(undefined);
        }}
        title="Permanently delete this item?"
        message="This removes the file record and schedules its Telegram data for physical cleanup. This action cannot be undone."
        confirmLabel="Delete forever"
        isPending={actions.pending}
        onConfirm={() => {
          if (!purging) return;
          void purge(purging).finally(() => setPurging(undefined));
        }}
      />

      <ConfirmDialog
        open={cleaningTrash}
        onOpenChange={setCleaningTrash}
        title="Clean all trash?"
        message="This permanently deletes every item in trash and schedules its Telegram data for physical cleanup. This action cannot be undone."
        confirmLabel="Clean trash"
        isPending={actions.pending}
        onConfirm={() => {
          void cleanTrash().finally(() => setCleaningTrash(false));
        }}
      />
    </Page>
  );
}
