import { fmtCost, fmtDuration, fmtTokens } from "@/lib/format";
import type { AgentRunMetrics } from "@/plugins/builtin/agent/public/viewState";

/**
 * What a finished turn cost, ready to set.
 *
 * Every field is optional and absent rather than zero: a Run restored across a
 * restart carries empty metrics, and a close row reading "0s · 0 steps · ↑0" is a
 * worse end to a turn than no row at all. The whole readout is `null` when there is
 * nothing true to say.
 */
export interface RunCloseReadout {
  duration?: string;
  steps?: number;
  inputTokens?: string;
  outputTokens?: string;
  cost?: string;
}

export function runCloseReadout(metrics: AgentRunMetrics | null): RunCloseReadout | null {
  if (!metrics) return null;

  const readout: RunCloseReadout = {};
  if (metrics.activeDurationMillis > 0)
    readout.duration = fmtDuration(metrics.activeDurationMillis);
  if (metrics.steps > 0) readout.steps = metrics.steps;
  if (metrics.usage.inputTokens > 0) readout.inputTokens = fmtTokens(metrics.usage.inputTokens);
  if (metrics.usage.outputTokens > 0) readout.outputTokens = fmtTokens(metrics.usage.outputTokens);
  const cost = metrics.usage.costUsd;
  if (cost !== undefined && cost > 0) readout.cost = fmtCost(cost);

  return Object.keys(readout).length > 0 ? readout : null;
}
