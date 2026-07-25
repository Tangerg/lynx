import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

/**
 * The opaque content surface that floats as a card over the drawer.
 *
 * The 1px line that divides it from the drawer and the depth shadow both live in
 * globals.css, and both switch off when the drawer collapses — at that point the
 * card reaches the window edge and there is no drawer left to divide from. The seam
 * side is deliberately square: a corner arc there resolves over so few pixels of
 * height that it reads as a kink in the divider rather than a curve.
 *
 * `label` names the region for assistive tech and comes from the caller — see
 * AgentSidebar for why the design system does not get to name it.
 */
export function AgentContentCard({
  label,
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"main"> & { label: string }) {
  return (
    <main aria-label={label} {...props} className={cn("agent-content-card", className)}>
      {children}
    </main>
  );
}
