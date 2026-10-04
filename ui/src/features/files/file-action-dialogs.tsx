import { Button, Input, Label, TextField } from "@heroui/react";
import type { FileEntry } from "../../api/types";
import { AppDialog } from "../../components/dialogs/app-dialog";
import { FilePreviewDialog } from "../../components/file-preview-dialog";
import { FolderPicker } from "./folder-picker";
import { ShareDialog } from "./share-dialog";

export function FileActionDialogs({
  renameFile,
  renameName,
  onRenameNameChange,
  onRenameClose,
  onRename,
  pending,
  error,
  destinationAction,
  shareFile,
  onShareClose,
  previewFile,
  onPreviewClose,
}: {
  renameFile?: FileEntry;
  renameName: string;
  onRenameNameChange: (name: string) => void;
  onRenameClose: () => void;
  onRename: () => void;
  pending: boolean;
  error?: string;
  destinationAction?: {
    mode: "move" | "copy";
    count: number;
    onClose: () => void;
    onConfirm: (parentId?: string) => void;
  };
  shareFile?: FileEntry;
  onShareClose: () => void;
  previewFile?: FileEntry;
  onPreviewClose: () => void;
}) {
  return (
    <>
      <AppDialog
        open={Boolean(renameFile)}
        onOpenChange={(open) => {
          if (!open) onRenameClose();
        }}
        title="Rename item"
        isDismissable={!pending}
        isCloseDisabled={pending}
        size="md"
        footer={
          <>
            <Button variant="secondary" isDisabled={pending} onPress={onRenameClose}>
              Cancel
            </Button>
            <Button variant="primary" isDisabled={!renameName.trim() || pending} onPress={onRename}>
              Rename
            </Button>
          </>
        }
      >
        <TextField
          isDisabled={pending}
          autoFocus
          value={renameName}
          onChange={onRenameNameChange}
          onKeyDown={(event) => {
            if (event.key === "Enter" && renameName.trim() && !pending) {
              event.preventDefault();
              onRename();
            } else event.continuePropagation();
          }}
        >
          <Label>New name</Label>
          <Input />
        </TextField>
        {error && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
      </AppDialog>

      {destinationAction ? (
        <AppDialog
          open
          onOpenChange={(open) => {
            if (!open) destinationAction.onClose();
          }}
          title={`${destinationAction.mode === "move" ? "Move" : "Copy"} ${destinationAction.count} item${destinationAction.count === 1 ? "" : "s"}`}
          description="Choose the destination folder."
          isDismissable={!pending}
          isCloseDisabled={pending}
        >
          {error && (
            <p role="alert" className="mb-3 text-sm text-danger">
              {error}
            </p>
          )}
          <FolderPicker
            initialPath="/"
            confirmLabel={destinationAction.mode === "move" ? "Move here" : "Copy here"}
            isDisabled={pending}
            onConfirm={(parentId) => destinationAction.onConfirm(parentId)}
          />
        </AppDialog>
      ) : null}

      <ShareDialog
        file={shareFile}
        onOpenChange={(open) => {
          if (!open) onShareClose();
        }}
      />
      <FilePreviewDialog
        file={previewFile}
        onOpenChange={(open) => {
          if (!open) onPreviewClose();
        }}
      />
    </>
  );
}
