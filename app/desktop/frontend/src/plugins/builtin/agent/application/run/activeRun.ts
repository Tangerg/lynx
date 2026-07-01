import type {
  PlanItem,
  RunError,
  RunUsage,
  TimelineEntry,
  ToolCall,
} from "@/protocol/run/viewState";
import {
  useAgentAction,
  useAgentPlan,
  useAgentRunContextTokens,
  useAgentRunId,
  useAgentRunning,
  useAgentRunUsage,
  useAgentSlice,
  useAgentStore,
  useAgentTimeline,
  useAgentToolCalls,
} from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

interface ActiveRunTokenUsage {
  usage: RunUsage;
  contextTokens: number | undefined;
}

export function useIsAgentRunning(): boolean {
  return useAgentRunning();
}

export function useActiveRunId(): string | null {
  return useAgentRunId();
}

export function useActiveRunPlan(): PlanItem[] {
  return useAgentPlan();
}

export function useActiveRunToolCalls(): Record<string, ToolCall> {
  return useAgentToolCalls();
}

export function useActiveRunTimeline(): TimelineEntry[] {
  return useAgentTimeline();
}

export function useActiveRunError(): RunError | null {
  return useAgentSlice((view) => view.error);
}

export function useActiveRunTokenUsage(): ActiveRunTokenUsage {
  return {
    usage: useAgentRunUsage(),
    contextTokens: useAgentRunContextTokens(),
  };
}

export function useStopActiveAgentRun(): (() => void) | null {
  return useAgentAction("stop");
}

export function stopActiveAgentRun(): boolean {
  const sessionId = useSessionStore.getState().activeSessionId;
  const entry = useAgentStore.getState().sessions[sessionId];
  if (!entry?.view.run.running) return false;
  entry.stop?.();
  return true;
}

export function clearActiveRunError(): void {
  const sessionId = useSessionStore.getState().activeSessionId;
  if (!sessionId) return;
  useAgentStore.getState().clearError(sessionId);
}
