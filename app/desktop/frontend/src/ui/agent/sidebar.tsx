import type { ReactNode } from "react";
import { clampSidebarWidth, maxSidebarWidth, SIDEBAR_MIN_WIDTH_PX } from "@/lib/shellGeometry";
import { ResizeHandle } from "@/ui/atoms/resize-handle";

/**
 * The work-index drawer: an in-flow spacer that reserves the width, plus a
 * fixed-position panel that slides. Both read `--sidebar-width` from the shell,
 * so a resize is one custom-property write and a collapse is one attribute flip.
 *
 * `label` names the region for assistive tech and comes from the caller: what
 * this drawer holds is the application's business, and a design-system ring that
 * knows the phrase "work index" is a ring that knows the product.
 */
export function AgentSidebar({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <div className="agent-drawer-gap" aria-hidden />
      <aside aria-label={label} className="agent-drawer">
        <div className="agent-drawer-surface">{children}</div>
      </aside>
    </>
  );
}

/**
 * Resize separator for the drawer↔reading-plane seam. It draws no resting line:
 * the reading plane owns that boundary, while this rail only strengthens the
 * same coordinate on hover, focus and drag.
 *
 * The gesture belongs to the `ResizeHandle` atom; what is declared here is only what
 * is true of this seam — which side of the drawer it sits on, where the width lives,
 * and that the drawer animates its own width and so must be told to stop while the
 * user is moving it.
 */
export function AgentSeamRail({
  label,
  width,
  onCommit,
}: {
  label: string;
  width: number;
  onCommit: (width: number) => void;
}) {
  return (
    <ResizeHandle
      aria-label={label}
      className="agent-seam-rail"
      edge="end"
      value={width}
      container={(rail) => rail.closest<HTMLElement>(".agent-shell")}
      property={SIDEBAR_WIDTH_PROPERTY}
      read={readSidebarWidth}
      minWidth={SIDEBAR_MIN_WIDTH_PX}
      maxWidth={maxSidebarWidth}
      onCommit={onCommit}
      resizingAttribute="data-resizing"
    />
  );
}

/** Where the drawer's width lives. The rail writes it during a gesture and the shell
 *  re-syncs it from the persisted preference, so both must spell it the same way. */
export const SIDEBAR_WIDTH_PROPERTY = "--sidebar-width";

function readSidebarWidth(shell: HTMLElement): number {
  const value = Number.parseFloat(getComputedStyle(shell).getPropertyValue(SIDEBAR_WIDTH_PROPERTY));
  return clampSidebarWidth(Number.isFinite(value) ? value : 0, shell.clientWidth);
}
