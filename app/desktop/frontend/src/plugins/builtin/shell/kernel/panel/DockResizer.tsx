// Drag handle between the chat stream and the context dock. The gesture lives in the
// `ResizeHandle` atom (DESIGN §4 exemption: Base UI has no split-pane primitive, and
// this is interactive chrome rather than a decorative divider — idle shows nothing but a
// col-resize cursor). What is declared here is only what is true of this seam.

import { clampDockWidth, DOCK_MIN_WIDTH_PX, maxDockWidth } from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { ResizeHandle } from "@/ui";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { DOCK_WIDTH_PROPERTY } from "./dockWidth";

export function DockResizer() {
  const t = useT();
  const { width, setWidth } = useDockWidth();

  return (
    <ResizeHandle
      aria-label={t("dock.action.resize")}
      className="agent-pane-resizer"
      edge="start"
      value={width}
      container={(rail) => rail.parentElement}
      property={DOCK_WIDTH_PROPERTY}
      read={readDockWidth}
      minWidth={DOCK_MIN_WIDTH_PX}
      maxWidth={maxDockWidth}
      onCommit={setWidth}
    />
  );
}

/**
 * The dock's live width, preferring what it actually occupies.
 *
 * The rendered width is the truth a drag has to start from: the column is laid out by
 * the row's grid, so it can end up narrower than the property asked for. The property is
 * the fallback for the one case where there is no rendered width to read — before the
 * dock has been laid out at all.
 */
function readDockWidth(row: HTMLElement): number {
  const dock = row.querySelector<HTMLElement>(".agent-context-dock");
  const renderedWidth = dock?.getBoundingClientRect().width ?? 0;
  if (renderedWidth > 0) return clampDockWidth(renderedWidth, row.clientWidth);
  const propertyWidth = Number.parseFloat(
    getComputedStyle(row).getPropertyValue(DOCK_WIDTH_PROPERTY),
  );
  return clampDockWidth(Number.isFinite(propertyWidth) ? propertyWidth : 0, row.clientWidth);
}
