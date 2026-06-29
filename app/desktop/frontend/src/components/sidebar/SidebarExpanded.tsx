// Expanded sidebar — slim chrome: collapse button + search box + scroll
// area of plugin-contributed sections + plugin-contributed footer.

import { Icon, dragClasses, noDragClasses, Panel, ScrollArea } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { SIDEBAR_SECTION, useExtensionPoint } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";

interface Props {
  onToggleRail: () => void;
}

export function SidebarExpanded({ onToggleRail }: Props) {
  const t = useT();
  const sections = useExtensionPoint(SIDEBAR_SECTION);

  return (
    // `sidebar` class is kept as a DOM hook for layout.css (macOS
    // titlebar padding). The entire panel is a drag region; interactive
    // children opt out via `noDragClasses`.
    <Panel className={cn("sidebar relative gap-1.5 px-2 pb-3 pt-3", dragClasses)}>
      {/* Collapse button — pinned at the top-right corner of the
          sidebar, vertically aligned with the macOS traffic-light row. */}
      <button
        type="button"
        onClick={onToggleRail}
        title={t("sidebar.action.collapse")}
        aria-label={t("sidebar.action.collapse")}
        className={cn(
          "absolute top-2 right-3 z-10 grid h-6.5 w-6.5 place-items-center",
          "rounded-md border-0 bg-transparent text-fg-muted transition-colors",
          "hover:bg-surface-2 hover:text-fg",
          noDragClasses,
        )}
      >
        <Icon name="panel-l" size={14} />
      </button>

      <div className={noDragClasses}>
        <Slot name="sidebar.search" />
      </div>

      <ScrollArea hideScrollbar style={{ padding: "0 0 8px 0" }}>
        <div className={noDragClasses}>
          {sections.map((section) => {
            const Body = section.component;
            return (
              <PluginBoundary
                key={section.id}
                plugin={`sidebar:${section.id}`}
                label={`${section.id} section`}
              >
                <Body />
              </PluginBoundary>
            );
          })}
        </div>
      </ScrollArea>

      <div className="mt-auto px-1 pt-4">
        <Slot name="sidebar.footer" />
      </div>
    </Panel>
  );
}
