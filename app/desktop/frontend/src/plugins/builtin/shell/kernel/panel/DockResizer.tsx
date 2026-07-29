// Drag handle between the chat stream and the context dock. Self-written because
// Base UI has no split-pane primitive (DESIGN §4 exemption: this is interactive
// chrome, not a decorative divider — idle shows nothing but a col-resize cursor;
// the accent guide appears only on hover/drag).
//
// During the drag the width goes straight onto the row as `--dock-width`, and the
// store hears about it once, on release. State per pointer-move re-rendered
// ChatPanel — the transcript, the composer and the dock's view with it — at
// pointer frequency, which is the mistake AgentSeamRail already documents.

import { useCallback, useEffect, useRef } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { clampDockWidth, type DockDensity } from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { DOCK_WIDTH_PROPERTY } from "./dockWidth";

export function DockResizer({ density }: { density: DockDensity }) {
  const t = useT();
  // The drag settles into the width slot for the material on screen — resizing
  // for a diff must not move the width every list opens at.
  const { setWidth } = useDockWidth(density);
  // Track the row element so `move` re-reads getBoundingClientRect on each
  // event — if the window resizes mid-drag, the stale-captured rect would
  // otherwise compute an incorrect width.
  const rowRef = useRef<HTMLElement | null>(null);
  // Track attached listeners so the unmount cleanup can detach them even
  // when the pointerup event fires outside the window (or never fires).
  const listenersRef = useRef<{ move: (ev: PointerEvent) => void; up: () => void } | null>(null);

  // Clean up window listeners on unmount — the `pointerup` handler normally
  // does this, but if the component unmounts mid-drag the listeners would leak.
  useEffect(() => {
    return () => {
      const listeners = listenersRef.current;
      if (listeners) {
        window.removeEventListener("pointermove", listeners.move);
        window.removeEventListener("pointerup", listeners.up);
        listenersRef.current = null;
      }
    };
  }, []);

  const onPointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      e.preventDefault();
      const row = e.currentTarget.parentElement;
      if (!row) return;
      rowRef.current = row;

      // Remove any stale listeners from a previous drag that escaped cleanup
      // (e.g. a drag ended while the browser tab was hidden).
      const prev = listenersRef.current;
      if (prev) {
        window.removeEventListener("pointermove", prev.move);
        window.removeEventListener("pointerup", prev.up);
      }

      let width = 0;
      const move = (ev: PointerEvent) => {
        const element = rowRef.current;
        const rect = element?.getBoundingClientRect();
        if (!element || !rect) return;
        width = clampDockWidth(rect.right - ev.clientX, rect.width);
        element.style.setProperty(DOCK_WIDTH_PROPERTY, `${width}px`);
      };
      const up = () => {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", up);
        listenersRef.current = null;
        rowRef.current = null;
        if (width > 0) setWidth(width);
      };
      listenersRef.current = { move, up };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
    },
    [setWidth],
  );

  return (
    <div
      // A draggable vertical splitter IS role="separator" per ARIA; an <hr> is
      // a non-interactive horizontal thematic break — wrong for a resize handle.
      role="separator"
      aria-orientation="vertical"
      aria-label={t("dock.action.resize")}
      onPointerDown={onPointerDown}
      className="agent-pane-resizer relative w-2 shrink-0 cursor-col-resize touch-none"
    />
  );
}
