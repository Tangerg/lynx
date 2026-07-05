// Built-in plugin: "Timeline" workspace view — the per-thread audit log
// of run-significant events accumulated by the protocol reducer.
//
// UX review §2.2 / §4.1: users need a single surface that answers
// "what did the agent actually do this run". Tool cards live inline in
// the message stream; this view aggregates them with run/step/approval/
// error boundaries so the answer reads chronologically.
//
// Pure renderer — data comes from the agent public run read model.

import type { IconName } from "@/ui";
import type { TimelineEntry, TimelineEntryKind } from "@/plugins/builtin/agent/public/viewState";
import { EmptyState, Icon, IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { cn } from "@/lib/utils";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useActiveRunTimeline } from "@/plugins/builtin/agent/public/run";
import { selectWorkspaceChat } from "@/plugins/builtin/workspace/public/navigation";
import {
  timelineGroupKey,
  timelineSubtext,
  timelineTimeOfDay,
  timelineViewModel,
} from "@/plugins/builtin/workspace/application/timelineViewModel";

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

function TimelineRow({ entry }: { entry: TimelineEntry }) {
  const t = useT();
  const icon = KIND_ICON[entry.kind];
  return (
    <div className="flex items-start gap-2.5 px-3.5 py-1.5">
      <Icon name={icon} size={12} className="mt-1 shrink-0 text-fg-faint" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="shrink-0 text-[11.5px] font-medium text-fg">
            {t(KIND_I18N[entry.kind])}
          </span>
          {entry.summary && (
            // `title=` preserves full text when the inline column
            // truncates a long command / tool name on hover.
            <span title={entry.summary} className="truncate font-mono text-[11.5px] text-fg-muted">
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
      <span className="mt-0.5 shrink-0 font-mono text-[10.5px] text-fg-faint">
        {timelineTimeOfDay(entry.ts)}
      </span>
    </div>
  );
}

function TimelineTab() {
  const t = useT();
  const timeline = useActiveRunTimeline();
  const view = timelineViewModel(timeline);

  return (
    <WorkspaceViewLayout
      icon="history"
      titleStrong
      title="timeline.title"
      sub={timelineSubtext(view)}
      scrollClassName="py-1"
      actions={
        <IconButton title={t("timeline.jumpToChat")} onClick={selectWorkspaceChat}>
          <Icon name="chat" size={14} />
        </IconButton>
      }
    >
      {view.eventCount === 0 ? (
        <EmptyState
          icon="history"
          title={t("timeline.empty.title")}
          sub={t("timeline.empty.sub")}
        />
      ) : (
        view.groups.map((g, gi) => (
          <div key={timelineGroupKey(g, gi)} className={cn(gi > 0 && "mt-3 pt-1")}>
            {g.runId && (
              <div className="px-3.5 pb-1 font-mono text-[10px] text-fg-faint">run {g.runId}</div>
            )}
            {g.items.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} />
            ))}
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
  order: 35,
  splittable: true,
  component: TimelineTab,
});
