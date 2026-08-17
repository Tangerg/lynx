import type {
  AgentProblem,
  AgentRunMetrics,
  AgentRunOutcome,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import type { AgentRootAttention, AgentRunTreeNode } from "../view/runTree";

export function useCurrentRootAttention(): AgentRootAttention {
  return agentSessionView().useCurrentRootAttention();
}

export function useIsCurrentRootRunning(): boolean {
  return useCurrentRootAttention().status === "running";
}

export function useCurrentRootOutcome(): AgentRunOutcome | null {
  return agentSessionView().useCurrentRootOutcome();
}

export function useCurrentRootMetrics(): AgentRunMetrics | null {
  return agentSessionView().useCurrentRootMetrics();
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
