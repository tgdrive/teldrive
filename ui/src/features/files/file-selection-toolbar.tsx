import { Button } from "@heroui/react";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import CopyIcon from "~icons/gravity-ui/copy";
import CopyLinkIcon from "~icons/gravity-ui/copy-arrow-right";
import MoveIcon from "~icons/gravity-ui/folder-arrow-right";
import LinkIcon from "~icons/gravity-ui/link";
import PencilIcon from "~icons/gravity-ui/pencil";
import CutIcon from "~icons/gravity-ui/scissors";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import CloseIcon from "~icons/gravity-ui/xmark";
import type { FileEntry } from "../../api/types";

export function FileSelectionToolbar({
  selectedFiles,
  pending,
  onCut,
  onCopy,
  onRename,
  onDuplicate,
  onShare,
  onDownload,
  onCopyDownloadLinks,
  onMove,
  onTrash,
  onClear,
}: {
  selectedFiles: FileEntry[];
  pending: boolean;
  onCut?: () => void;
  onCopy?: () => void;
  onRename?: () => void;
  onDuplicate?: () => void;
  onShare?: () => void;
  onDownload?: () => void;
  onCopyDownloadLinks?: () => void;
  onMove?: () => void;
  onTrash?: () => void;
  onClear?: () => void;
}) {
  if (selectedFiles.length === 0) return null;
  const selectedCount = selectedFiles.length;
  const singleSelectedFile = selectedCount === 1 ? selectedFiles[0] : undefined;
  const selectedOnlyFiles = selectedFiles.every((file) => file.kind === "file");

  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center px-4">
      <fieldset
        aria-label="Selected file actions"
        className="pointer-events-auto flex max-w-full items-center gap-1.5 overflow-x-auto rounded-full border border-border bg-surface/95 p-1.5 shadow-xl backdrop-blur"
      >
        <span className="shrink-0 rounded-full bg-accent/10 px-3 py-2 text-sm font-medium text-accent">
          {selectedCount} selected
        </span>
        {onCut ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Cut selected items"
            isDisabled={pending}
            onPress={onCut}
          >
            <CutIcon className="size-4" />
          </Button>
        ) : null}
        {onCopy ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Copy selected items"
            isDisabled={pending}
            onPress={onCopy}
          >
            <CopyIcon className="size-4" />
          </Button>
        ) : null}
        {singleSelectedFile && onRename ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Rename selected item"
            isDisabled={pending}
            onPress={onRename}
          >
            <PencilIcon className="size-4" />
          </Button>
        ) : null}
        {singleSelectedFile && onDuplicate ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Duplicate selected item"
            isDisabled={pending}
            onPress={onDuplicate}
          >
            <CopyIcon className="size-4" />
          </Button>
        ) : null}
        {singleSelectedFile && onShare ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Share selected item"
            isDisabled={pending}
            onPress={onShare}
          >
            <LinkIcon className="size-4" />
          </Button>
        ) : null}
        {singleSelectedFile?.kind === "file" && onDownload ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Download selected file"
            isDisabled={pending}
            onPress={onDownload}
          >
            <DownloadIcon className="size-4" />
          </Button>
        ) : null}
        {selectedOnlyFiles && onCopyDownloadLinks ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={
              selectedCount === 1
                ? "Copy selected file download link"
                : "Copy selected files download links"
            }
            onPress={onCopyDownloadLinks}
            isDisabled={pending}
          >
            <CopyLinkIcon className="size-4" />
          </Button>
        ) : null}
        {onMove ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Move selected items"
            isDisabled={pending}
            onPress={onMove}
          >
            <MoveIcon className="size-4" />
          </Button>
        ) : null}
        {onTrash ? (
          <Button
            isIconOnly
            size="sm"
            variant="danger"
            aria-label="Move selected items to trash"
            isDisabled={pending}
            onPress={onTrash}
          >
            <TrashIcon className="size-4" />
          </Button>
        ) : null}
        {onClear ? (
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label="Clear selection"
            isDisabled={pending}
            onPress={onClear}
          >
            <CloseIcon className="size-4" />
          </Button>
        ) : null}
      </fieldset>
    </div>
  );
}
