import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

/**
 * The composer's card.
 *
 * No CSS border. The edge is the first layer of the shadow, drawn from the same
 * `--seam-line` token as the drawer↔card divider, so the whole app has ONE
 * hairline value — and so this floating surface has one edge mechanism rather than
 * a border fighting a shadow ring at exactly the spot the eye is drawn to. The
 * ring also follows the corner radius, which a border does more coarsely.
 *
 * The fill is the CARD colour, not a recessed grey: a grey slab sitting on a white
 * reading column reads heavy however the grey is tuned. What makes the composer
 * read as a control is the ring plus the ambient shadow, so the interior stays as
 * clean as the page.
 *
 * Focus strengthens the ring but keeps the ambient layer where it is — lifting the
 * whole surface on focus makes every click read as the composer jumping.
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
        "rounded-composer bg-canvas",
        "shadow-[var(--shadow-composer)] focus-within:shadow-[var(--shadow-composer-focus)]",
        "transition-[box-shadow] duration-200 ease-out",
        className,
      )}
    >
      {children}
    </div>
  );
}
