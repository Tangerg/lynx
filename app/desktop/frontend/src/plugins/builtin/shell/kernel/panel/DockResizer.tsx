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
import {
  clampDockWidth,
  DOCK_MIN_WIDTH_PX,
  maxDockWidth,
  type DockDensity,
} from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { DOCK_WIDTH_PROPERTY } from "./dockWidth";

export function DockResizer({ density }: { density: DockDensity }) {
  const t = useT();
  // The drag settles into the width slot for the material on screen — resizing
  // for a diff must not move the width every list opens at.
  const { width: persistedWidth, setWidth } = useDockWidth(density);
  const railRef = useRef<HTMLDivElement>(null);
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
        window.removeEventListener("pointercancel", listeners.up);
        listenersRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    const rail = railRef.current;
    const row = rail?.parentElement;
    if (!rail || !row) return;
    const syncRange = () => {
      rail.setAttribute("aria-valuemax", String(maxDockWidth(row.clientWidth)));
      rail.setAttribute("aria-valuenow", String(clampDockWidth(persistedWidth, row.clientWidth)));
    };
    syncRange();
    const observer = new ResizeObserver(syncRange);
    observer.observe(row);
    return () => observer.disconnect();
  }, [persistedWidth]);

  const onPointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (e.button !== 0) return;
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
        window.removeEventListener("pointercancel", prev.up);
      }

      const startX = e.clientX;
      const startWidth = readDockWidth(row);
      let width = startWidth;
      const move = (ev: PointerEvent) => {
        const element = rowRef.current;
        if (!element) return;
        width = clampDockWidth(startWidth + startX - ev.clientX, element.clientWidth);
        element.style.setProperty(DOCK_WIDTH_PROPERTY, `${width}px`);
        railRef.current?.setAttribute("aria-valuenow", String(width));
      };
      const up = () => {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", up);
        window.removeEventListener("pointercancel", up);
        listenersRef.current = null;
        rowRef.current = null;
        setWidth(width);
      };
      listenersRef.current = { move, up };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
      window.addEventListener("pointercancel", up);
    },
    [setWidth],
  );

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      const rail = railRef.current;
      const row = rail?.parentElement;
      if (!rail || !row) return;
      event.preventDefault();

      const max = maxDockWidth(row.clientWidth);
      const current = readDockWidth(row);
      const step = event.shiftKey ? 24 : 8;
      const next =
        event.key === "Home"
          ? DOCK_MIN_WIDTH_PX
          : event.key === "End"
            ? max
            : clampDockWidth(current + (event.key === "ArrowLeft" ? step : -step), row.clientWidth);

      row.style.setProperty(DOCK_WIDTH_PROPERTY, `${next}px`);
      rail.setAttribute("aria-valuemax", String(max));
      rail.setAttribute("aria-valuenow", String(next));
      setWidth(next);
    },
    [setWidth],
  );

  return (
    <div
      ref={railRef}
      // A draggable vertical splitter IS role="separator" per ARIA; an <hr> is
      // a non-interactive horizontal thematic break — wrong for a resize handle.
      role="separator"
      tabIndex={0}
      aria-orientation="vertical"
      aria-label={t("dock.action.resize")}
      aria-valuemin={DOCK_MIN_WIDTH_PX}
      aria-valuenow={Math.round(persistedWidth)}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      className="agent-pane-resizer relative w-2 shrink-0 cursor-col-resize touch-none"
    />
  );
}

function readDockWidth(row: HTMLElement): number {
  const dock = row.querySelector<HTMLElement>(".agent-context-dock");
  const renderedWidth = dock?.getBoundingClientRect().width ?? 0;
  if (renderedWidth > 0) return clampDockWidth(renderedWidth, row.clientWidth);
  const propertyWidth = Number.parseFloat(
    getComputedStyle(row).getPropertyValue(DOCK_WIDTH_PROPERTY),
  );
  return clampDockWidth(Number.isFinite(propertyWidth) ? propertyWidth : 0, row.clientWidth);
}
