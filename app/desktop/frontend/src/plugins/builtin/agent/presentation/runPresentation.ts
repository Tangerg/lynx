import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";

export type AgentRunPresentationState =
  "running" | "waiting" | "finished" | "error" | "canceled" | "limit";

export function agentRunPresentationState(run: AgentRunView): AgentRunPresentationState {
  if (run.status !== "finished") return run.status;
  switch (run.outcome?.type) {
    case "error":
      return "error";
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
  switch (run.outcome?.type) {
    case "error":
      return run.outcome.error.message ?? null;
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
