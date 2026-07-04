import type { ReactNode } from "react";
import { AgentSectionLabel, StudioTabs } from "@/components/agent-studio";
import { Icon, type IconName } from "./Icon";

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
    <StudioTabs.Root
      orientation="vertical"
      value={value}
      onValueChange={(next) => onValueChange(next ? String(next) : undefined)}
      className="grid h-full w-full grid-cols-[260px_1fr] overflow-hidden bg-canvas"
    >
      <StudioTabs.List
        className="flex flex-col gap-px overflow-y-auto bg-surface px-4 py-8 shadow-[inset_-0.5px_0_0_var(--color-field)]"
        aria-label={ariaLabel}
        activateOnFocus
      >
        {groups.map((group) => (
          <div key={group.id} className="flex flex-col gap-px">
            <AgentSectionLabel className="px-2 pb-1 pt-4">{group.label}</AgentSectionLabel>
            {group.items.map((item) => (
              <StudioTabs.Tab
                key={item.id}
                value={item.id}
                className="flex h-8 items-center gap-2.5 rounded-[8px] border-0 bg-transparent px-2.5 text-left font-sans text-[13px] leading-none text-fg-soft transition-[background-color,color] duration-[120ms] ease-out hover:bg-fg/[0.045] hover:text-fg focus-visible:outline-none data-[active]:bg-fg/[0.075] data-[active]:text-fg"
              >
                {item.icon && <Icon name={item.icon} size={15} className="shrink-0" />}
                <span className="truncate">{item.label}</span>
              </StudioTabs.Tab>
            ))}
          </div>
        ))}
      </StudioTabs.List>
      <div className="min-h-0 min-w-0 overflow-y-auto bg-canvas">
        <div className="mx-auto max-w-[760px] px-8 py-10">
          {items.map((item) => (
            <StudioTabs.Panel key={item.id} value={item.id} className="outline-none">
              {item.content}
            </StudioTabs.Panel>
          ))}
        </div>
      </div>
    </StudioTabs.Root>
  );
}
