// What the session's tools actually cost and how often they went wrong.
//
// The transcript answers "what happened, in order". It cannot answer "which
// tool is slow" or "which one keeps failing" — those are questions about the
// SHAPE of a session, and reading them off a scrolling log means counting by
// eye. Everything here is derived from tool calls the fold already holds: the
// runtime measures `durationMillis` per call, so nothing is timed in the client.

import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";

export interface ToolStat {
  /** Wire tool identity — the same name the icon and preview registries key on. */
  name: string;
  calls: number;
  failed: number;
  denied: number;
  /** Summed across the calls that reported one. A tool still running has no
   *  duration yet, and counting it as zero would drag the total down. */
  totalMs: number;
  /** The slowest single call, which is what a person waited on. */
  slowestMs: number;
  /** Every timed call's duration, in the order they ran — the only series this view
   *  has, and enough to show a tool that is getting slower. */
  durations: number[];
  /** Calls that reported a duration — the denominator behind the two above, and
   *  the reason a row can honestly show "—" instead of a made-up zero. */
  timed: number;
}

export interface ToolStatsSummary {
  rows: ToolStat[];
  calls: number;
  failed: number;
  denied: number;
  totalMs: number;
}

/**
 * One row per tool, heaviest first.
 *
 * Ordered by total time rather than call count: twenty greps that cost nothing
 * are not the reason a session took ten minutes, and the row a reader is looking
 * for is the one that spent the time. Ties fall back to call count so a set of
 * untimed tools still has a stable order.
 */
export function toolStats(calls: Record<string, ToolCall>): ToolStatsSummary {
  const byName = new Map<string, ToolStat>();

  for (const call of Object.values(calls)) {
    // A call still in flight has no outcome and no duration; counting it would
    // make the totals move backwards when it settles.
    if (call.status === "running" || call.status === "requires-action") continue;
    const row = byName.get(call.name) ?? {
      name: call.name,
      calls: 0,
      failed: 0,
      denied: 0,
      totalMs: 0,
      slowestMs: 0,
      durations: [],
      timed: 0,
    };
    row.calls += 1;
    // Counted apart: a denial is a person saying no, not the tool failing, and
    // lumping the two would make an approval policy read as a broken tool.
    if (call.status === "err") row.failed += 1;
    if (call.status === "denied") row.denied += 1;
    if (call.durationMillis !== undefined) {
      row.timed += 1;
      row.totalMs += call.durationMillis;
      row.slowestMs = Math.max(row.slowestMs, call.durationMillis);
      row.durations.push(call.durationMillis);
    }
    byName.set(call.name, row);
  }

  const rows = [...byName.values()].sort(
    (a, b) => b.totalMs - a.totalMs || b.calls - a.calls || a.name.localeCompare(b.name),
  );

  return {
    rows,
    calls: rows.reduce((total, row) => total + row.calls, 0),
    failed: rows.reduce((total, row) => total + row.failed, 0),
    denied: rows.reduce((total, row) => total + row.denied, 0),
    totalMs: rows.reduce((total, row) => total + row.totalMs, 0),
  };
}

/** A row's share of the session's tool time — the bar's length. Zero when
 *  nothing was timed, so an all-untimed session draws no bars rather than a
 *  row of full ones. */
export function toolTimeShare(row: ToolStat, summary: ToolStatsSummary): number {
  return summary.totalMs > 0 ? row.totalMs / summary.totalMs : 0;
}
