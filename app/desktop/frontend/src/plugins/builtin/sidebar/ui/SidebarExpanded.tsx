// Expanded sidebar — slim chrome: collapse button, scroll area of
// plugin-contributed sections, and plugin-contributed footer.

import { Icon, dragClasses, noDragClasses, Panel, ScrollArea } from "@/components/common";
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
    <Panel className={cn("sidebar relative gap-2 px-2.5 pb-4 pt-3", dragClasses)}>
      {/* Collapse button — pinned at the top-right corner of the
          sidebar, vertically aligned with the macOS traffic-light row. */}
      <button
        type="button"
        onClick={onToggleRail}
        data-chrome-focus=""
        title={t("sidebar.action.collapse")}
        aria-label={t("sidebar.action.collapse")}
        className={cn(
          "absolute top-2 right-3 z-10 grid h-6.5 w-6.5 place-items-center",
          "rounded-md border-0 bg-transparent text-fg-muted transition-colors",
          "hover:bg-fg/[0.05] hover:text-fg focus-visible:bg-fg/[0.06] focus-visible:text-fg focus-visible:outline-none",
          noDragClasses,
        )}
      >
        <Icon name="panel-l" size={14} />
      </button>

      <ScrollArea hideScrollbar style={{ padding: "0 0 8px 0" }}>
        <div className={noDragClasses}>
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

      <div className="mt-auto px-0.5 pt-4">
        <Slot name="sidebar.footer" />
      </div>
    </Panel>
  );
}
