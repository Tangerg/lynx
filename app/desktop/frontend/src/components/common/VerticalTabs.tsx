import type { ReactNode } from "react";
import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import { Icon, type IconName } from "./Icon";
import { SectionLabel } from "./SectionLabel";

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
}

export function VerticalTabs({ ariaLabel, groups, value, onValueChange }: VerticalTabsProps) {
  const items = groups.flatMap((group) => group.items);
  return (
    <BaseTabs.Root
      orientation="vertical"
      value={value}
      onValueChange={(next) => onValueChange(next ? String(next) : undefined)}
      className="grid h-full w-full grid-cols-[236px_1fr] overflow-hidden"
    >
      <BaseTabs.List
        className="flex flex-col gap-0.5 overflow-y-auto bg-surface/45 px-3 py-8"
        aria-label={ariaLabel}
        activateOnFocus
      >
        {groups.map((group) => (
          <div key={group.id} className="flex flex-col gap-0.5">
            <SectionLabel>{group.label}</SectionLabel>
            {group.items.map((item) => (
              <BaseTabs.Tab
                key={item.id}
                value={item.id}
                className="flex items-center gap-2.5 rounded-md border-0 bg-transparent px-3 py-2 text-left text-[13px] text-fg-muted transition-colors duration-150 hover:bg-fg/[0.04] hover:text-fg data-[active]:bg-fg/[0.055] data-[active]:text-fg"
              >
                {item.icon && <Icon name={item.icon} size={15} className="shrink-0" />}
                <span className="truncate">{item.label}</span>
              </BaseTabs.Tab>
            ))}
          </div>
        ))}
      </BaseTabs.List>
      <div className="min-h-0 min-w-0 overflow-y-auto bg-canvas">
        <div className="mx-auto max-w-[920px] px-8 py-10">
          {items.map((item) => (
            <BaseTabs.Panel key={item.id} value={item.id} className="outline-none">
              {item.content}
            </BaseTabs.Panel>
          ))}
        </div>
      </div>
    </BaseTabs.Root>
  );
}
