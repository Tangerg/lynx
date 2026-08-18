// The bar a workspace view puts above its body — one shape per placement.
//
// Full width, the view owns the window's top-left corner, so the bar carries the
// drawer toggle, the view's identity and its way out. In the dock, the dock's own
// bar already names the view and owns the container controls, so this one appears
// only when the view has something of its own to add (subtext, actions) — two
// stacked bars saying the same word in a 400px column was the old shape.

import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
import { Icon, IconButton } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useViewPlacement } from "@/plugins/builtin/workspace/public/viewPlacement";

export interface ViewHeaderProps {
  icon: IconName;
  title: ReactNode;
  /** Material identity that remains meaningful inside a generic dock tab. */
  dockIdentity?: ReactNode;
  sub?: ReactNode;
  actions?: ReactNode;
  /**
   * Render the title in the UI font (13/700) instead of mono. Used by
   * views whose "title" is a label ("Notifications", "Connected MCP
   * servers") rather than a filename or process name.
   */
  titleStrong?: boolean;
}

export function ViewHeader({
  icon,
  title,
  dockIdentity,
  sub,
  actions,
  titleStrong,
}: ViewHeaderProps) {
  const placement = useViewPlacement();
  if (placement?.placement === "dock") {
    return <DockViewBar identity={dockIdentity} sub={sub} actions={actions} />;
  }
  return (
    <FullViewBar icon={icon} title={title} sub={sub} actions={actions} titleStrong={titleStrong} />
  );
}

/** Material identity, subtext and per-view actions only — the dock tab already
 * carries the generic view name. */
function DockViewBar({
  identity,
  sub,
  actions,
}: Pick<ViewHeaderProps, "sub" | "actions"> & { identity?: ReactNode }) {
  if (identity === undefined && sub === undefined && actions === undefined) return null;
  return (
    <AgentSurfaceHeader className="gap-2">
      <div className="flex min-w-0 flex-1 items-center gap-2 font-mono text-ui-md text-fg-muted">
        {identity !== undefined && <span className="min-w-0 flex-1">{identity}</span>}
        {identity !== undefined && sub !== undefined && (
          <span aria-hidden className="shrink-0 leading-none text-fg-faint">
            ·
          </span>
        )}
        {sub !== undefined && (
          <span className={cn("truncate", identity === undefined ? "min-w-0 flex-1" : "shrink-0")}>
            {sub}
          </span>
        )}
      </div>
      {actions !== undefined && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
    </AgentSurfaceHeader>
  );
}

function FullViewBar({ icon, title, sub, actions, titleStrong }: ViewHeaderProps) {
  const placement = useViewPlacement();
  const t = useT();

  return (
    // Every chrome bar in the app is this component: one height
    // (`--surface-header-height`), one inset, one bottom hairline, one drag
    // region. A view can be opened beside the chat, so its header sits directly
    // next to that one and any divergence reads as two different kinds of bar.
    <AgentSurfaceHeader className="gap-2" windowCorner>
      <Icon name={icon} size="md" className="shrink-0 text-fg-muted" />
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <span
          className={cn(
            "min-w-0 truncate text-ui-md font-medium text-fg",
            // A label title ("Notifications") reads in the UI face; a filename /
            // process title stays mono so paths and identifiers align.
            titleStrong ? "font-sans" : "font-mono",
          )}
        >
          {/* A string title is an i18n key (built-in views) or a literal
              (filenames, third-party) — t() resolves the former, passes the
              latter through. Non-string titles (ReactNode) render as-is. */}
          {typeof title === "string" ? t(title) : title}
        </span>
        {sub !== undefined && (
          <>
            <span aria-hidden="true" className="shrink-0 text-ui-md leading-none text-fg-faint">
              ·
            </span>
            <span className="min-w-0 truncate font-mono text-ui-md text-fg-muted">{sub}</span>
          </>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {actions}
        {placement?.splittable && (
          <IconButton
            icon="panel-r"
            size="sm"
            title={t("workspace.view.openBeside")}
            onClick={placement.onOpenInDock}
          />
        )}
        {/* The way out. Without it, a maximised view left Escape (which an
            input steals) and ⌘W as the only exits. */}
        {placement && (
          <IconButton icon="x" size="sm" title={t("common.close")} onClick={placement.onClose} />
        )}
      </div>
    </AgentSurfaceHeader>
  );
}
