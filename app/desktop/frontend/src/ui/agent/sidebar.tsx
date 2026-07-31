import type { ReactNode } from "react";
import { useCallback, useEffect, useRef } from "react";
import { clampSidebarWidth, maxSidebarWidth, SIDEBAR_MIN_WIDTH_PX } from "@/lib/shellGeometry";

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
 * Resize separator for the drawer↔card seam. It draws nothing: the visible
 * divider is the card's inset ring, which this rail only intensifies on hover or
 * keyboard focus (a `:has()` rule in globals.css). Drawing a line here would put
 * a second one beside it.
 *
 * While dragging, the width goes straight onto the shell element as a custom
 * property — React state per pointer-move would drop frames on a 60Hz trackpad.
 * `onCommit` persists the settled value once, on release. Keyboard resize uses
 * the same direct-write + single-commit path.
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
  const railRef = useRef<HTMLDivElement>(null);
  // Read in the pointerup handler, which is registered once — a ref keeps the
  // listener from having to be torn down and rebuilt on every parent render.
  const commitRef = useRef(onCommit);
  useEffect(() => {
    commitRef.current = onCommit;
  }, [onCommit]);

  useEffect(() => {
    const rail = railRef.current;
    const shell = rail?.closest<HTMLElement>(".agent-shell");
    if (!rail || !shell) return;
    const syncRange = () => {
      const max = maxSidebarWidth(shell.clientWidth);
      rail.setAttribute("aria-valuemax", String(max));
      // Derive the settled value from the persisted preference plus current
      // geometry, exactly as AgentAppShell does for the CSS property. Reading
      // the property here would make callback ordering between two
      // ResizeObservers observable: the rail could announce the previous
      // viewport's clamp even after the shell had restored the layout.
      rail.setAttribute("aria-valuenow", String(clampSidebarWidth(width, shell.clientWidth)));
    };
    syncRange();
    const observer = new ResizeObserver(syncRange);
    observer.observe(shell);
    return () => observer.disconnect();
  }, [width]);

  const handlePointerDown = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    const shell = railRef.current?.closest<HTMLElement>(".agent-shell");
    if (!shell || event.button !== 0) return;
    event.preventDefault();

    const startX = event.clientX;
    const startWidth = readSidebarWidth(shell);
    let width = startWidth;
    const move = (moveEvent: PointerEvent) => {
      width = clampSidebarWidth(startWidth + moveEvent.clientX - startX, shell.clientWidth);
      shell.style.setProperty("--sidebar-width", `${width}px`);
      railRef.current?.setAttribute("aria-valuenow", String(width));
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

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const rail = railRef.current;
    const shell = rail?.closest<HTMLElement>(".agent-shell");
    if (!rail || !shell) return;
    event.preventDefault();

    const max = maxSidebarWidth(shell.clientWidth);
    const current = readSidebarWidth(shell);
    const step = event.shiftKey ? 24 : 8;
    const next =
      event.key === "Home"
        ? SIDEBAR_MIN_WIDTH_PX
        : event.key === "End"
          ? max
          : clampSidebarWidth(
              current + (event.key === "ArrowLeft" ? -step : step),
              shell.clientWidth,
            );

    shell.setAttribute("data-resizing", "");
    shell.style.setProperty("--sidebar-width", `${next}px`);
    rail.setAttribute("aria-valuemax", String(max));
    rail.setAttribute("aria-valuenow", String(next));
    shell.removeAttribute("data-resizing");
    commitRef.current(next);
  }, []);

  return (
    <div
      ref={railRef}
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemin={SIDEBAR_MIN_WIDTH_PX}
      aria-valuenow={Math.round(width)}
      className="agent-seam-rail"
      onPointerDown={handlePointerDown}
      onKeyDown={handleKeyDown}
    />
  );
}

function readSidebarWidth(shell: HTMLElement): number {
  const value = Number.parseFloat(getComputedStyle(shell).getPropertyValue("--sidebar-width"));
  return clampSidebarWidth(Number.isFinite(value) ? value : 0, shell.clientWidth);
}
