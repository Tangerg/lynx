import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

/**
 * The opaque content surface that floats as a card over the drawer.
 *
 * The 14.4px seam-side radius, clipped 1px inset ring, backing wedge, and depth
 * shadow live together in globals.css. They transition off as one object when
 * the drawer collapses, leaving the native window corner as the only curve.
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
