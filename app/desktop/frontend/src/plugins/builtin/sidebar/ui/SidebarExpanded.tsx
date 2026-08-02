import { IconButton } from "@/ui";
import { AgentSurfaceHeader, AgentWorkIndexBody, AgentWorkIndexSection } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";
import { usePaletteStore } from "@/plugins/builtin/command/paletteStore";

export function SidebarExpanded() {
  const t = useT();
  const items = useWorkIndexItems("expanded");

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {/* Window controls live in AgentAppShell; this bar owns sidebar-local tools. */}
      <AgentSurfaceHeader divider={false} className="agent-drawer-header">
        <span className="min-w-2 flex-1" />
        <IconButton
          icon="search"
          size="sm"
          title={t("command.openPalette")}
          onClick={() => usePaletteStore.getState().setOpen(true)}
        />
      </AgentSurfaceHeader>

      <AgentWorkIndexBody>
        {items.map((item) => {
          const Body = item.component;
          return (
            <AgentWorkIndexSection key={item.id}>
              <PluginBoundary plugin={`work-index:${item.id}`} label={`${item.id} work index item`}>
                <Body />
              </PluginBoundary>
            </AgentWorkIndexSection>
          );
        })}
      </AgentWorkIndexBody>

      <div className="mt-auto shrink-0">
        <Slot name="sidebar.footer" />
      </div>
    </div>
  );
}
