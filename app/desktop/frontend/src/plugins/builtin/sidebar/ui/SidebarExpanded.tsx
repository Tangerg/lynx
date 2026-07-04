import { AgentPane } from "@/ui/agent";
import { dragClasses, noDragClasses, ScrollArea } from "@/ui";
import { cn } from "@/lib/utils";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";

export function SidebarExpanded() {
  const items = useWorkIndexItems("expanded");

  return (
    <AgentPane tone="sidebar" className={cn("sidebar", dragClasses)}>
      {/* Draggable top strip that clears the native macOS traffic-light inset
          (TitleBarHiddenInset) — the OS draws the only window controls here. */}
      <div className="h-[38px] shrink-0" aria-hidden />

      <ScrollArea hideScrollbar style={{ padding: "2px 10px 14px" }}>
        <div className={cn("flex flex-col gap-y-3", noDragClasses)}>
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

      <div className={cn("mt-auto", noDragClasses)}>
        <Slot name="sidebar.footer" />
      </div>
    </AgentPane>
  );
}
