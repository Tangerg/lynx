import type { TimelineEntry } from "@/plugins/builtin/agent/public/viewState";

export interface TimelineRunGroup {
  runId: string | null;
  items: TimelineEntry[];
}

export interface TimelineViewModel {
  eventCount: number;
  runCount: number;
  groups: TimelineRunGroup[];
}

export function timelineViewModel(entries: readonly TimelineEntry[]): TimelineViewModel {
  const groups = groupTimelineByRun(entries);
  return {
    eventCount: entries.length,
    runCount: groups.filter((group) => group.runId !== null).length,
    groups,
  };
}

export function groupTimelineByRun(entries: readonly TimelineEntry[]): TimelineRunGroup[] {
  const groups: TimelineRunGroup[] = [];
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

export function timelineGroupKey(group: TimelineRunGroup, index: number): string {
  return `${group.runId ?? "pre"}:${index}`;
}

export function timelineSubtext({
  eventCount,
  runCount,
}: Pick<TimelineViewModel, "eventCount" | "runCount">): string {
  return `${eventCount} events · ${runCount} run${runCount === 1 ? "" : "s"}`;
}

export function timelineTimeOfDay(ts: number): string {
  const date = new Date(ts);
  const hh = String(date.getHours()).padStart(2, "0");
  const mm = String(date.getMinutes()).padStart(2, "0");
  const ss = String(date.getSeconds()).padStart(2, "0");
  return `${hh}:${mm}:${ss}`;
}
