// Collapsed sidebar — slim vertical strip. Phase 12 turned its contents
// into plugin contributions: the shell owns the brand mark + the expand
// button + the rail container; everything else is registered via
// `host.sidebar.registerRailItem`.
//
// Order convention (loosely enforced by `order` numbers, see types.ts):
//   - 0..99  : top (brand stack, new-session, search)
//   - 100..899 : middle (sessions stack)
//   - 900..999 : bottom (tools, settings, user)
//
// The shell renders items strictly in sorted order — items that want to
// "stick to the bottom" should leave a flex spacer or set their own
// `margin-top: auto`.

import { Icon, IconButton, Panel } from "@/components/common";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { Slot } from "@/plugins/Slot";
import { useSidebarRailItems } from "@/plugins/sdk";
import type { SidebarSession } from "./types";

type Props = {
  // Forwarded purely so the rail-sessions plugin doesn't have to refetch
  // — Sidebar* shells are already passing this for the expanded view.
  // The rail-sessions plugin reads these from stores/queries directly.
  sessions: SidebarSession[];
  activeSessionId: string;
  onSelect: (id: string) => void;
  onToggleRail: () => void;
};

export function SidebarRail({ onToggleRail }: Props) {
  const items = useSidebarRailItems();
  return (
    <Panel className="sidebar rail">
      <div className="rail-brand">
        <Slot name="sidebar.rail.brand" />
      </div>
      <IconButton variant="rail" title="Expand sidebar" onClick={onToggleRail}>
        <Icon name="panel-l" size={16} />
      </IconButton>
      {items.map((item) => {
        const Body = item.component;
        return (
          <PluginBoundary
            key={item.id}
            plugin={`sidebar-rail:${item.id}`}
            label={`${item.id} rail item`}
          >
            <Body />
          </PluginBoundary>
        );
      })}
    </Panel>
  );
}
