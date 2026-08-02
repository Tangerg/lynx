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
 * The fill is the CARD colour, not a recessed grey: a grey slab sitting on a white
 * reading column reads heavy however the grey is tuned. What makes the composer
 * read as a control is the edge plus the ambient shadow, so the interior stays
 * as clean as the page.
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
        "rounded-composer border-[length:var(--composer-edge-width)] border-field bg-canvas",
        "shadow-[var(--shadow-composer-depth)] focus-within:border-field-strong",
        "transition-[border-color] duration-[var(--dur-med)] ease-out",
        className,
      )}
    >
      {children}
    </div>
  );
}
