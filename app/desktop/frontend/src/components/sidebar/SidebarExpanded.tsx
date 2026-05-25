// Expanded sidebar — slim chrome: collapse button + search box + scroll
// area of plugin-contributed sections + plugin-contributed footer.

import { Icon, Panel, ScrollArea } from "@/components/common";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { Slot } from "@/plugins/Slot";
import { useSidebarSections } from "@/plugins/sdk";

type Props = {
  onToggleRail: () => void;
};

export function SidebarExpanded({ onToggleRail }: Props) {
  const t = useT();
  const sections = useSidebarSections();

  return (
    // `sidebar` class is kept as a DOM hook for layout.css (macOS
    // titlebar padding + Wails drag region opt-outs). All visual styling
    // is Tailwind here.
    <Panel className="sidebar gap-2 px-3 pb-3">
      <div className="flex items-center pt-1 pb-4">
        <button
          type="button"
          onClick={onToggleRail}
          title={t("sidebar.action.collapse")}
          aria-label={t("sidebar.action.collapse")}
          className="ml-auto grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
        >
          <Icon name="panel-l" size={14} />
        </button>
      </div>

      <Slot name="sidebar.search" />

      <ScrollArea style={{ padding: "0 0 8px 0" }}>
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
