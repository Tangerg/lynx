// Collapsed sidebar — slim vertical strip. The kernel owns the expand
// button + the rail container; every other item is contributed via
// `host.sidebar.registerRailItem`.
//
// Order convention (loosely enforced by `order` numbers, see types.ts):
//   - 0..99    : top (new-session, search)
//   - 100..899 : middle (sessions stack)
//   - 900..999 : bottom (tools, settings, user)
//
// Items render strictly in sorted order — anything that wants to "stick
// to the bottom" should leave a flex spacer or set its own
// `margin-top: auto`.

import type { SidebarSession } from "./types";
import { DragStrip, Icon, IconButton, Panel } from "@/components/common";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useSidebarRailItems } from "@/plugins/sdk";

interface Props {
  // Forwarded purely so the rail-sessions plugin doesn't have to refetch
  // — sidebar callers already pass these for the expanded view. The
  // rail-sessions plugin reads these from stores/queries directly.
  sessions: SidebarSession[];
  activeSessionId: string;
  onSelect: (id: string) => void;
  onToggleRail: () => void;
}

export function SidebarRail({ onToggleRail }: Props) {
  const t = useT();
  const items = useSidebarRailItems();
  return (
    // `sidebar` / `rail` classes are kept as DOM hooks for layout.css
    // (macOS titlebar padding + Wails drag region). All visual styling is
    // Tailwind here.
    <Panel className="sidebar rail w-14 items-center gap-1 px-1.5 pb-3">
      {/* macOS drag-region strip — 36px above the expand button. See
          SidebarExpanded for the rationale (don't make rows draggable). */}
      <DragStrip height={36} />
      <IconButton variant="rail" title={t("sidebar.action.expand")} onClick={onToggleRail}>
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
