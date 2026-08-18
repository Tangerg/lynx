import { useLayoutEffect, useRef, type ReactNode } from "react";
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
 *
 * It carries no state of its own. Whether the flank is showing is a fact about the row
 * it and the conversation share — the row is what has to reflow, and the bar that
 * reaches the plane's trailing corner has to know it too — so the row declares it once
 * and this stays pure structure, the same way the drawer reads it off the shell.
 */
export function AgentContextDock({ children }: { children: ReactNode }) {
  return <aside className="agent-context-dock agent-pane-split">{children}</aside>;
}

function reflectDockTabOverflow(element: HTMLElement): void {
  const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
  element.toggleAttribute("data-overflow-start", element.scrollLeft > 1);
  element.toggleAttribute("data-overflow-end", maxScrollLeft - element.scrollLeft > 1);
}

function keepActiveDockTabInsideFade(element: HTMLElement): void {
  if (element.clientWidth === 0) return;
  const active = element.querySelector<HTMLElement>('[role="tab"][data-active]');
  if (!active) return;
  const stripBox = element.getBoundingClientRect();
  const activeBox = active.getBoundingClientRect();
  const edgeHint = 16;
  if (activeBox.left < stripBox.left + edgeHint) {
    element.scrollLeft -= stripBox.left + edgeHint - activeBox.left;
  } else if (activeBox.right > stripBox.right - edgeHint) {
    element.scrollLeft += activeBox.right - (stripBox.right - edgeHint);
  }
}

/** Dock tabs share one structural pattern while the visual style chooses the
 *  active treatment (quiet chip, underline, or elevation). The tab primitive
 *  owns roving focus and arrow-key navigation; styling buttons to resemble tabs
 *  without those semantics made the dock keyboard-hostile. */
export function AgentDockTabs({ tabs, ariaLabel }: { tabs: AgentDockTab[]; ariaLabel: string }) {
  const rootRef = useRef<HTMLDivElement>(null);
  const activeId = tabs.find((tab) => tab.active)?.id ?? tabs[0]?.id;
  useLayoutEffect(() => {
    const root = rootRef.current;
    root
      ?.querySelector<HTMLElement>('[role="tab"][data-active]')
      ?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    if (root) {
      keepActiveDockTabInsideFade(root);
      reflectDockTabOverflow(root);
    }
  }, [activeId]);
  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const reflect = () => reflectDockTabOverflow(root);
    const reconcileGeometry = () => {
      keepActiveDockTabInsideFade(root);
      reflect();
    };
    root.addEventListener("scroll", reflect, { passive: true });
    const resizeObserver = new ResizeObserver(reconcileGeometry);
    resizeObserver.observe(root);
    root.querySelectorAll<HTMLElement>('[role="tablist"] > *').forEach((tab) => {
      resizeObserver.observe(tab);
    });
    reconcileGeometry();
    return () => {
      root.removeEventListener("scroll", reflect);
      resizeObserver.disconnect();
    };
  }, [tabs.length]);
  if (tabs.length === 0) return null;
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
                // Keep labels readable; the strip owns horizontal overflow and
                // brings the active identity into view when navigation changes it.
                "group flex h-[var(--dock-tab-height)] min-w-0 shrink-0 items-center rounded-[var(--dock-tab-radius)]",
                "text-fg-muted transition-[background-color,color] duration-[var(--dur-color)] ease-out",
                "hover:bg-hover hover:text-fg focus-within:text-fg",
                // The selected tab fills with the PANEL's ground, not with a
                // selection wash: a tab is the top of the thing it opens, and the
                // shared value is the only part of that claim a static bar can
                // make. A wash instead made the strip read as a row of chips that
                // happened to sit above some content.
                "data-[active]:bg-[var(--dock-tab-active-surface)] data-[active]:text-fg",
              )}
            >
              <TabsPrimitive.Tab
                value={tab.id}
                data-chrome-focus=""
                className={cn(
                  "inline-flex h-full min-w-0 max-w-40 items-center gap-1.5 rounded-[inherit] border-0 bg-transparent py-0 text-ui-sm font-normal text-inherit focus-visible:outline-none",
                  // Symmetric unless a close button already occupies the trailing
                  // side — the inset is there to keep the label off the tab's edge,
                  // and with a control there the control is what does that.
                  tab.onClose ? "pl-2 pr-1" : "px-2",
                )}
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
