import type { ReactNode } from "react";
import { useCallback, useEffect, useRef } from "react";
import { clampSidebarWidth } from "@/lib/shellGeometry";

/**
 * The work-index drawer: an in-flow spacer that reserves the width, plus a
 * fixed-position panel that slides. Both read `--sidebar-width` from the shell,
 * so a resize is one custom-property write and a collapse is one attribute flip
 * — see the `.agent-drawer*` rules in globals.css for why this isn't a grid.
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
 * Drag handle for the drawer↔card seam. It draws nothing: the visible divider is
 * the card's inset ring, which this rail only intensifies on hover (a `:has()`
 * rule in globals.css). Drawing a line here would put a second one beside it.
 *
 * While dragging, the width goes straight onto the shell element as a custom
 * property — React state per pointer-move would drop frames on a 60Hz trackpad.
 * `onCommit` persists the settled value once, on release.
 */
export function AgentSeamRail({ onCommit }: { onCommit: (width: number) => void }) {
  const railRef = useRef<HTMLButtonElement>(null);
  // Read in the pointerup handler, which is registered once — a ref keeps the
  // listener from having to be torn down and rebuilt on every parent render.
  const commitRef = useRef(onCommit);
  useEffect(() => {
    commitRef.current = onCommit;
  }, [onCommit]);

  const handlePointerDown = useCallback((event: React.PointerEvent<HTMLButtonElement>) => {
    const shell = railRef.current?.closest<HTMLElement>(".agent-shell");
    if (!shell || event.button !== 0) return;
    event.preventDefault();

    let width = clampSidebarWidth(event.clientX, shell.clientWidth);
    const move = (moveEvent: PointerEvent) => {
      width = clampSidebarWidth(moveEvent.clientX, shell.clientWidth);
      shell.style.setProperty("--sidebar-width", `${width}px`);
    };
    const end = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", end);
      window.removeEventListener("pointercancel", end);
      // The drag suppressed the slide transition; restore it for the next
      // collapse so the drawer animates again.
      shell.removeAttribute("data-resizing");
      commitRef.current(width);
    };
    // Suppress the slide transition for the duration of the drag, or every
    // pointer-move would start a 300ms animation toward the new width and the
    // handle would lag the cursor.
    shell.setAttribute("data-resizing", "");
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", end);
    window.addEventListener("pointercancel", end);
  }, []);

  return (
    <button
      ref={railRef}
      type="button"
      tabIndex={-1}
      aria-hidden
      className="agent-seam-rail"
      onPointerDown={handlePointerDown}
    />
  );
}
