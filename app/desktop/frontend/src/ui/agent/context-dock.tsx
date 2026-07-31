import type { CSSProperties, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";

export interface AgentDockTab {
  id: string;
  title: ReactNode;
  icon?: IconName;
  active?: boolean;
  onSelect?: () => void;
}

/**
 * The right-hand workspace column.
 *
 * Deliberately a real column and not a floating overlay: it hosts full views
 * (diff, file tree, terminal), and the whole point of the dock is reading those
 * BESIDE the conversation — an overlay would cover the thing being compared
 * against. It lives inside the content card, so its left edge is an internal pane
 * split and takes the chrome hairline rather than a background step.
 */
export function AgentContextDock({
  className,
  style,
  children,
}: {
  className?: string;
  /** Carries the resizable width, which lives in a custom property so a drag
   *  doesn't re-render the card. */
  style?: CSSProperties;
  children: ReactNode;
}) {
  return (
    <aside className={cn("agent-context-dock agent-pane-split", className)} style={style}>
      {children}
    </aside>
  );
}

/** Dock tabs wear the chrome-chip skin, so a tab and a header toggle read as one
 *  family of control instead of two unrelated affordances. */
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
          className={cn(
            "inline-flex h-7 min-w-0 max-w-40 shrink-0 items-center gap-1.5 rounded-md border-0 bg-transparent px-1.5",
            "text-ui-sm font-normal text-fg-muted transition-colors duration-[var(--dur-fast)] ease-out",
            "hover:bg-hover hover:text-fg",
            "data-[active]:bg-selected data-[active]:text-fg",
          )}
        >
          {tab.icon && (
            <Icon name={tab.icon} size={14} strokeWidth={1.8} className="shrink-0 opacity-70" />
          )}
          <span className="truncate">{tab.title}</span>
        </button>
      ))}
    </div>
  );
}
