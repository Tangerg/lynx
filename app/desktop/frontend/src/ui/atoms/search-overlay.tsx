import type { KeyboardEvent, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { DialogPrimitive } from "@/ui/primitives";
import { Icon } from "@/ui/icons";
import { Kbd } from "./kbd";
import { TextField } from "./text-field";

interface SearchOverlayProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Names the dialog AND the list inside it — one label for one surface, so a
   *  screen reader hears the same thing on the way in and while arrowing. */
  label: string;
  value: string;
  onValueChange: (value: string) => void;
  placeholder: string;
  /** Arrow/Enter handling. The overlay owns the field and the scroll box; which
   *  row is highlighted and what Enter means belong to whoever has the list. */
  onKeyDown?: (event: KeyboardEvent<HTMLDivElement>) => void;
  className?: string;
  children: ReactNode;
}

/**
 * The shell of a type-to-find overlay: a scrim, a panel dropped from the top, a
 * bare search row, and a scrolling list under it.
 *
 * Extracted as an atom rather than left in the one feature that uses it, because
 * the previous version of this shape lived inside a feature and paid for it: the
 * panel was hand-spelled, the field was a native `<input>`, and the scrim was a
 * class on a variant that could never reach the element it named — so there was
 * no scrim at all and nobody could see that from the code. The ring draws the
 * shape; the feature says what is in the list.
 *
 * `SearchField` is the boxed sibling of this row and deliberately not reused: a
 * field with its own edge inside a panel is two edges around one input.
 */
export function SearchOverlay({
  open,
  onOpenChange,
  label,
  value,
  onValueChange,
  placeholder,
  onKeyDown,
  className,
  children,
}: SearchOverlayProps) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop
          data-slot="search-overlay-backdrop"
          className="fixed inset-0 z-50 bg-scrim"
        />
        <DialogPrimitive.Popup
          data-slot="search-overlay"
          aria-label={label}
          onKeyDown={onKeyDown}
          className={cn(
            "fixed inset-x-0 top-24 z-50 mx-auto flex w-[min(520px,calc(100vw-32px))] flex-col",
            "overflow-hidden rounded-[var(--floating-panel-radius)] outline-none",
            "bg-[var(--app-floating-surface)] shadow-[var(--shadow-popover)] data-[open]:animate-rise-in",
            className,
          )}
        >
          <div className="flex items-center gap-2.5 border-b border-line-soft px-3.5 py-2.5 text-fg-muted">
            <Icon name="search" size="md" />
            <TextField
              variant="bare"
              autoFocus
              value={value}
              onChange={(event) => onValueChange(event.target.value)}
              placeholder={placeholder}
              aria-label={placeholder}
              className="flex-1"
            />
            <Kbd>esc</Kbd>
          </div>
          <div role="listbox" aria-label={label} className="max-h-80 overflow-y-auto p-1.5">
            {children}
          </div>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
