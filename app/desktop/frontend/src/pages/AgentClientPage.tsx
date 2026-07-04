import { AgentAppShell } from "@/ui/agent";
import { Slot } from "@/plugins/host/Slot";
import { useActiveWorkspaceViewId } from "@/plugins/builtin/workspace/public/navigation";
import { useSidebarRail } from "@/plugins/builtin/workspace/public/sidebarRail";

export function AgentClientPage() {
  const railed = useSidebarRail();
  const activeViewId = useActiveWorkspaceViewId();
  const singleMode = activeViewId === "settings";

  return (
    <AgentAppShell
      rail={railed}
      mode={singleMode ? "single" : "work"}
      sidebar={singleMode ? undefined : <Slot name="app.sidebar" />}
      main={<Slot name="app.main" />}
      overlay={<Slot name="app.overlay" />}
    />
  );
}
