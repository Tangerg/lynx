import { AgentAppShell } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { useActiveWorkspaceViewId } from "@/plugins/builtin/workspace/public/navigation";
import {
  useSidebarDrawer,
  useSidebarWidth,
} from "@/plugins/builtin/workspace/public/sidebarDrawer";

export function AgentClientPage() {
  const t = useT();
  const drawer = useSidebarDrawer();
  const { width, setWidth } = useSidebarWidth();
  const activeViewId = useActiveWorkspaceViewId();
  // Settings takes the whole window: no work index to sit beside.
  const singleMode = activeViewId === "settings";

  return (
    <AgentAppShell
      sidebarLabel={t("shell.region.workIndex")}
      sidebarResizeLabel={t("sidebar.action.resize")}
      sidebarOpen={!drawer.collapsed}
      sidebarWidth={width}
      onResize={setWidth}
      onSidebarToggle={drawer.toggle}
      sidebarExpandLabel={t("sidebar.action.expand")}
      sidebarCollapseLabel={t("sidebar.action.collapse")}
      sidebar={singleMode ? undefined : <Slot name="app.sidebar" />}
      main={<Slot name="app.main" />}
      overlay={<Slot name="app.overlay" />}
    />
  );
}
