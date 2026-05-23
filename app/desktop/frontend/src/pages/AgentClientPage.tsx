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
        <Slot name="app.sidebar" />
        <Slot name="app.main" />
      </div>
      <div className="app-statusbar">
        <Slot name="app.statusbar" />
      </div>
      <Slot name="app.overlay" />
    </div>
  );
}
