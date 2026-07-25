import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

/**
 * The opaque content surface that floats as a card over the drawer.
 *
 * The rounded seam corner, the 1px inset ring that divides it from the drawer,
 * and the depth shadow all live in globals.css and switch off together when the
 * drawer collapses — at that point the card reaches the window edge, and its
 * corner would double up with the OS window's own. `overflow: hidden` clips
 * children to the rounded edge, which is also why the corner wedge has to be
 * backed by the parent rather than filled from in here.
 */
export function AgentContentCard({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"main">) {
  return (
    <main aria-label="Agent workspace" {...props} className={cn("agent-content-card", className)}>
      {children}
    </main>
  );
}
