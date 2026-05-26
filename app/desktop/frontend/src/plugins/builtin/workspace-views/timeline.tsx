// Built-in plugin: "Timeline" workspace view — the per-thread audit log
// of run-significant events accumulated by the AG-UI reducer.
//
// UX review §2.2 / §4.1: users need a single surface that answers
// "what did the agent actually do this run". Tool cards live inline in
// the message stream; this view aggregates them with run/step/approval/
// error boundaries so the answer reads chronologically.
//
// Pure renderer — the data lives on agentStore (`view.timeline`) and is
// populated by core-reducer + agui-handlers. See viewState.TimelineEntry
// for the entry shape.

import type { IconName } from "@/components/common";
import type { TimelineEntry, TimelineEntryKind } from "@/protocol/agui/viewState";
import { EmptyState, Icon, IconButton, ScrollArea } from "@/components/common";
import { ViewHeader } from "@/components/views/ViewHeader";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useAgentSlice } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

// kind → icon + display label. Lookup table beats a switch — adding a
// new TimelineEntryKind is one row here.
const KIND_META: Record<TimelineEntryKind, { icon: IconName; label: string }> = {
  "run-start": { icon: "play", label: "Run started" },
  "run-end": { icon: "check", label: "Run finished" },
  "run-error": { icon: "bug", label: "Run error" },
  "step-start": { icon: "skip-fwd", label: "Step started" },
  "step-end": { icon: "skip-back", label: "Step finished" },
  "tool-start": { icon: "tool", label: "Tool" },
  "tool-end": { icon: "tool", label: "Tool finished" },
  "reasoning-start": { icon: "spark", label: "Thinking" },
  "reasoning-end": { icon: "spark", label: "Thought" },
  "approval-request": { icon: "shield", label: "Approval requested" },
  "approval-result": { icon: "shield", label: "Approval" },
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
    const last = groups[groups.length - 1];
    if (last && last.runId === entry.runId) {
      last.items.push(entry);
    } else {
      groups.push({ runId: entry.runId, items: [entry] });
    }
  }
  return groups;
}

const STATUS_DOT: Record<NonNullable<TimelineEntry["status"]>, string> = {
  ok: "bg-positive",
  err: "bg-negative",
  approved: "bg-positive",
  declined: "bg-warning",
};

function TimelineRow({ entry }: { entry: TimelineEntry }) {
  const meta = KIND_META[entry.kind];
  return (
    <div className="flex items-start gap-2.5 px-3.5 py-1.5">
      <Icon name={meta.icon} size={12} className="mt-1 shrink-0 text-fg-faint" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="shrink-0 text-[11.5px] font-medium text-fg">{meta.label}</span>
          {entry.summary && (
            // `title=` preserves full text when the inline column
            // truncates a long command / tool name on hover.
            <span
              title={entry.summary}
              className="truncate font-mono text-[11.5px] text-fg-muted"
            >
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
  const timeline = useAgentSlice((v) => v.timeline);
  const groups = groupByRun(timeline);
  const runCount = groups.filter((g) => g.runId !== null).length;

  return (
    <>
      <ViewHeader
        icon="history"
        titleStrong
        title="Run timeline"
        sub={`${timeline.length} events · ${runCount} run${runCount === 1 ? "" : "s"}`}
        actions={
          <IconButton title="Jump to chat" onClick={() => useSessionStore.getState().selectChat()}>
            <Icon name="chat" size={14} />
          </IconButton>
        }
      />
      <ScrollArea className="py-1">
        {timeline.length === 0 ? (
          <EmptyState
            icon="history"
            title="No activity yet"
            sub="As the agent runs, every tool call, approval, and run boundary lands here."
          />
        ) : (
          groups.map((g, gi) => (
            <div
              key={`${g.runId ?? "pre"}:${gi}`}
              className={cn(gi > 0 && "mt-2 border-t border-line-soft pt-2")}
            >
              {g.runId && (
                <div className="px-3.5 pb-1 font-mono text-[10px] uppercase tracking-wider text-fg-faint">
                  run {g.runId}
                </div>
              )}
              {g.items.map((entry) => (
                <TimelineRow key={entry.id} entry={entry} />
              ))}
            </div>
          ))
        )}
      </ScrollArea>
    </>
  );
}

export const timelineView = definePlugin({
  name: "lyra.builtin.view-timeline",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "timeline",
      title: "Timeline",
      icon: "history",
      openByDefault: false,
      // Sits between Diff (10) / Files (20) / Plan (30) and Tools (40).
      // Timeline is "what happened" — closer to Plan than Tools.
      order: 35,
      component: TimelineTab,
    });
  },
});
