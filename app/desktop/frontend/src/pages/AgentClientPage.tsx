// AgentClientPage — the shell.
//
// The shell owns just two things:
//   1. Outer layout classes (rail / inspector-open) — they drive CSS grid
//      breakpoints in App.css and live too close to the document root to
//      meaningfully delegate.
//   2. The `<Slot>` mount points.
//
// Adding a new region = registering a new layout slot, not editing this file.

import { Slot } from "@/plugins/Slot";
import { useUIStore } from "@/state/uiStore";

export function AgentClientPage() {
  const sidebarRail = useUIStore((s) => s.sidebarRail);
  const inspectorOpen = useUIStore((s) => s.inspectorOpen);

  return (
    <div className={`app ${sidebarRail ? "rail" : ""} ${inspectorOpen ? "insp-open" : ""}`}>
      <div className="app-main">
        <Slot name="app.sidebar" />
        <Slot name="app.main" />
        <Slot name="app.inspector" />
      </div>
      <Slot name="app.overlay" />
    </div>
  );
}
