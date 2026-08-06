import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button } from "./button";
import { DialogPrimitive } from "@/ui/primitives";
import { FLOATING_MOTION, MODAL_SCRIM } from "./floating-surface";

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  /** What the action will do, in the user's words. Rendered as the dialog's
   *  description, so it is announced with the title — which is also why it is
   *  required: a confirmation with nothing but a verb is not one. */
  body: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  /** Colour the confirm button as a loss. */
  destructive?: boolean;
  onConfirm: () => void;
}

/**
 * The last step before something the user cannot undo.
 *
 * Controlled and trigger-less: what needs confirming is usually a menu item that
 * has already closed by the time this opens, so the caller owns `open`. Copy is
 * passed in — the design system draws the shape, the feature knows what it is
 * about to destroy.
 */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  body,
  confirmLabel,
  cancelLabel,
  destructive,
  onConfirm,
}: ConfirmDialogProps) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop data-slot="confirm-dialog-backdrop" className={MODAL_SCRIM} />
        <DialogPrimitive.Popup
          data-slot="confirm-dialog"
          className={cn(
            "fixed inset-0 z-[var(--layer-modal)] m-auto h-fit w-[min(400px,calc(100vw-32px))]",
            "rounded-[var(--floating-panel-radius)] bg-canvas p-4 shadow-[var(--shadow-modal)] outline-none",
            FLOATING_MOTION,
          )}
        >
          <DialogPrimitive.Title className="text-display-sm font-semibold text-fg">
            {title}
          </DialogPrimitive.Title>
          <DialogPrimitive.Description className="mt-1.5 text-ui-md leading-relaxed text-fg-muted">
            {body}
          </DialogPrimitive.Description>
          <div className="mt-4 flex items-center justify-end gap-2">
            <DialogPrimitive.Close render={<Button variant="ghost">{cancelLabel}</Button>} />
            <Button
              variant={destructive ? "tonal" : "primary"}
              tone={destructive ? "negative" : undefined}
              onClick={() => {
                onOpenChange(false);
                onConfirm();
              }}
            >
              {confirmLabel}
            </Button>
          </div>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
