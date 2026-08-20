import type { ReactNode } from "react";
import { useLayoutEffect, useRef } from "react";
import { clampSidebarWidth } from "@/lib/shellGeometry";
import { AgentSeamRail, AgentSidebar, SIDEBAR_WIDTH_PROPERTY } from "./sidebar";
import { AgentDrawerToggle } from "./surface-header";

interface AgentAppShellProps {
  /** Work-index content. Omit to run without a drawer (settings takes over the window). */
  sidebar?: ReactNode;
  /** Assistive-tech name for the drawer region. The shell renders the region; the
   *  application says what it is. */
  sidebarLabel: string;
  /** Assistive-tech name for the drawer resize separator. */
  sidebarResizeLabel: string;
  /** Drawer open state. Collapsing slides it behind the reading plane. */
  sidebarOpen: boolean;
  /** Persisted drawer width in px; the rail commits new values through `onResize`. */
  sidebarWidth: number;
  onResize: (width: number) => void;
  onSidebarToggle: () => void;
  sidebarExpandLabel: string;
  sidebarCollapseLabel: string;
  main: ReactNode;
  overlay?: ReactNode;
}

/**
 * The window shell: one resizable work index, one flush reading plane, overlays.
 *
 * The drawer's width lives as a custom property on this element so the rail can
 * drag it without a React render, and so the spacer and the panel read one number.
 * Collapse is a single attribute flip that both of them and the plane's divider
 * transition off. Region geometry is deliberately independent from page content.
 */
export function AgentAppShell({
  sidebar,
  sidebarLabel,
  sidebarResizeLabel,
  sidebarOpen,
  sidebarWidth,
  onResize,
  onSidebarToggle,
  sidebarExpandLabel,
  sidebarCollapseLabel,
  main,
  overlay,
}: AgentAppShellProps) {
  const shellRef = useRef<HTMLDivElement>(null);
  const hasSidebar = sidebar !== undefined;

  // The rail writes the width directly while the user is moving it. This effect
  // re-syncs it with the persisted preference afterwards and re-clamps that
  // preference whenever the window changes size. The preference itself is not
  // overwritten by a temporary narrow window, so widening restores the user's
  // chosen width.
  useLayoutEffect(() => {
    const shell = shellRef.current;
    if (!shell) return;
    const syncWidth = () => {
      // Not mid-gesture. The observer below fires on any layout change, and a
      // resize IS one — so under load it could land between two pointer-moves
      // and snap the drawer back to the value not yet committed. The rail marks
      // the shell for exactly this window.
      if (shell.hasAttribute("data-resizing")) return;
      shell.style.setProperty(
        SIDEBAR_WIDTH_PROPERTY,
        `${clampSidebarWidth(sidebarWidth, shell.clientWidth)}px`,
      );
    };
    syncWidth();
    const observer = new ResizeObserver(syncWidth);
    observer.observe(shell);
    return () => observer.disconnect();
  }, [sidebarWidth]);

  return (
    <div
      ref={shellRef}
      className="agent-shell"
      data-sidebar={hasSidebar && sidebarOpen ? "expanded" : "collapsed"}
    >
      {hasSidebar && <AgentSidebar label={sidebarLabel}>{sidebar}</AgentSidebar>}
      {hasSidebar && (
        <div className="agent-window-sidebar-control">
          <AgentDrawerToggle
            collapsed={!sidebarOpen}
            onToggle={onSidebarToggle}
            expandLabel={sidebarExpandLabel}
            collapseLabel={sidebarCollapseLabel}
          />
        </div>
      )}
      <div className="agent-card-backing">
        {hasSidebar && sidebarOpen && (
          <AgentSeamRail label={sidebarResizeLabel} width={sidebarWidth} onCommit={onResize} />
        )}
        {main}
      </div>
      {overlay}
    </div>
  );
}
