import type { ReactElement, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { DialogPrimitive } from "@/ui/primitives";
import { FLOATING_MOTION, MODAL_SCRIM } from "./floating-surface";

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
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Trigger render={trigger} />
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className={cn(MODAL_SCRIM, "cursor-zoom-out")} />
        <DialogPrimitive.Popup
          aria-describedby={undefined}
          onClick={closeOnContentClick ? () => onOpenChange(false) : undefined}
          className={cn(
            "fixed inset-0 z-[var(--layer-modal)] m-auto h-fit w-fit max-h-[90vh] max-w-[min(1400px,95vw)] overflow-auto rounded-[var(--floating-panel-radius)] bg-card shadow-[var(--shadow-modal)] outline-none",
            FLOATING_MOTION,
            closeOnContentClick && "cursor-zoom-out",
            className,
          )}
        >
          <DialogPrimitive.Title className="sr-only">{title}</DialogPrimitive.Title>
          {children}
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
