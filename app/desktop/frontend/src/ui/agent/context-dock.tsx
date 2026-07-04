import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Icon, type IconName } from "@/ui/icons";

export interface AgentDockTab {
  id: string;
  title: ReactNode;
  icon?: IconName;
  active?: boolean;
  onSelect?: () => void;
}

export function AgentContextDock({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return <aside className={cn("agent-context-dock", className)}>{children}</aside>;
}

export function AgentDockTabs({ tabs }: { tabs: AgentDockTab[] }) {
  if (tabs.length === 0) return null;
  return (
    <div className="agent-dock-tabs">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          data-active={tab.active ? "" : undefined}
          data-chrome-focus=""
          onClick={tab.onSelect}
          className="agent-dock-tab transition-[background-color,color,box-shadow] duration-[120ms] ease-out"
        >
          {tab.icon && <Icon name={tab.icon} size={14} strokeWidth={1.8} />}
          <span className="truncate">{tab.title}</span>
        </button>
      ))}
    </div>
  );
}
