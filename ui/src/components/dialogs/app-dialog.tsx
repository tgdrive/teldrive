import { cn, Modal } from "@heroui/react";
import type { ReactNode } from "react";

const DEFAULT_DIALOG_CLASS = "min-w-0 sm:w-[min(92vw,30rem)] sm:max-w-none bg-surface";

type AppDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  isDismissable?: boolean;
  size?: "md" | "lg";
  className?: string;
  bodyClassName?: string;
  headerClassName?: string;
};

export function AppDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  isDismissable = true,
  size = "lg",
  className,
  bodyClassName,
  headerClassName,
}: AppDialogProps) {
  return (
    <Modal.Backdrop isOpen={open} onOpenChange={onOpenChange} isDismissable={isDismissable}>
      <Modal.Container size={size} scroll="inside">
        <Modal.Dialog className={cn(DEFAULT_DIALOG_CLASS, className)}>
          <Modal.CloseTrigger />
          <Modal.Header className={headerClassName}>
            <Modal.Heading>{title}</Modal.Heading>
            {description ? <div className="text-sm text-muted">{description}</div> : null}
          </Modal.Header>
          <Modal.Body className={bodyClassName}>{children}</Modal.Body>
          {footer ? <Modal.Footer>{footer}</Modal.Footer> : null}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
