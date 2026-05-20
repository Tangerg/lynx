// AgentClientPage — the shell.
//
// VS Code-inspired layout: left sidebar + center editor area + bottom
// status bar. The right "inspector" pane is gone; inspector views are
// promoted to main-area tabs when the user opens them.

import { Slot } from "@/plugins/Slot";
import { useUIStore } from "@/state/uiStore";

export function AgentClientPage() {
  const sidebarRail = useUIStore((s) => s.sidebarRail);

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
