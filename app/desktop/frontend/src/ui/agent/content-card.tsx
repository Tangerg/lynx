import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

/**
 * The shell's flush reading plane.
 *
 * The shell owns region material and its single leading divider. Pages compose
 * business content inside this plane without learning how the drawer collapses
 * or how a visual-style contribution paints the boundary.
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
