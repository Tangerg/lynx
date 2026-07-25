import { ScrollArea } from "@/ui";
import { dragClasses, noDragClasses } from "@/lib/windowDrag";
import { AgentSurfaceHeader } from "@/ui/agent";
import { cn } from "@/lib/utils";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";

export function SidebarExpanded() {
  const items = useWorkIndexItems("expanded");

  return (
    <div className={cn("flex min-h-0 min-w-0 flex-1 flex-col", dragClasses)}>
      {/* Same chrome-bar height as the content card's header so the two line up
          across the seam. Empty and undivided by design: the OS draws the window
          controls in this corner (TitleBarHiddenInset). */}
      <AgentSurfaceHeader divider={false} aria-hidden />

      <ScrollArea hideScrollbar className="px-2 pt-1 pb-4">
        <div className={cn("flex flex-col gap-y-5", noDragClasses)}>
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
    </div>
  );
}
