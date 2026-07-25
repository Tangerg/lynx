import { AgentAppShell } from "@/ui/agent";
import { Slot } from "@/plugins/host/Slot";
import { useActiveWorkspaceViewId } from "@/plugins/builtin/workspace/public/navigation";
import { useSidebarRail, useSidebarWidth } from "@/plugins/builtin/workspace/public/sidebarRail";

export function AgentClientPage() {
  const railed = useSidebarRail();
  const { width, setWidth } = useSidebarWidth();
  const activeViewId = useActiveWorkspaceViewId();
  // Settings takes the whole window: no work index to sit beside.
  const singleMode = activeViewId === "settings";

  return (
    <AgentAppShell
      sidebarOpen={!railed}
      sidebarWidth={width}
      onResize={setWidth}
      sidebar={singleMode ? undefined : <Slot name="app.sidebar" />}
      main={<Slot name="app.main" />}
      overlay={<Slot name="app.overlay" />}
    />
  );
}
