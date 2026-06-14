// AgentClientPage — the kernel layout.
//
// VS Code-inspired regions: left sidebar + center main area + a full-width
// status bar along the bottom + global overlay. Each region is a named Slot
// whose body comes from plugin contributions; this file owns no functional
// code, only the grid. (The status bar carries persistent run telemetry —
// DESIGN.md §8; the composer footer keeps input-adjacent session context.)

import { cn } from "@/lib/utils";
import { Slot } from "@/plugins/host/Slot";
import { useSessionStore } from "@/state/sessionStore";
import { useUiStore } from "@/state/uiStore";

export function AgentClientPage() {
  const sidebarRail = useUiStore((s) => s.sidebarRail);
  // While a split (beside) view is open, hand the full window width to
  // chat + split: the `split` modifier collapses the sidebar grid column to
  // 0 (mirroring `.app.rail`). The sidebar Slot stays MOUNTED — only the
  // track collapses — so closing the split restores it without remount.
  const splitOpen = useSessionStore((s) => s.splitViewId !== null);

  return (
    <div className={cn("app", sidebarRail && "rail", splitOpen && "split")}>
      <div className="app-main">
        {/* Landmark roles for SR users to skip between regions, while
            `display: contents` keeps the wrapper transparent to the
            outer grid — `.app-main` is `grid-template-columns` and
            expects each Slot's Panel to be a direct child so it can
            stretch into the cell. Wrapping with a layout-active element
            would steal the cell's height and Panel's `flex flex-col +
            min-h-0` would collapse to 0. */}
        <aside aria-label="Sidebar" className="contents">
          <Slot name="app.sidebar" />
        </aside>
        <main aria-label="Main" className="contents">
          <Slot name="app.main" />
        </main>
      </div>
      {/* Full-width chrome row below the cards. `contents` keeps the footer
          transparent to the grid; the wrapper Slot renders the `.statusbar`
          div that actually occupies the bottom track. */}
      <footer aria-label="Status bar" className="contents">
        <Slot name="app.statusbar" wrapper className="statusbar" />
      </footer>
      <Slot name="app.overlay" />
    </div>
  );
}
