import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

/**
 * The composer's card.
 *
 * A real border owns the edge; `--shadow-composer-depth` owns depth only. Keeping
 * those roles separate makes the edge inspectable, lets focus strengthen only
 * the border colour, and prevents a shadow ring and border from doubling the
 * same pixels.
 *
 * The fill is the first chrome surface: enough separation from the reading plane
 * to read as a durable workbench control without becoming a floating web card.
 *
 * Focus strengthens the border but keeps the ambient layer where it is —
 * lifting the whole surface on focus makes every click read as the composer
 * jumping.
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
        "overflow-hidden rounded-composer border-[length:var(--composer-edge-width)] border-field bg-surface",
        "shadow-[var(--shadow-composer-depth)] focus-within:border-field-strong",
        "transition-[border-color] duration-[var(--dur-med)] ease-out",
        className,
      )}
    >
      {children}
    </div>
  );
}
