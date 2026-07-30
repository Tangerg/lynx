import type { Translate } from "@/lib/i18n";
import type { Tone } from "@/lib/tone";
import type { AgentRunView, TimelineEntry } from "@/plugins/builtin/agent/public/viewState";
import type { AgentRunTreeNode } from "@/plugins/builtin/agent/public/run";
import {
  agentRunDetail,
  agentRunPresentationState,
  agentRunStepCount,
  type AgentRunPresentationState,
} from "@/plugins/builtin/agent/public/runPresentation";

export interface TimelineRunGroup {
  runId: string | null;
  run: AgentRunView | null;
  depth: number;
  items: TimelineEntry[];
}

export interface TimelineViewModel {
  eventCount: number;
  runCount: number;
  groups: TimelineRunGroup[];
}

export interface TimelineRunStatusView {
  state: AgentRunPresentationState;
  labelKey: string;
  tone: Tone;
  detail: string | null;
  stepCount: number;
  cancelable: boolean;
}

const STATUS_VIEW: Record<AgentRunPresentationState, { labelKey: string; tone: Tone }> = {
  running: { labelKey: "agent.runTree.status.running", tone: "accent" },
  waiting: { labelKey: "agent.runTree.status.waiting", tone: "warning" },
  finished: { labelKey: "agent.runTree.status.finished", tone: "success" },
  error: { labelKey: "agent.runTree.status.error", tone: "negative" },
  canceled: { labelKey: "agent.runTree.status.canceled", tone: "neutral" },
  limit: { labelKey: "agent.runTree.status.limit", tone: "warning" },
};

export function timelineViewModel(
  entries: readonly TimelineEntry[],
  roots: readonly AgentRunTreeNode[],
): TimelineViewModel {
  const entriesByRunId = new Map<string | null, TimelineEntry[]>();
  for (const entry of entries) {
    const items = entriesByRunId.get(entry.runId) ?? [];
    items.push(entry);
    entriesByRunId.set(entry.runId, items);
  }

  const groups: TimelineRunGroup[] = [];
  const sessionItems = entriesByRunId.get(null);
  if (sessionItems) {
    groups.push({ runId: null, run: null, depth: 0, items: sessionItems });
    entriesByRunId.delete(null);
  }

  appendRunGroups(groups, roots, entriesByRunId, 0);

  // Trust-boundary validation normally makes this empty. If an event still
  // references an unknown Run, retain it as an explicit audit group instead of
  // making evidence disappear from the Context Dock.
  for (const [runId, items] of entriesByRunId) {
    groups.push({ runId, run: null, depth: 0, items });
  }

  return {
    eventCount: entries.length,
    runCount: countRunNodes(roots),
    groups,
  };
}

function appendRunGroups(
  groups: TimelineRunGroup[],
  nodes: readonly AgentRunTreeNode[],
  entriesByRunId: Map<string | null, TimelineEntry[]>,
  depth: number,
): void {
  for (const node of nodes) {
    groups.push({
      runId: node.run.id,
      run: node.run,
      depth,
      items: entriesByRunId.get(node.run.id) ?? [],
    });
    entriesByRunId.delete(node.run.id);
    appendRunGroups(groups, node.children, entriesByRunId, depth + 1);
  }
}

function countRunNodes(nodes: readonly AgentRunTreeNode[]): number {
  let count = 0;
  for (const node of nodes) count += 1 + countRunNodes(node.children);
  return count;
}

export function timelineRunStatusView(run: AgentRunView): TimelineRunStatusView {
  const state = agentRunPresentationState(run);
  return {
    state,
    labelKey: STATUS_VIEW[state].labelKey,
    tone: STATUS_VIEW[state].tone,
    detail: agentRunDetail(run),
    stepCount: agentRunStepCount(run),
    cancelable: run.status !== "finished",
  };
}

export function timelineGroupKey(group: TimelineRunGroup): string {
  return group.runId ?? "session";
}

export function timelineSubtext(
  t: Translate,
  { eventCount, runCount }: Pick<TimelineViewModel, "eventCount" | "runCount">,
): string {
  return t("timeline.summary", { events: eventCount, runs: runCount });
}

export function timelineTimeOfDay(ts: number): string {
  const date = new Date(ts);
  const hh = String(date.getHours()).padStart(2, "0");
  const mm = String(date.getMinutes()).padStart(2, "0");
  const ss = String(date.getSeconds()).padStart(2, "0");
  return `${hh}:${mm}:${ss}`;
}
