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
 * Focus takes the border to the accent and leaves the surface where it is. The
 * accent is this language's "live" colour and the composer is where live starts;
 * lifting the whole surface instead would make every click read as the composer
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
        "overflow-hidden rounded-composer border-[length:var(--composer-edge-width)] border-field bg-card",
        "shadow-[var(--shadow-composer-depth)] focus-within:border-accent",
        "transition-[border-color] duration-[var(--dur-med)] ease-out",
        className,
      )}
    >
      {children}
    </div>
  );
}
