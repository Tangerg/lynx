import type { ReactNode } from "react";
import { useEffect, useRef } from "react";
import { clampSidebarWidth } from "@/lib/shellGeometry";
import { AgentSeamRail, AgentSidebar } from "./sidebar";

interface AgentAppShellProps {
  /** Work-index content. Omit to run without a drawer (settings takes over the window). */
  sidebar?: ReactNode;
  /** Assistive-tech name for the drawer region. The shell renders the region; the
   *  application says what it is. */
  sidebarLabel: string;
  /** Assistive-tech name for the drawer resize separator. */
  sidebarResizeLabel: string;
  /** Drawer open state. Collapsing slides it under the content card. */
  sidebarOpen: boolean;
  /** Persisted drawer width in px; the rail commits new values through `onResize`. */
  sidebarWidth: number;
  onResize: (width: number) => void;
  main: ReactNode;
  overlay?: ReactNode;
}

/**
 * The window shell: drawer, content card, overlays.
 *
 * The drawer's width lives as a custom property on this element so the rail can
 * drag it without a React render, and so the spacer and the panel read one number.
 * Collapse is a single attribute flip that both of them, plus the card's divider
 * and depth, transition off.
 */
export function AgentAppShell({
  sidebar,
  sidebarLabel,
  sidebarResizeLabel,
  sidebarOpen,
  sidebarWidth,
  onResize,
  main,
  overlay,
}: AgentAppShellProps) {
  const shellRef = useRef<HTMLDivElement>(null);
  const previousSidebarOpenRef = useRef(sidebarOpen);
  const hasSidebar = sidebar !== undefined;

  // The rail writes `--sidebar-width` directly during a drag. This effect
  // re-syncs it with the persisted preference afterwards and re-clamps that
  // preference whenever the window changes size. The preference itself is not
  // overwritten by a temporary narrow window, so widening restores the user's
  // chosen width.
  useEffect(() => {
    const shell = shellRef.current;
    if (!shell) return;
    const syncWidth = () => {
      shell.style.setProperty(
        "--sidebar-width",
        `${clampSidebarWidth(sidebarWidth, shell.clientWidth)}px`,
      );
    };
    syncWidth();
    const observer = new ResizeObserver(syncWidth);
    observer.observe(shell);
    return () => observer.disconnect();
  }, [sidebarWidth]);

  // The visible toggle changes owners when the drawer opens or closes. Perform
  // the handoff after React has committed the new owner instead of asking the
  // button click handler to guess whether one animation frame is late enough.
  // This also covers keyboard shortcuts and other application-level toggles.
  useEffect(() => {
    const previousSidebarOpen = previousSidebarOpenRef.current;
    previousSidebarOpenRef.current = sidebarOpen;
    if (!hasSidebar || previousSidebarOpen === sidebarOpen) return;

    // Let the committed visibility transition apply before focusing the owner.
    // WebKit can keep the opening drawer hidden for more than one painted frame
    // while it resolves the inherited visibility transition. Wait for actual
    // visibility instead of coupling semantics to an animation duration or an
    // arbitrary timer.
    let frame = 0;
    const focusOwner = () => {
      const target = shellRef.current?.querySelector<HTMLButtonElement>(
        `[data-drawer-toggle][aria-expanded="${sidebarOpen ? "true" : "false"}"]`,
      );
      if (!target) return;
      const style = getComputedStyle(target);
      if (style.display === "none" || style.visibility === "hidden") {
        frame = requestAnimationFrame(focusOwner);
        return;
      }
      target.focus();
    };
    frame = requestAnimationFrame(focusOwner);
    return () => cancelAnimationFrame(frame);
  }, [hasSidebar, sidebarOpen]);

  return (
    <div
      ref={shellRef}
      className="agent-shell"
      data-sidebar={hasSidebar && sidebarOpen ? "expanded" : "collapsed"}
    >
      {hasSidebar && <AgentSidebar label={sidebarLabel}>{sidebar}</AgentSidebar>}
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
