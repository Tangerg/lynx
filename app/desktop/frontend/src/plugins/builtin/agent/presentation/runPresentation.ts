import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { isAgentRunFailure } from "../application/view/runOutcome";

export type AgentRunPresentationState =
  "running" | "waiting" | "finished" | "error" | "canceled" | "limit";

export function agentRunPresentationState(run: AgentRunView): AgentRunPresentationState {
  if (run.status !== "finished") return run.status;
  if (isAgentRunFailure(run.outcome)) return "error";
  switch (run.outcome?.type) {
    case "canceled":
      return "canceled";
    case "maxSteps":
    case "maxBudget":
      return "limit";
    case "completed":
    case undefined:
      return "finished";
  }
}

export function agentRunDetail(run: AgentRunView): string | null {
  if (run.status !== "finished") return run.progress?.activity ?? null;
  if (isAgentRunFailure(run.outcome)) return run.outcome.error.message ?? null;
  switch (run.outcome?.type) {
    case "canceled":
    case "maxSteps":
    case "maxBudget":
      return run.outcome.detail ?? null;
    case "completed":
    case undefined:
      return null;
  }
}

export function agentRunStepCount(run: AgentRunView): number {
  return run.progress?.step ?? run.metrics.steps;
}
