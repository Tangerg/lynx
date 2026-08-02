import { Icon, ScrollArea } from "@/ui";
import { AgentDrawerToggle, AgentSurfaceHeader } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { useSidebarDrawer } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";

export function SidebarExpanded() {
  const t = useT();
  const items = useWorkIndexItems("expanded");
  const drawer = useSidebarDrawer();

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {/* The expanded drawer owns the top-left window corner, so the only visible
          sidebar toggle lives here and clears the native traffic lights. Once
          collapsed this whole drawer slides away and the content header takes
          over the same control. */}
      <AgentSurfaceHeader divider={false} className="agent-drawer-header">
        <AgentDrawerToggle
          collapsed={false}
          onToggle={drawer.toggle}
          expandLabel={t("sidebar.action.expand")}
          collapseLabel={t("sidebar.action.collapse")}
        />
        <span className="min-w-2 flex-1" />
        <Icon name="spark" size="sm" className="text-fg-faint" />
        <span className="sr-only">{t("common.appName")}</span>
      </AgentSurfaceHeader>

      <ScrollArea hideScrollbar className="px-1.5 pt-1 pb-3">
        <div className="flex flex-col gap-y-3">
          {items.map((item) => {
            const Body = item.component;
            return (
              <PluginBoundary
                key={item.id}
                plugin={`work-index:${item.id}`}
                label={`${item.id} work index item`}
              >
                <Body />
              </PluginBoundary>
            );
          })}
        </div>
      </ScrollArea>

      <div className="mt-auto">
        <Slot name="sidebar.footer" />
      </div>
    </div>
  );
}
