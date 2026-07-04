import type { ReactElement, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { DialogPrimitive } from "@/ui/primitives";

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
        <DialogPrimitive.Backdrop className="fixed inset-0 z-[200] cursor-zoom-out bg-black/60 light:bg-black/25" />
        <DialogPrimitive.Popup
          aria-describedby={undefined}
          onClick={closeOnContentClick ? () => onOpenChange(false) : undefined}
          className={cn(
            "fixed inset-0 z-[201] m-auto h-fit w-fit max-h-[90vh] max-w-[min(1400px,95vw)] overflow-auto rounded-lg bg-surface shadow-[var(--shadow-popover)] outline-none data-[open]:animate-rise-in",
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
