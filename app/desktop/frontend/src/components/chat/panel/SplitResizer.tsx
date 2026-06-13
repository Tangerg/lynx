// Drag handle between the chat stream and a split workspace view. Self-written
// because Radix has no split-pane primitive (DESIGN §4 exemption: this is
// interactive chrome, not a decorative divider — idle shows nothing but a
// col-resize cursor; the accent guide appears only on hover/drag). On drag it
// computes the chat's fraction of the parent row's width and persists it
// (clamped 0.25–0.75) to uiStore so the split holds across sessions/launches.

import type { PointerEvent as ReactPointerEvent } from "react";
import { useUiStore } from "@/state/uiStore";

export function SplitResizer() {
  const setSplitRatio = useUiStore((s) => s.setSplitRatio);

  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const row = e.currentTarget.parentElement;
    if (!row) return;
    const rect = row.getBoundingClientRect();
    const move = (ev: PointerEvent) =>
      setSplitRatio(Math.max(0.25, Math.min(0.75, (ev.clientX - rect.left) / rect.width)));
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  return (
    <div
      // A draggable vertical splitter IS role="separator" per ARIA; an <hr> is
      // a non-interactive horizontal thematic break — wrong for a resize handle.
      // eslint-disable-next-line jsx-a11y/prefer-tag-over-role
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize chat / view split"
      onPointerDown={onPointerDown}
      className="group relative w-2 shrink-0 cursor-col-resize touch-none"
    >
      <div className="absolute inset-y-0 left-1/2 w-0.5 -translate-x-1/2 rounded-full bg-transparent transition-colors group-hover:bg-accent group-active:bg-accent" />
    </div>
  );
}
