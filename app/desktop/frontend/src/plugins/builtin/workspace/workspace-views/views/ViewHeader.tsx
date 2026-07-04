// Shared header bar for workspace-view tabs: icon · title · subtitle · actions.

import type { ReactNode } from "react";
import type { IconName } from "@/components/common";
import { Icon, IconButton } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";

export interface ViewHeaderProps {
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
}

export function ViewHeader({ icon, title, sub, actions, titleStrong }: ViewHeaderProps) {
  // Placement toggle — present only when this view is promoted (ChatPanel
  // provides the context). Lets the view move full ↔ beside-chat / close from
  // its own header, leaving the tab strip untouched.
  const placement = useViewPlacement();
  const t = useT();
  let placementControls: ReactNode = null;
  if (placement?.placement === "split") {
    // Promote the side pane to a full-width tab, or close it (chat returns to
    // full width). Promote sits before close to mirror the tab strip's order.
    placementControls = (
      <div className="flex items-center gap-0.5">
        <IconButton title={t("workspace.view.promote")} onClick={placement.onPromote}>
          <Icon name="maximize" size={13} />
        </IconButton>
        <IconButton title={t("common.close")} onClick={placement.onClose}>
          <Icon name="x" size={14} />
        </IconButton>
      </div>
    );
  } else if (placement?.placement === "full" && placement.splittable) {
    placementControls = (
      <IconButton title={t("workspace.view.openBeside")} onClick={placement.onSplit}>
        <Icon name="panel-r" size={14} />
      </IconButton>
    );
  }

  return (
    <div className="grid min-h-12 grid-cols-[28px_1fr_auto] items-center gap-2.5 px-3.5 py-2.5">
      <div className="grid h-7 w-7 place-items-center rounded-md bg-surface-2 text-fg-muted">
        <Icon name={icon} size={14} />
      </div>
      <div className="min-w-0">
        <div
          className={cn(
            "text-fg whitespace-nowrap overflow-hidden text-ellipsis",
            titleStrong
              ? "font-sans text-[14px] font-semibold tracking-[-0.01em]"
              : "font-mono text-[13px]",
          )}
        >
          {/* A string title is an i18n key (built-in views) or a literal
              (filenames, third-party) — t() resolves the former, passes the
              latter through. Non-string titles (ReactNode) render as-is. */}
          {typeof title === "string" ? t(title) : title}
        </div>
        {sub !== undefined && (
          // Label views get a sans subtitle (descriptive text); filename /
          // process views keep mono (the sub is a path / stat there).
          <div
            className={cn(
              "mt-0.5 text-[12px] text-fg-faint",
              titleStrong ? "font-sans" : "font-mono",
            )}
          >
            {sub}
          </div>
        )}
      </div>
      {(actions !== undefined || placementControls) && (
        <div className="flex gap-1">
          {actions}
          {placementControls}
        </div>
      )}
    </div>
  );
}
