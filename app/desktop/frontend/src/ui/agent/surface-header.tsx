import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui";

interface AgentSurfaceHeaderProps extends ComponentPropsWithoutRef<"div"> {
  /** Drop the bottom hairline — for a bar that already butts against one. */
  divider?: boolean;
  /**
   * This bar can reach the window's top-left corner, so it yields the stable
   * window-control strip while the drawer is collapsed. Set it on the first bar
   * of whatever fills the content card; CSS decides when the corner is exposed.
   */
  windowCorner?: boolean;
}

/**
 * A chrome bar: fixed height, standard inset, bottom hairline — and the
 * frameless window's drag handle.
 *
 * Height, inset and drag region live in globals.css because the collapsed-drawer
 * state has to clear the stable window controls. Interactive children opt out
 * there too, so a new button cannot silently become undraggable window chrome.
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
 * The plane-level dock toggle — the drawer toggle's mirror, and for the same reason.
 *
 * The flank used to be opened by a button in the conversation's bar and closed by a
 * different button in its own, which is one affordance in one place (the plane's
 * trailing corner) implemented as two controls that changed owner. The handover was
 * visible: hiding the flank mounted the first button and shifted the bar's trailing
 * cluster 26px on the frame the travel began. It was also the wrong ergonomics — a
 * toggle that belongs to whichever bar reaches the corner is a toggle that moves out
 * from under the cursor that just used it.
 */
export function AgentDockToggle({
  open,
  onToggle,
  showLabel,
  hideLabel,
}: {
  open: boolean;
  onToggle: () => void;
  showLabel: string;
  hideLabel: string;
}) {
  return (
    <IconButton
      icon="panel-r"
      // Only while it is showing: the glyph promises what the click does, and there is
      // nothing to close when the flank is already away.
      hoverIcon={open ? "x" : undefined}
      size="sm"
      aria-expanded={open}
      title={open ? hideLabel : showLabel}
      onClick={onToggle}
    />
  );
}

/**
 * The window-level drawer toggle. AgentAppShell keeps this single instance at a
 * stable coordinate instead of handing ownership between sidebar and page bars.
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
  return (
    <IconButton
      icon="panel-l"
      size="sm"
      data-drawer-toggle=""
      aria-expanded={!collapsed}
      aria-label={collapsed ? expandLabel : collapseLabel}
      onClick={onToggle}
    />
  );
}
