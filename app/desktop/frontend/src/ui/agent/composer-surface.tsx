import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

/**
 * The composer's card.
 *
 * A real 1px border, not a shadow ring: the composer overlaps the scrolling
 * transcript, and an optical ring plus a border would read as a double edge at
 * exactly the spot the eye is drawn to. The fill is a control surface rather than
 * the card color, so the composer reads as something you type INTO instead of a
 * floating panel that happens to contain a textarea.
 *
 * Carries no padding: the editor and the footer own their own insets, because the
 * footer sits flush to the card's edges and shared padding here would push it
 * inward.
 */
export function AgentComposerSurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      className={cn(
        "rounded-composer border border-field-strong bg-surface-2",
        "shadow-[var(--shadow-composer)]",
        "transition-[border-color,box-shadow] duration-200 ease-out",
        "focus-within:shadow-[var(--shadow-popover)]",
        className,
      )}
    >
      {children}
    </div>
  );
}
