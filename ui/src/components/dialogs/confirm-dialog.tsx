import { AlertDialog, Button } from "@heroui/react";
import TrashBinIcon from "~icons/gravity-ui/trash-bin";
import { useAppLocale } from "@/lib/locale";

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  title: string;
  message: string;
  confirmLabel?: string;
  isPending?: boolean;
  isDestructive?: boolean;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
  title,
  message,
  confirmLabel,
  isPending = false,
  isDestructive = true,
}: ConfirmDialogProps) {
  const { t } = useAppLocale();
  return (
    <AlertDialog.Backdrop isOpen={open} onOpenChange={onOpenChange}>
      <AlertDialog.Container>
        <AlertDialog.Dialog className="sm:max-w-[400px]">
          <AlertDialog.CloseTrigger />
          <AlertDialog.Header>
            {isDestructive && (
              <AlertDialog.Icon status="danger">
                <TrashBinIcon className="size-5" />
              </AlertDialog.Icon>
            )}
            <AlertDialog.Heading>{title}</AlertDialog.Heading>
          </AlertDialog.Header>
          <AlertDialog.Body>
            <p>{message}</p>
          </AlertDialog.Body>
          <AlertDialog.Footer>
            <Button slot="close" variant="tertiary">
              {t("dialog.cancel")}
            </Button>
            <Button
              variant={isDestructive ? "danger" : "primary"}
              isPending={isPending}
              onPress={() => {
                onConfirm();
              }}
            >
              {confirmLabel ?? t("dialog.delete")}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  );
}
