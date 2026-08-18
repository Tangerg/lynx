// Built-in plugin: "Timeline" workspace view — the per-thread audit log
// of run-significant events accumulated by the protocol reducer.
//
// UX review §2.2 / §4.1: users need a single surface that answers
// "what did the agent actually do this run". Tool cards live inline in
// the message stream; this view aggregates them under durable Run lineage,
// then keeps each source Run's events chronological.
//
// Pure renderer — data comes from the agent public run read model.

import type { IconName } from "@/ui";
import type { TimelineEntry, TimelineEntryKind } from "@/plugins/builtin/agent/public/viewState";
import { Badge, EmptyState, Icon, IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { cn } from "@/lib/classNames";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  cancelSessionRun,
  useActiveSessionRunTree,
  useActiveSessionTimeline,
} from "@/plugins/builtin/agent/public/run";
import {
  locateWorkspaceTool,
  selectWorkspaceChat,
} from "@/plugins/builtin/workspace/public/navigation";
import {
  timelineGroupKey,
  type TimelineRunGroup,
  timelineRunStatusView,
  timelineSubtext,
  timelineTimeOfDay,
  timelineViewModel,
} from "@/plugins/builtin/workspace/application/timelineViewModel";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

// i18n key → icon. Labels are resolved at render via t().
const KIND_ICON: Record<TimelineEntryKind, IconName> = {
  "run-start": "play",
  "run-end": "check",
  "run-error": "bug",
  "tool-start": "tool",
  "tool-end": "tool",
  "approval-request": "shield",
  "approval-result": "shield",
};

const KIND_I18N: Record<TimelineEntryKind, string> = {
  "run-start": "timeline.kind.runStart",
  "run-end": "timeline.kind.runEnd",
  "run-error": "timeline.kind.runError",
  "tool-start": "timeline.kind.toolStart",
  "tool-end": "timeline.kind.toolEnd",
  "approval-request": "timeline.kind.approvalRequest",
  "approval-result": "timeline.kind.approvalResult",
};

const STATUS_DOT: Record<NonNullable<TimelineEntry["status"]>, string> = {
  ok: "bg-success",
  err: "bg-negative",
  approved: "bg-success",
  declined: "bg-warning",
};

const TREE_INDENT = ["", "ml-3", "ml-6", "ml-9", "ml-12", "ml-16"] as const;

function TimelineRow({ entry }: { entry: TimelineEntry }) {
  const t = useT();
  const icon = KIND_ICON[entry.kind];
  return (
    <div className="flex items-start gap-2.5 px-3.5 py-1.5">
      <Icon name={icon} size="xs" className="mt-1 shrink-0 text-fg-faint" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="shrink-0 text-ui-sm font-medium text-fg">
            {t(KIND_I18N[entry.kind])}
          </span>
          {entry.summary && (
            // `title=` preserves full text when the inline column
            // truncates a long command / tool name on hover.
            <span title={entry.summary} className="truncate font-mono text-ui-sm text-fg-muted">
              {entry.summary}
            </span>
          )}
        </div>
      </div>
      {entry.status && (
        <span
          aria-label={entry.status}
          className={cn("mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full", STATUS_DOT[entry.status])}
        />
      )}
      <span className="mt-0.5 shrink-0 font-mono text-ui-xs text-fg-faint">
        {timelineTimeOfDay(entry.ts)}
      </span>
    </div>
  );
}

function TimelineRunHeader({
  group,
  runtimeAvailable,
}: {
  group: TimelineRunGroup;
  runtimeAvailable: boolean;
}) {
  const t = useT();
  const run = group.run;
  if (!run) {
    return group.runId ? (
      <div className="px-3.5 pb-1 font-mono text-ui-xs text-fg-faint">
        {t("timeline.unknownRun", { id: group.runId })}
      </div>
    ) : null;
  }

  const status = timelineRunStatusView(run);
  const parentRunId = run.parentRunId;
  const spawnedByItemId = run.spawnedByItemId;
  const child = parentRunId !== null;
  return (
    <div className="flex min-h-10 items-center gap-2 rounded-md bg-sunken pl-3">
      <Icon name={child ? "bot" : "branch"} size="sm" className="shrink-0 text-fg-muted" />
      <div className="min-w-0 flex-1 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 text-ui-sm font-semibold text-fg">
            {t(child ? "timeline.delegatedRun" : "timeline.rootRun")}
          </span>
          <span title={run.id} className="truncate font-mono text-ui-xs text-fg-faint">
            {run.id}
          </span>
          <Badge tone={status.tone}>{t(status.labelKey)}</Badge>
        </div>
        <div className="mt-0.5 flex min-w-0 gap-2 text-ui-xs text-fg-muted">
          {status.detail && (
            <span title={status.detail} className="truncate text-pretty">
              {status.detail}
            </span>
          )}
          {child && (
            <span title={parentRunId} className="truncate font-mono text-fg-faint">
              {t("timeline.parentRun", { id: parentRunId })}
            </span>
          )}
          <span className="ml-auto shrink-0 font-mono tabular-nums">
            {t("agent.steps", { count: status.stepCount })}
          </span>
        </div>
      </div>
      {spawnedByItemId && (
        <IconButton
          icon="chat"
          size="lg"
          quiet
          disabled={!runtimeAvailable}
          title={t("timeline.locateParent")}
          onClick={() => locateWorkspaceTool(spawnedByItemId)}
        />
      )}
      {status.cancelable && (
        <IconButton
          icon="stop"
          size="lg"
          quiet
          disabled={!runtimeAvailable}
          title={t("agent.runTree.action.cancel")}
          onClick={() => {
            cancelSessionRun({ sessionId: run.sessionId, runId: run.id });
          }}
        />
      )}
    </div>
  );
}

function TimelineTab() {
  const t = useT();
  const timeline = useActiveSessionTimeline();
  const runTree = useActiveSessionRunTree();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const view = timelineViewModel(timeline, runTree);

  return (
    <WorkspaceViewLayout
      icon="history"
      titleStrong
      title="timeline.title"
      sub={timelineSubtext(t, view)}
      scrollClassName="py-1"
      actions={
        <IconButton
          icon="chat"
          iconSize="sm"
          title={t("timeline.jumpToChat")}
          onClick={selectWorkspaceChat}
        />
      }
    >
      {view.groups.length === 0 ? (
        <EmptyState
          icon="history"
          title={t("timeline.empty.title")}
          sub={t("timeline.empty.sub")}
        />
      ) : (
        view.groups.map((group, index) => (
          <div
            key={timelineGroupKey(group)}
            className={cn(
              index > 0 && "mt-3 pt-1",
              TREE_INDENT[Math.min(group.depth, TREE_INDENT.length - 1)],
              group.depth > 0 && "border-l border-field pl-2",
            )}
          >
            <TimelineRunHeader group={group} runtimeAvailable={runtimeAvailable} />
            {group.items.length > 0 ? (
              group.items.map((entry) => <TimelineRow key={entry.id} entry={entry} />)
            ) : (
              <p className="px-3.5 py-2 text-pretty text-ui-xs text-fg-faint">
                {t("timeline.noEvents")}
              </p>
            )}
          </div>
        ))
      )}
    </WorkspaceViewLayout>
  );
}

export const timelineView = defineWorkspaceView({
  id: "timeline",
  title: "workspace.view.title.timeline",
  icon: "history",
  // Sits between Diff (10) / Files (20) / Plan (30) and Tools (40).
  // Timeline is "what happened" — closer to Plan than Tools.
  order: 140,
  splittable: true,
  component: TimelineTab,
});
