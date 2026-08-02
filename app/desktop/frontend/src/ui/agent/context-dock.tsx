import { useRef, type CSSProperties, type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { IconButton } from "@/ui/atoms/icon-button";
import { TabsPrimitive } from "@/ui/primitives";

export interface AgentDockTab {
  id: string;
  title: ReactNode;
  icon?: IconName;
  /** A few characters of live count, set after the title. */
  badge?: ReactNode;
  active?: boolean;
  onSelect?: () => void;
  onClose?: () => void;
  closeLabel?: string;
}

/**
 * The right-hand workspace column.
 *
 * Deliberately a real column and not a floating overlay: it hosts full views
 * (diff, file tree, terminal), and the whole point of the dock is reading those
 * BESIDE the conversation — an overlay would cover the thing being compared
 * against. It lives inside the content card, so its left edge is an internal pane
 * split and takes the active visual style's boundary rather than inventing its
 * own background step.
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

/** Dock tabs share one structural pattern while the visual style chooses the
 *  active treatment (quiet chip, underline, or elevation). The tab primitive
 *  owns roving focus and arrow-key navigation; styling buttons to resemble tabs
 *  without those semantics made the dock keyboard-hostile. */
export function AgentDockTabs({ tabs, ariaLabel }: { tabs: AgentDockTab[]; ariaLabel: string }) {
  const rootRef = useRef<HTMLDivElement>(null);
  if (tabs.length === 0) return null;
  const activeId = tabs.find((tab) => tab.active)?.id ?? tabs[0]?.id;
  return (
    <TabsPrimitive.Root
      ref={rootRef}
      value={activeId}
      onValueChange={(id) => tabs.find((tab) => tab.id === id)?.onSelect?.()}
      className="agent-dock-tabs"
    >
      <TabsPrimitive.List aria-label={ariaLabel} className="contents" activateOnFocus>
        {tabs.map((tab) => {
          return (
            <div
              key={tab.id}
              data-active={tab.active ? "" : undefined}
              className={cn(
                "group flex h-[var(--dock-tab-height)] min-w-0 shrink-0 items-center rounded-[var(--dock-tab-radius)]",
                "text-fg-muted transition-[background-color,color] duration-[var(--dur-fast)] ease-out",
                "hover:bg-hover hover:text-fg focus-within:text-fg",
                "data-[active]:bg-selected data-[active]:text-fg",
              )}
            >
              <TabsPrimitive.Tab
                value={tab.id}
                data-chrome-focus=""
                className="inline-flex h-full min-w-0 max-w-40 items-center gap-1.5 rounded-[inherit] border-0 bg-transparent py-0 pl-2 pr-1 text-ui-sm font-normal text-inherit focus-visible:outline-none"
              >
                {tab.icon && <Icon name={tab.icon} size="sm" className="shrink-0 opacity-70" />}
                <span className="truncate">{tab.title}</span>
                {tab.badge != null && (
                  <span className="shrink-0 font-mono text-ui-2xs leading-none text-fg-faint tabular-nums">
                    {tab.badge}
                  </span>
                )}
              </TabsPrimitive.Tab>
              {tab.onClose && (
                <IconButton
                  icon="x"
                  size="xs"
                  quiet
                  title={tab.closeLabel}
                  onClick={() => {
                    tab.onClose?.();
                    requestAnimationFrame(() => {
                      rootRef.current
                        ?.querySelector<HTMLElement>('[role="tab"][data-active]')
                        ?.focus({ preventScroll: true });
                    });
                  }}
                  className="mr-0.5 invisible opacity-0 transition-opacity duration-[var(--dur-fast)] group-hover:visible group-hover:opacity-100 group-focus-within:visible group-focus-within:opacity-100"
                />
              )}
            </div>
          );
        })}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
