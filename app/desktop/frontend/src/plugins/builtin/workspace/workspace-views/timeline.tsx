// Built-in plugin: "Timeline" workspace view — the per-thread audit log
// of run-significant events accumulated by the protocol reducer.
//
// UX review §2.2 / §4.1: users need a single surface that answers
// "what did the agent actually do this run". Tool cards live inline in
// the message stream; this view aggregates them with run/step/approval/
// error boundaries so the answer reads chronologically.
//
// Pure renderer — data comes from the agent public run read model.

import type { IconName } from "@/components/common";
import type { TimelineEntry, TimelineEntryKind } from "@/protocol/run/viewState";
import { EmptyState, Icon, IconButton } from "@/components/common";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { cn } from "@/lib/utils";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useActiveRunTimeline } from "@/plugins/builtin/agent/public/run";
import { selectWorkspaceChat } from "@/plugins/builtin/workspace/public/navigation";

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

function timeOfDay(ts: number): string {
  const d = new Date(ts);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  const ss = String(d.getSeconds()).padStart(2, "0");
  return `${hh}:${mm}:${ss}`;
}

// Group consecutive entries by runId so the view reads as "this run did
// X, Y, Z". Entries with the same runId stay together; null runIds
// (events from before RUN_STARTED, edge case) group on their own.
function groupByRun(entries: TimelineEntry[]): { runId: string | null; items: TimelineEntry[] }[] {
  const groups: { runId: string | null; items: TimelineEntry[] }[] = [];
  for (const entry of entries) {
    const last = groups.at(-1);
    if (last && last.runId === entry.runId) {
      last.items.push(entry);
    } else {
      groups.push({ runId: entry.runId, items: [entry] });
    }
  }
  return groups;
}

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
        {timeOfDay(entry.ts)}
      </span>
    </div>
  );
}

function TimelineTab() {
  const t = useT();
  const timeline = useActiveRunTimeline();
  const groups = groupByRun(timeline);
  const runCount = groups.filter((g) => g.runId !== null).length;

  return (
    <WorkspaceViewLayout
      icon="history"
      titleStrong
      title="timeline.title"
      sub={`${timeline.length} events · ${runCount} run${runCount === 1 ? "" : "s"}`}
      scrollClassName="py-1"
      actions={
        <IconButton title={t("timeline.jumpToChat")} onClick={selectWorkspaceChat}>
          <Icon name="chat" size={14} />
        </IconButton>
      }
    >
      {timeline.length === 0 ? (
        <EmptyState
          icon="history"
          title={t("timeline.empty.title")}
          sub={t("timeline.empty.sub")}
        />
      ) : (
        groups.map((g, gi) => (
          <div
            key={`${g.runId ?? "pre"}:${gi}`}
            className={cn(gi > 0 && "mt-2 border-t border-line-soft pt-2")}
          >
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
  openByDefault: false,
  // Sits between Diff (10) / Files (20) / Plan (30) and Tools (40).
  // Timeline is "what happened" — closer to Plan than Tools.
  order: 35,
  splittable: true,
  component: TimelineTab,
});
