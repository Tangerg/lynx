// Expanded sidebar — slim chrome: collapse button + search box + scroll
// area of plugin-contributed sections + plugin-contributed footer.

import { DragStrip, Icon, noDragClasses, Panel, ScrollArea } from "@/components/common";
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
    // titlebar padding + Wails drag region opt-outs). All visual styling
    // is Tailwind here. `pt-9` reserves the titlebar / traffic-lights
    // row so the search box doesn't ride up against the window edge.
    <Panel className="sidebar relative gap-2 px-3 pb-3 pt-9">
      {/* macOS drag-region strip — invisible 36px band over the
          titlebar row so the window can be dragged by its top edge.
          The collapse button below sits inside the same band but opts
          back out of the drag region via `noDragClasses`. */}
      <DragStrip height={36} />

      {/* Collapse button — pinned at the top-right corner of the
          sidebar, vertically aligned with the macOS traffic-light row.
          The previous version had a wide empty row underneath the
          drag strip (originally meant for a logo); that row is gone
          and the button moves up here. */}
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

      <Slot name="sidebar.search" />

      <ScrollArea hideScrollbar style={{ padding: "0 0 8px 0" }}>
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
      </ScrollArea>

      <div className="mt-auto px-1 pt-4">
        <Slot name="sidebar.footer" />
      </div>
    </Panel>
  );
}
