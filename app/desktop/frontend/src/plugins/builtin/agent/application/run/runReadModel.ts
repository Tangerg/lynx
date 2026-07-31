import type {
  AgentProblem,
  AgentRunOutcome,
  PlanItem,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import type { AgentRunTreeNode } from "../view/runTree";

export function useIsCurrentRootRunning(): boolean {
  return agentSessionView().useCurrentRootAttention().status === "running";
}

export function useCurrentRootRunId(): string | null {
  return agentSessionView().useCurrentRootRunId();
}

export function useCurrentRootOutcome(): AgentRunOutcome | null {
  return agentSessionView().useCurrentRootOutcome();
}

export function useCurrentRootPlan(): PlanItem[] {
  return agentSessionView().useCurrentRootPlan();
}

export function useActiveSessionToolCalls(): Record<string, ToolCall> {
  return agentSessionView().useToolCalls();
}

export function useActiveSessionTimeline(): TimelineEntry[] {
  return agentSessionView().useSessionTimeline();
}

export function useActiveSessionRunTree(): AgentRunTreeNode[] {
  return agentSessionView().useRunTree();
}

export function useActiveSessionProblem(): AgentProblem | null {
  return agentSessionView().useProblem();
}
