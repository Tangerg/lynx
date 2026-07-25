import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

interface AgentSurfaceHeaderProps extends ComponentPropsWithoutRef<"div"> {
  /** Drop the bottom hairline — for a bar that already butts against one. */
  divider?: boolean;
}

/**
 * A chrome bar: fixed height, standard inset, bottom hairline.
 *
 * Height comes from `--surface-header-height`, the same value the drawer's own
 * header uses, so the two align across the seam. Both the height and the inset
 * live in globals.css because the collapsed-drawer state has to widen the inset
 * to clear the macOS traffic lights, and an unlayered rule is the only thing
 * that can outrank a utility class at the call site.
 */
export function AgentSurfaceHeader({
  divider = true,
  className,
  children,
  ...props
}: AgentSurfaceHeaderProps) {
  return (
    <div
      {...props}
      className={cn("agent-surface-header", divider && "agent-surface-divider", className)}
    >
      {children}
    </div>
  );
}
