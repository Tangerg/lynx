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
      <IconButton
        variant="rail"
        title={t("sidebar.action.expand")}
        onClick={onToggleRail}
        className="mt-2"
      >
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
    </AgentPane>
  );
}
