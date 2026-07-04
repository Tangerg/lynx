// Drag handle between the chat stream and a split workspace view. Self-written
// because Base UI has no split-pane primitive (DESIGN §4 exemption: this is
// interactive chrome, not a decorative divider — idle shows nothing but a
// col-resize cursor; the accent guide appears only on hover/drag). On drag it
// computes the chat's fraction of the parent row's width and persists it
// (clamped 0.25–0.75) to uiStore so the split holds across sessions/launches.

import { useCallback, useEffect, useRef } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { useT } from "@/lib/i18n";
import { useUiStore } from "@/state/uiStore";

export function SplitResizer() {
  const t = useT();
  const setSplitRatio = useUiStore((s) => s.setSplitRatio);
  // Track the row element so `move` re-reads getBoundingClientRect on each
  // event — if the window resizes mid-drag, the stale-captured rect would
  // otherwise compute an incorrect ratio.
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

      const move = (ev: PointerEvent) => {
        const rect = rowRef.current?.getBoundingClientRect();
        if (!rect) return;
        setSplitRatio(Math.max(0.25, Math.min(0.75, (ev.clientX - rect.left) / rect.width)));
      };
      const up = () => {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", up);
        listenersRef.current = null;
        rowRef.current = null;
      };
      listenersRef.current = { move, up };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
    },
    [setSplitRatio],
  );

  return (
    <div
      // A draggable vertical splitter IS role="separator" per ARIA; an <hr> is
      // a non-interactive horizontal thematic break — wrong for a resize handle.
      // eslint-disable-next-line jsx-a11y/prefer-tag-over-role
      role="separator"
      aria-orientation="vertical"
      aria-label={t("panel.split.resize")}
      onPointerDown={onPointerDown}
      className="group relative w-2 shrink-0 cursor-col-resize touch-none"
    >
      <div className="absolute inset-y-0 left-1/2 w-0.5 -translate-x-1/2 rounded-full bg-transparent transition-colors group-hover:bg-fg/[0.08] group-active:bg-fg/[0.12]" />
    </div>
  );
}
