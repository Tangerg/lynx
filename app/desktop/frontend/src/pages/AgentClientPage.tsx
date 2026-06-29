// AgentClientPage — the kernel layout.
//
// VS Code-inspired regions: left sidebar + center main area + a full-width
// status bar along the bottom + global overlay. Each region is a named Slot
// whose body comes from plugin contributions; this file owns no functional
// code, only the grid. (The status bar carries persistent run telemetry —
// DESIGN.md §8; the composer footer keeps input-adjacent session context.)

import { cn } from "@/lib/utils";
import { Slot } from "@/plugins/host/Slot";
import { useSidebarRail } from "@/state/useSidebarRail";

export function AgentClientPage() {
  // Collapses to a 56px rail by the user's preference OR while a split view is
  // open ("open right → collapse left"). One `.rail` modifier drives the grid
  // column; the sidebar Slot reads the same source (useSidebarRail) so its
  // content renders as a rail to match. The rail stays a usable, focusable
  // strip — never zero-width — so it isn't made `inert`.
  const railed = useSidebarRail();

  return (
    <>
      {/* Atmospheric depth layer — fixed behind everything, a single
          heavily-blurred radial gradient for ambient depth. Desaturated
          cool gray positioned in the top corner; never behind readable
          text or the composer. */}
      <div className="lyra-atmosphere" aria-hidden="true">
        <div className="lyra-atmosphere__orb lyra-atmosphere__orb--a" />
      </div>
      <div className={cn("app", railed && "rail")}>
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
      <Slot name="app.overlay" />
    </div>
    </>
  );
}
