import type { ReactNode } from "react";
import { Icon, type IconName } from "@/ui/icons";
import { TabsPrimitive } from "@/ui/primitives";
import { SectionLabel } from "./section-label";

export interface VerticalTabItem {
  id: string;
  label: ReactNode;
  icon?: IconName;
  content: ReactNode;
}

export interface VerticalTabGroup {
  id: string;
  label: ReactNode;
  items: VerticalTabItem[];
}

interface VerticalTabsProps {
  ariaLabel: string;
  groups: VerticalTabGroup[];
  value?: string;
  onValueChange: (value: string | undefined) => void;
  /** Chrome above the rail's list — a window-corner bar, a back link, a filter.
   *  Outside the scroller, so it stays put and can be full-bleed. */
  railHeader?: ReactNode;
}

export function VerticalTabs({
  ariaLabel,
  groups,
  value,
  onValueChange,
  railHeader,
}: VerticalTabsProps) {
  const items = groups.flatMap((group) => group.items);
  return (
    <TabsPrimitive.Root
      orientation="vertical"
      // `null` is an intentional controlled "no matching tab" state. Passing
      // `undefined` when a settings filter hides every pane makes Base UI switch
      // from controlled to uncontrolled, then back again when the filter clears.
      value={value ?? null}
      onValueChange={(next) => onValueChange(next ? String(next) : undefined)}
      className="grid h-full w-full grid-cols-[256px_1fr] overflow-hidden bg-canvas"
    >
      <div data-split-side="end" className="agent-pane-split flex min-h-0 flex-col bg-surface">
        {railHeader}
        <TabsPrimitive.List
          className="flex min-h-0 flex-1 flex-col gap-px overflow-y-auto px-2 pb-6"
          aria-label={ariaLabel}
          activateOnFocus
        >
          {groups.map((group) => (
            <div key={group.id} className="flex flex-col gap-px">
              <SectionLabel className="px-2 pb-1 pt-4">{group.label}</SectionLabel>
              {group.items.map((item) => (
                <TabsPrimitive.Tab
                  key={item.id}
                  value={item.id}
                  className="flex h-[var(--control-height-md)] items-center gap-2.5 rounded-[var(--button-radius)] border-0 bg-transparent px-2.5 text-left font-sans text-ui-md leading-tight text-fg transition-[background-color] duration-[var(--dur-color)] ease-out hover:bg-hover focus-visible:outline-none data-[active]:bg-selected"
                >
                  {item.icon && <Icon name={item.icon} size="md" className="shrink-0" />}
                  <span className="truncate">{item.label}</span>
                </TabsPrimitive.Tab>
              ))}
            </div>
          ))}
        </TabsPrimitive.List>
      </div>
      <div className="min-h-0 min-w-0 overflow-y-auto bg-canvas">
        <div className="mx-auto max-w-[720px] px-6 py-8">
          {items.map((item) => (
            <TabsPrimitive.Panel key={item.id} value={item.id} className="outline-none">
              {item.content}
            </TabsPrimitive.Panel>
          ))}
        </div>
      </div>
    </TabsPrimitive.Root>
  );
}
