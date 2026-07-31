import type { ComponentPropsWithoutRef, MouseEvent } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui";

interface AgentSurfaceHeaderProps extends ComponentPropsWithoutRef<"div"> {
  /** Drop the bottom hairline — for a bar that already butts against one. */
  divider?: boolean;
  /**
   * This bar can reach the window's top-left corner, so it yields the slot the
   * macOS traffic lights occupy while the drawer is collapsed. Set it on the
   * first bar of whatever fills the content card; the CSS decides when the
   * corner is actually exposed.
   */
  windowCorner?: boolean;
}

/**
 * A chrome bar: fixed height, standard inset, bottom hairline — and the
 * frameless window's drag handle.
 *
 * Height, inset and the drag region live in globals.css because the
 * collapsed-drawer state has to widen the inset to clear the macOS traffic
 * lights, and an unlayered rule is the only thing that can outrank a utility
 * class at the call site. Interactive children opt out of the drag there too, so
 * a new button in a header can't silently become undraggable window chrome.
 */
export function AgentSurfaceHeader({
  divider = true,
  windowCorner,
  className,
  children,
  ...props
}: AgentSurfaceHeaderProps) {
  return (
    <div
      {...props}
      data-window-corner={windowCorner ? "" : undefined}
      className={cn("agent-surface-header", divider && "agent-surface-divider", className)}
    >
      {children}
    </div>
  );
}

/**
 * The drawer's collapse toggle, as it appears in a chrome bar.
 *
 * One component because every bar that can own the window's top-left corner
 * needs it — the chat's header and a full-width view's header. When only the
 * chat had one, collapsing the drawer and then maximising a view left no way
 * back to the session list but a keyboard shortcut.
 */
export function AgentDrawerToggle({
  collapsed,
  onToggle,
  expandLabel,
  collapseLabel,
}: {
  collapsed: boolean;
  onToggle: () => void;
  expandLabel: string;
  collapseLabel: string;
}) {
  const handleToggle = (event: MouseEvent<HTMLButtonElement>) => {
    const shell = event.currentTarget.closest<HTMLElement>(".agent-shell");
    const source = event.currentTarget;
    onToggle();
    // The visible toggle moves between the drawer and the content header.
    // Preserve keyboard focus across that ownership handoff instead of dropping
    // it onto <body> when the old control slides out or unmounts.
    requestAnimationFrame(() => {
      const target = [...(shell?.querySelectorAll<HTMLButtonElement>("[data-drawer-toggle]") ?? [])]
        .filter((button) => button !== source)
        .find((button) => {
          const style = getComputedStyle(button);
          return style.display !== "none" && style.visibility !== "hidden";
        });
      target?.focus();
    });
  };

  return (
    <IconButton
      icon="panel-l"
      size="sm"
      data-drawer-toggle=""
      aria-expanded={!collapsed}
      aria-label={collapsed ? expandLabel : collapseLabel}
      onClick={handleToggle}
    />
  );
}
