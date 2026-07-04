import { AgentPane } from "@/ui/agent";
import { dragClasses, Icon, IconButton } from "@/ui";
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
    <AgentPane
      tone="rail"
      className={cn("sidebar rail w-14 items-center gap-1 px-1.5 pb-3", dragClasses)}
    >
      {/* Draggable strip clearing the native macOS traffic-light inset — the
          rail's own controls start below it. */}
      <div className="h-[38px] shrink-0" aria-hidden />
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
      {/* Expand back to the full sidebar — pinned at the very bottom (Codex
          rail: the collapse/expand toggle lives with the bottom utilities). */}
      <IconButton variant="rail" title={t("sidebar.action.expand")} onClick={onToggleRail}>
        <Icon name="panel-l" size={16} />
      </IconButton>
    </AgentPane>
  );
}
