import { AgentSurfaceHeader, AgentWorkIndexBody, AgentWorkIndexSection } from "@/ui/agent";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";

// No pinned "where the agent points" block above the index. The active session
// carries the highlight inside the project that owns it, and the content header
// already opens with that project's name — a third statement of it, one row
// above the second, was the loudest thing in the column.
export function SidebarExpanded() {
  const items = useWorkIndexItems("expanded");

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {/* Window controls live in AgentAppShell; this bar owns sidebar-local tools. */}
      <AgentSurfaceHeader divider={false} className="agent-drawer-header">
        <span className="min-w-2 flex-1" />
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
