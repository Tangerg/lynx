// Collapsed sidebar — slim vertical strip. The kernel owns the expand
// button + the rail container; every other item is contributed by Work Index
// plugins.
//
// Order convention (loosely enforced by `order` numbers, see types.ts):
//   - 0..99    : top (new-session)
//   - 100..899 : middle (sessions stack)
//   - 900..999 : bottom (context, settings, user)
//
// Items render strictly in sorted order — anything that wants to "stick
// to the bottom" should leave a flex spacer or set its own
// `margin-top: auto`.

import { dragClasses, Icon, IconButton, Panel } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useWorkIndexItems } from "@/plugins/builtin/navigation/public/workIndex";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";

interface Props {
  onToggleRail: () => void;
}

export function SidebarRail({ onToggleRail }: Props) {
  const t = useT();
  const items = useWorkIndexItems("rail");
  return (
    // `sidebar` / `rail` classes are kept as DOM hooks for layout.css
    // (macOS titlebar padding). All visual styling is Tailwind here.
    <Panel className={cn("sidebar rail w-14 items-center gap-1 px-1.5 pb-3", dragClasses)}>
      <IconButton variant="rail" title={t("sidebar.action.expand")} onClick={onToggleRail}>
        <Icon name="panel-l" size={16} />
      </IconButton>
      {items.map((item) => {
        const Body = item.component;
        return (
          <PluginBoundary
            key={item.id}
            plugin={`work-index-rail:${item.id}`}
            label={`${item.id} work index rail item`}
          >
            <Body />
          </PluginBoundary>
        );
      })}
    </Panel>
  );
}
