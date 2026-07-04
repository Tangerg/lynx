// Shared header bar for workspace-view tabs: icon · title · subtitle · actions.

import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { AgentIconButton } from "@/ui/agent";
import { Icon } from "@/ui";
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
      <div className="flex items-center gap-1">
        <AgentIconButton
          icon="maximize"
          size="sm"
          title={t("workspace.view.promote")}
          aria-label={t("workspace.view.promote")}
          onClick={placement.onPromote}
        />
        <AgentIconButton
          icon="x"
          size="sm"
          title={t("common.close")}
          aria-label={t("common.close")}
          onClick={placement.onClose}
        />
      </div>
    );
  } else if (placement?.placement === "full" && placement.splittable) {
    placementControls = (
      <AgentIconButton
        icon="panel-r"
        size="sm"
        title={t("workspace.view.openBeside")}
        aria-label={t("workspace.view.openBeside")}
        onClick={placement.onSplit}
      />
    );
  }

  return (
    <div className="flex h-[52px] shrink-0 items-center gap-2 border-b-[0.5px] border-field/70 px-3.5">
      <Icon name={icon} size={15} strokeWidth={1.8} className="shrink-0 text-fg-muted" />
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <span
          className={cn(
            "min-w-0 truncate text-fg",
            titleStrong
              ? "font-sans text-[13.5px] font-semibold"
              : "font-mono text-[12.5px] font-medium",
          )}
        >
          {/* A string title is an i18n key (built-in views) or a literal
              (filenames, third-party) — t() resolves the former, passes the
              latter through. Non-string titles (ReactNode) render as-is. */}
          {typeof title === "string" ? t(title) : title}
        </span>
        {sub !== undefined && (
          <>
            <span aria-hidden="true" className="shrink-0 text-[13px] leading-none text-fg-faint">
              ·
            </span>
            <span
              className={cn(
                "min-w-0 truncate text-[11.5px] text-fg-faint",
                titleStrong ? "font-sans" : "font-mono",
              )}
            >
              {sub}
            </span>
          </>
        )}
      </div>
      {(actions !== undefined || placementControls) && (
        <div className="flex shrink-0 items-center gap-1">
          {actions}
          {placementControls}
        </div>
      )}
    </div>
  );
}
