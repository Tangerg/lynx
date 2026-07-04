// Expanded sidebar — slim chrome: collapse button, scroll area of
// plugin-contributed sections, and plugin-contributed footer.

import { AgentIconButton, AgentWindowControls } from "@/ui/agent";
import { dragClasses, noDragClasses, Panel, ScrollArea } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { Slot } from "@/plugins/host/Slot";

interface Props {
  onToggleRail: () => void;
}

export function SidebarExpanded({ onToggleRail }: Props) {
  const t = useT();
  const items = useWorkIndexItems("expanded");

  return (
    // `sidebar` class is kept as a DOM hook for layout.css (macOS
    // titlebar padding). The entire panel is a drag region; interactive
    // children opt out via `noDragClasses`.
    <Panel className={cn("sidebar relative", dragClasses)}>
      <AgentWindowControls />
      {/* Collapse button — pinned at the top-right corner of the
          sidebar, vertically aligned with the macOS traffic-light row. */}
      <AgentIconButton
        icon="panel-l"
        size="sm"
        onClick={onToggleRail}
        data-chrome-focus=""
        title={t("sidebar.action.collapse")}
        aria-label={t("sidebar.action.collapse")}
        className={cn("absolute right-3 top-3 z-10 opacity-55 hover:opacity-100", noDragClasses)}
      />

      <ScrollArea hideScrollbar style={{ padding: "6px 10px 14px" }}>
        <div className={cn("flex flex-col gap-px", noDragClasses)}>
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
    </Panel>
  );
}
