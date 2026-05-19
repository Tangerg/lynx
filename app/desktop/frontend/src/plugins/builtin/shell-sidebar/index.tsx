// Built-in plugin: fills the "app.sidebar" layout slot with the existing
// SidebarPanel. The slot component takes no props — every piece of state it
// needs (active session, rail toggle) is read directly from the relevant
// Zustand store / TanStack-Query hook. Theme + accent are owned by the
// sidebar-footer plugin (where the user card lives).

import { SidebarPanel } from "@/components/sidebar/SidebarPanel";
import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function ShellSidebar() {
  const sidebarRail = useUIStore((s) => s.sidebarRail);
  const activeSession = useUIStore((s) => s.activeSessionId);
  const selectTab = useUIStore((s) => s.selectTab);
  const toggleSidebar = useUIStore((s) => s.toggleSidebar);

  // Only the rail view still needs the sessions list (the expanded view
  // gets it via the plugin-contributed sidebar sections).
  const { data: sessions = [] } = useSessions();

  return (
    <SidebarPanel
      sessions={sessions}
      activeSessionId={activeSession}
      onSelect={selectTab}
      rail={sidebarRail}
      onToggleRail={toggleSidebar}
    />
  );
}

export default definePlugin({
  name: "lyra.builtin.shell-sidebar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.sidebar", {
      id: "sidebar",
      order: 0,
      component: ShellSidebar,
    });
  },
});
