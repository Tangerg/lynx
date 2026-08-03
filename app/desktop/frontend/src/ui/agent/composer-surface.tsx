import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

/**
 * The composer's card — a pane of glass resting on the transcript.
 *
 * ONE edge mechanism, and it is a shadow. A real 1px border used to own it, which
 * is correct for a control that sits IN a surface and wrong for one that floats
 * over a document: a hard line drew a box around the input while the three region
 * seams around it separate by cast, so the composer was the only object on screen
 * still outlined. The ring in `--shadow-composer-depth` implies the edge and the
 * cast under it puts the panel above the page; both live in that one token, so a
 * style that wants a drawn border spells it there and nothing here changes.
 *
 * Translucent, and it has to be for the ring to read as glass rather than as a
 * stroke: the material picks up whatever passes underneath, which is why the
 * overlay behind it paints nothing and the transcript dissolves under it instead.
 *
 * Focus takes the ring to the accent and leaves the surface where it is. The
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
        "agent-composer-glass overflow-hidden rounded-composer",
        "transition-[box-shadow] duration-[var(--dur-med)] ease-out",
        className,
      )}
    >
      {children}
    </div>
  );
}
