import type { ReactElement, ReactNode } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { cn } from "@/lib/utils";

interface LightboxDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: ReactElement;
  title: ReactNode;
  children: ReactNode;
  className?: string;
  closeOnContentClick?: boolean;
}

export function LightboxDialog({
  open,
  onOpenChange,
  trigger,
  title,
  children,
  className,
  closeOnContentClick = false,
}: LightboxDialogProps) {
  return (
    <BaseDialog.Root open={open} onOpenChange={onOpenChange}>
      <BaseDialog.Trigger render={trigger} />
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="fixed inset-0 z-[200] cursor-zoom-out bg-black/60 light:bg-black/25" />
        <BaseDialog.Popup
          aria-describedby={undefined}
          onClick={closeOnContentClick ? () => onOpenChange(false) : undefined}
          className={cn(
            "fixed inset-0 z-[201] m-auto h-fit w-fit max-h-[90vh] max-w-[min(1400px,95vw)] overflow-auto rounded-lg border-0 bg-surface shadow-[var(--shadow-popover)] outline-none data-[open]:animate-rise-in",
            closeOnContentClick && "cursor-zoom-out",
            className,
          )}
        >
          <BaseDialog.Title className="sr-only">{title}</BaseDialog.Title>
          {children}
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
