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
    // `sidebar` class is kept as a DOM hook for layout.css (macOS
    // titlebar padding + Wails drag region opt-outs). All visual styling
    // is Tailwind here.
    <Panel className="sidebar gap-2 px-3 pb-3">
      {/* Brand row — accent-coloured logo + "Lyra" name + collapse btn.
          The `brand` class is kept as a DOM hook so the layout.css
          .sidebar .brand no-drag rule still applies. */}
      <div className="brand relative flex items-center gap-2.5 px-2 pt-1 pb-4">
        <Slot name="sidebar.brand" />
        <button
          type="button"
          onClick={onToggleRail}
          title="Collapse to rail" aria-label="Collapse to rail"
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
