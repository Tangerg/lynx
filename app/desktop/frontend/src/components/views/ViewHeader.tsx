// Shared header bar for workspace-view tabs: icon · title · subtitle · actions.

import type { ReactNode } from "react";
import { Icon, type IconName } from "@/components/common";
import { cn } from "@/lib/utils";

type Props = {
  icon: IconName;
  title: ReactNode;
  sub?: ReactNode;
  actions?: ReactNode;
  /**
   * Render the title in the UI font (13/700) instead of mono. Used by
   * views whose "title" is a label ("Notifications", "Connected MCP
   * servers") rather than a filename or process name.
   */
  titleStrong?: boolean;
};

export function ViewHeader({ icon, title, sub, actions, titleStrong }: Props) {
  return (
    <div className="grid grid-cols-[28px_1fr_auto] items-center gap-2.5 px-4 py-3.5">
      <div className="grid h-7 w-7 place-items-center rounded-md bg-surface-2 text-fg-muted">
        <Icon name={icon} size={14} />
      </div>
      <div className="min-w-0">
        <div className={cn(
          "text-fg whitespace-nowrap overflow-hidden text-ellipsis",
          titleStrong ? "font-sans text-[15px] font-semibold tracking-[-0.005em]" : "font-mono text-[13px]",
        )}>
          {title}
        </div>
        {sub !== undefined && (
          <div className="mt-0.5 font-mono text-[12px] text-fg-faint">{sub}</div>
        )}
      </div>
      {actions !== undefined && (
        <div className="flex gap-1">{actions}</div>
      )}
    </div>
  );
}
