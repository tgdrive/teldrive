import { Button, Tooltip } from "@heroui/react";
import type { ComponentProps } from "react";
import { useAppLocale } from "@/lib/locale";
import DownloadIcon from "~icons/gravity-ui/arrow-down-to-line";
import BookmarkIcon from "~icons/gravity-ui/bookmark";
import CopyIcon from "~icons/gravity-ui/copy";
import CopyLinkIcon from "~icons/gravity-ui/copy-arrow-right";
import MoveIcon from "~icons/gravity-ui/folder-arrow-right";
import LinkIcon from "~icons/gravity-ui/link";
import PencilIcon from "~icons/gravity-ui/pencil";
import CutIcon from "~icons/gravity-ui/scissors";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import CloseIcon from "~icons/gravity-ui/xmark";
import type { FileEntry } from "../../api/types";

function SelectionActionButton(props: ComponentProps<typeof Button> & { "aria-label": string }) {
  return (
    <Tooltip delay={300}>
      <Button {...props} />
      <Tooltip.Content placement="top">{props["aria-label"]}</Tooltip.Content>
    </Tooltip>
  );
}

export function FileSelectionToolbar({
  selectedFiles,
  pending,
  onCut,
  onCopy,
  onBookmark,
  bookmarked = false,
  onRename,
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
  onBookmark?: () => void;
  bookmarked?: boolean;
  onRename?: () => void;
  onShare?: () => void;
  onDownload?: () => void;
  onCopyDownloadLinks?: () => void;
  onMove?: () => void;
  onTrash?: () => void;
  onClear?: () => void;
}) {
  const { t } = useAppLocale();
  if (selectedFiles.length === 0) return null;
  const selectedCount = selectedFiles.length;
  const singleSelectedFile = selectedCount === 1 ? selectedFiles[0] : undefined;
  const selectedOnlyFiles = selectedFiles.every((file) => file.kind === "file");

  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-4 z-30 flex justify-center px-4">
      <fieldset
        aria-label={t("selection.actions")}
        className="pointer-events-auto flex min-w-0 max-w-full flex-wrap items-center justify-center gap-1.5 rounded-2xl border border-border bg-surface/95 p-1.5 shadow-xl backdrop-blur @2xl/file-browser:rounded-full [&>button]:shrink-0"
      >
        <span className="shrink-0 rounded-full bg-accent/10 px-3 py-2 text-sm font-medium text-accent">
          {t("selection.count", { count: selectedCount })}
        </span>
        {onCut ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("selection.cut")}
            isDisabled={pending}
            onPress={onCut}
          >
            <CutIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {onCopy ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("selection.copy")}
            isDisabled={pending}
            onPress={onCopy}
          >
            <CopyIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {onBookmark ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={bookmarked ? t("selection.bookmarkRemove") : t("selection.bookmarkAdd")}
            isDisabled={pending}
            onPress={onBookmark}
          >
            <BookmarkIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {singleSelectedFile && onRename ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("selection.rename")}
            isDisabled={pending}
            onPress={onRename}
          >
            <PencilIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {singleSelectedFile && onShare ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("selection.share")}
            isDisabled={pending}
            onPress={onShare}
          >
            <LinkIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {singleSelectedFile?.kind === "file" && onDownload ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("selection.download")}
            isDisabled={pending}
            onPress={onDownload}
          >
            <DownloadIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {selectedOnlyFiles && onCopyDownloadLinks ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={
              selectedCount === 1 ? t("selection.copyLinkOne") : t("selection.copyLinkMany")
            }
            onPress={onCopyDownloadLinks}
            isDisabled={pending}
          >
            <CopyLinkIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {onMove ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("selection.move")}
            isDisabled={pending}
            onPress={onMove}
          >
            <MoveIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {onTrash ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="danger"
            aria-label={t("selection.trash")}
            isDisabled={pending}
            onPress={onTrash}
          >
            <TrashIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
        {onClear ? (
          <SelectionActionButton
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={t("selection.clear")}
            isDisabled={pending}
            onPress={onClear}
          >
            <CloseIcon className="size-4" />
          </SelectionActionButton>
        ) : null}
      </fieldset>
    </div>
  );
}
