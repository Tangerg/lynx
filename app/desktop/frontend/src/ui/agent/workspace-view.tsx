import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui/atoms/icon-button";

/**
 * The frame every dock view fills: a column on the canvas that owns no chrome of
 * its own beyond the ground it stands on.
 *
 * It is also the container the two tracks below measure themselves against —
 * what decides whether a navigator fits beside a diff is the width of THIS view,
 * which the dock's resize handle changes without the window changing at all.
 */
export function AgentWorkspaceView({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("agent-workspace-view flex min-h-0 flex-1 flex-col bg-canvas", className)}>
      {children}
    </div>
  );
}

/**
 * A view's body split into a navigator and the content it navigates.
 *
 * The caller passes two slots and no geometry. Which track yields at which width
 * is a fact about this shape, not about diffs or file trees, and it had been
 * spelled as a percentage at the one callsite that has it — a share is the wrong
 * response here, because the navigator wants a roughly constant width (a path is
 * as wide as a path) while the content has a hard floor (a line of code). Below
 * the width where both fit, the navigator withdraws and takes its toggle with it
 * (see the container query in globals.css).
 */
export function AgentViewSplit({
  navigator,
  children,
}: {
  /** An `AgentViewNavigator`, or nothing when the caller has it hidden. */
  navigator?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="agent-view-split" data-navigator={navigator ? "" : undefined}>
      <div className="agent-view-body">{children}</div>
      {navigator}
    </div>
  );
}

/**
 * The control that shows and hides the navigator.
 *
 * Part of the split's contract rather than the view's own header furniture: the
 * width at which a navigator stops fitting is this shape's to know, and the
 * control has to disappear on the same breakpoint or it becomes a button that
 * reports a state nothing on screen can reach.
 */
export function AgentViewNavigatorToggle({
  open,
  onToggle,
  showLabel,
  hideLabel,
}: {
  open: boolean;
  onToggle: () => void;
  showLabel: string;
  hideLabel: string;
}) {
  return (
    <IconButton
      icon="list"
      size="sm"
      aria-pressed={open}
      title={open ? hideLabel : showLabel}
      onClick={onToggle}
      className="agent-view-navigator-toggle"
    />
  );
}

/**
 * The navigator track: the seam against the content, its width, and an optional
 * control strip above the list.
 *
 * The seam is the dock's own pane split, which is why this is a shape here and
 * not a class a view reaches for — a boundary is drawn by whatever owns both
 * sides of it.
 */
export function AgentViewNavigator({
  label,
  header,
  children,
}: {
  label: string;
  /** Filter field, close button — the strip that sits over the list. */
  header?: ReactNode;
  children: ReactNode;
}) {
  return (
    <aside aria-label={label} className="agent-view-navigator agent-pane-split">
      {header && <div className="agent-view-navigator-header">{header}</div>}
      {children}
    </aside>
  );
}
