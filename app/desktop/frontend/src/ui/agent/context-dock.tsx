import type { CSSProperties, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { Tooltip } from "@/ui/atoms/tooltip";
import { TabsPrimitive } from "@/ui/primitives";

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
 *  family of control instead of two unrelated affordances. The tab primitive
 *  owns roving focus and arrow-key navigation; styling buttons to resemble tabs
 *  without those semantics made the dock keyboard-hostile. */
export function AgentDockTabs({
  tabs,
  ariaLabel,
  compactInactive = false,
}: {
  tabs: AgentDockTab[];
  ariaLabel: string;
  /** Keep the active destination readable while inactive pinned destinations
   *  collapse to icon affordances. The dock can then carry several useful
   *  destinations without pushing its container actions off-screen. */
  compactInactive?: boolean;
}) {
  if (tabs.length === 0) return null;
  const activeId = tabs.find((tab) => tab.active)?.id ?? tabs[0]?.id;
  return (
    <TabsPrimitive.Root
      value={activeId}
      onValueChange={(id) => tabs.find((tab) => tab.id === id)?.onSelect?.()}
      className="agent-dock-tabs"
    >
      <TabsPrimitive.List aria-label={ariaLabel} className="contents" activateOnFocus>
        {tabs.map((tab) => {
          const compact = compactInactive && !tab.active;
          const control = (
            <TabsPrimitive.Tab
              key={tab.id}
              value={tab.id}
              data-chrome-focus=""
              className={cn(
                "inline-flex h-7 min-w-0 shrink-0 items-center justify-center rounded-md border-0 bg-transparent",
                compact ? "w-7 px-0" : "max-w-40 gap-1.5 px-1.5",
                "text-ui-sm font-normal text-fg-muted transition-[background-color,color,width] duration-[var(--dur-fast)] ease-out",
                "hover:bg-hover hover:text-fg focus-visible:outline-none",
                "data-[active]:bg-selected data-[active]:text-fg",
              )}
            >
              {tab.icon && (
                <Icon name={tab.icon} size={14} strokeWidth={1.8} className="shrink-0 opacity-70" />
              )}
              <span className={cn("truncate", compact && "sr-only")}>{tab.title}</span>
            </TabsPrimitive.Tab>
          );
          return compactInactive ? (
            <Tooltip key={tab.id} label={tab.title} side="bottom">
              {control}
            </Tooltip>
          ) : (
            control
          );
        })}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
