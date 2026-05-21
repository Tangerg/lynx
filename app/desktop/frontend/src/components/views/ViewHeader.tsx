// Shared header bar for workspace-view tabs: icon · title · subtitle · actions.
// Concentrates the markup so individual views don't redo the inline-style
// overrides they used to.

import type { ReactNode } from "react";
import { Icon, type IconName } from "@/components/common";

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
    <div className="view-head">
      <div className="view-icon"><Icon name={icon} size={14} /></div>
      <div style={{ minWidth: 0 }}>
        <div className={`view-title${titleStrong ? " strong" : ""}`}>{title}</div>
        {sub !== undefined && <div className="view-sub">{sub}</div>}
      </div>
      {actions !== undefined && (
        <div style={{ display: "flex", gap: 4 }}>{actions}</div>
      )}
    </div>
  );
}
