import type { ReactNode } from "react";
import { useId, useLayoutEffect, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import { DialogPrimitive } from "@/ui/primitives";
import { Icon } from "@/ui/icons";
import { FLOATING_MOTION, MODAL_SCRIM } from "./floating-surface";
import { Kbd } from "./kbd";
import { OptionRow } from "./option-row";
import { TextField } from "./text-field";

export interface SearchOption {
  /** React key. The row's DOM id is derived from position, which is all
   *  `aria-activedescendant` needs and cannot be broken by a key holding a space. */
  key: string;
  onSelect: () => void;
  children: ReactNode;
}

interface SearchOverlayProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Names the dialog and its list, so a screen reader hears the same thing on the
   *  way in and while arrowing. */
  label: string;
  placeholder: string;
  /** What the typed query matches. Called on every render with the current query:
   *  the overlay owns the query so that closing resets it, the highlight and the
   *  scroll position together. */
  options: (query: string) => readonly SearchOption[];
  empty: ReactNode;
  /** Restores the control or editor that opened a controlled dialog without a
   *  Base UI trigger in the same React tree. */
  finalFocus?: () => HTMLElement | null;
}

function wrap(index: number, count: number, step: number) {
  if (count === 0) return 0;
  return (index + step + count) % count;
}

/**
 * A type-to-find overlay: scrim, panel dropped from the top, one search row, and a
 * list of options under it.
 *
 * The whole listbox is in here, rows included, because the invariants that bind the
 * field to the list cannot be met from one side of the boundary: the field announces
 * the active row through `aria-activedescendant`, so it needs that row's id; focus
 * never leaves the field, so the rows must not be tab stops; and the active row has
 * to be scrolled to. A caller rendering its own rows silently owed all three, and the
 * first one to do so paid none of them.
 */
export function SearchOverlay({
  open,
  onOpenChange,
  label,
  placeholder,
  options,
  empty,
  finalFocus,
}: SearchOverlayProps) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop data-slot="search-overlay-backdrop" className={MODAL_SCRIM} />
        <DialogPrimitive.Popup
          data-slot="search-overlay"
          aria-label={label}
          finalFocus={finalFocus}
          className={cn(
            "fixed inset-x-0 top-24 z-[var(--layer-modal)] mx-auto flex w-[min(520px,calc(100vw-32px))]",
            "flex-col overflow-hidden rounded-[var(--floating-panel-radius)] outline-none",
            // Opaque, and the modal shadow — this is a modal, so it gives the answer
            // ConfirmDialog gives. The ring's frosted fill is for a popover: small,
            // anchored, read as glass. Over a whole transcript at 520px it was a
            // window onto the prose underneath.
            "bg-canvas shadow-[var(--shadow-modal)]",
            FLOATING_MOTION,
          )}
        >
          <SearchOverlayContent
            key={open ? "open" : "closed"}
            open={open}
            label={label}
            placeholder={placeholder}
            options={options}
            empty={empty}
          />
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

type SearchOverlayContentProps = Omit<SearchOverlayProps, "onOpenChange" | "finalFocus">;

function SearchOverlayContent({
  open,
  label,
  placeholder,
  options,
  empty,
}: SearchOverlayContentProps) {
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const baseId = useId();
  const listboxId = `${baseId}-list`;

  const rows = options(query);
  // Clamped on read, not stored clamped: one more character can shorten the list
  // under an index held in state, and a stale index renders as no highlight at all
  // and an Enter that opens nothing.
  const active = rows.length === 0 ? 0 : Math.min(Math.max(highlight, 0), rows.length - 1);
  const activeId = rows.length === 0 ? undefined : `${baseId}-${active}`;

  // The list is taller than its box, so a highlight the keyboard moved past the
  // eighth row is a highlight nobody can see.
  useLayoutEffect(() => {
    if (!open) return;
    listRef.current?.querySelector("[aria-selected='true']")?.scrollIntoView({ block: "nearest" });
  }, [activeId, open]);

  return (
    <div className="contents">
      <div className="flex items-center gap-2.5 border-b border-line-soft px-3.5 py-2.5 text-fg-muted">
        <Icon name="search" size="md" />
        <TextField
          variant="bare"
          // A session title is prose; the default mono is for paths and patterns.
          font="sans"
          // The surface exists to be typed into, and the user opened it with a
          // keystroke — landing anywhere else would be the surprise.
          // oxlint-disable-next-line jsx-a11y/no-autofocus
          autoFocus={open}
          role="combobox"
          aria-expanded
          aria-controls={listboxId}
          aria-activedescendant={activeId}
          value={query}
          onKeyDown={(event) => {
            // Candidate navigation and acceptance belong to the IME while it is
            // composing; the search list takes over only after commit.
            if (event.nativeEvent.isComposing) return;
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              event.preventDefault();
              setHighlight(wrap(active, rows.length, event.key === "ArrowDown" ? 1 : -1));
              return;
            }
            if (event.key === "Enter") {
              event.preventDefault();
              rows[active]?.onSelect();
            }
          }}
          onChange={(event) => {
            setQuery(event.target.value);
            setHighlight(0);
          }}
          placeholder={placeholder}
          aria-label={placeholder}
          className="flex-1"
        />
        <Kbd>esc</Kbd>
      </div>
      <div
        ref={listRef}
        id={listboxId}
        role="listbox"
        aria-label={label}
        className="max-h-80 overflow-y-auto p-1.5"
      >
        {rows.length === 0
          ? empty
          : rows.map((option, index) => (
              <OptionRow
                key={option.key}
                id={`${baseId}-${index}`}
                layout="flex"
                size="lg"
                tabIndex={-1}
                selected={index === active}
                onPointerMove={() => setHighlight(index)}
                onClick={option.onSelect}
              >
                {option.children}
              </OptionRow>
            ))}
      </div>
    </div>
  );
}
