// Collapsed sidebar — slim vertical strip. The kernel owns the brand
// mark + the expand button + the rail container; everything else is
// contributed via `host.sidebar.registerRailItem`.
//
// Order convention (loosely enforced by `order` numbers, see types.ts):
//   - 0..99  : top (brand stack, new-session, search)
//   - 100..899 : middle (sessions stack)
//   - 900..999 : bottom (tools, settings, user)
//
// Items render strictly in sorted order — anything that wants to "stick
// to the bottom" should leave a flex spacer or set its own
// `margin-top: auto`.

import { Icon, IconButton, Panel } from "@/components/common";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { Slot } from "@/plugins/Slot";
import { useSidebarRailItems } from "@/plugins/sdk";
import type { SidebarSession } from "./types";

type Props = {
  // Forwarded purely so the rail-sessions plugin doesn't have to refetch
  // — sidebar callers already pass these for the expanded view. The
  // rail-sessions plugin reads these from stores/queries directly.
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
