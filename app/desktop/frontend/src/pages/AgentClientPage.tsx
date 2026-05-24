// AgentClientPage — the kernel layout.
//
// VS Code-inspired regions: left sidebar + center main area + bottom
// status bar + global overlay. Each region is a named Slot whose body
// comes from plugin contributions; this file owns no functional code,
// only the grid.

import { Slot } from "@/plugins/Slot";
import { useLayoutStore } from "@/state/layoutStore";

export function AgentClientPage() {
  const sidebarRail = useLayoutStore((s) => s.sidebarRail);

  return (
    <div className={`app ${sidebarRail ? "rail" : ""}`}>
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
      {/* `role="status"` + `aria-live="polite"` means SR users hear
          telemetry updates (run state, tokens, branch) when they change,
          without interruption. */}
      <div className="app-statusbar" role="status" aria-live="polite">
        <Slot name="app.statusbar" />
      </div>
      <Slot name="app.overlay" />
    </div>
  );
}
