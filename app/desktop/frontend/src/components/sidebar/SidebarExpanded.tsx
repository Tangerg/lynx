// Expanded sidebar — slim chrome: collapse button + search box + scroll
// area of plugin-contributed sections + plugin-contributed footer.
//
// Brand bar (Lyra logo + name) and the user-card footer used to be
// hardcoded; both are now layout slots so a re-branded build or an account
// plugin can replace them.

import { Icon, Panel, ScrollArea } from "@/components/common";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { Slot } from "@/plugins/Slot";
import { useSidebarSections } from "@/plugins/sdk";

type Props = {
  onToggleRail: () => void;
};

export function SidebarExpanded({ onToggleRail }: Props) {
  const sections = useSidebarSections();

  return (
    <Panel className="sidebar">
      <div className="brand">
        <Slot name="sidebar.brand" />
        <button
          type="button"
          onClick={onToggleRail}
          title="Collapse to rail"
          className="ml-auto grid h-[26px] w-[26px] place-items-center rounded-md border-0 bg-transparent text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
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

      <div className="side-footer">
        <Slot name="sidebar.footer" />
      </div>
    </Panel>
  );
}
