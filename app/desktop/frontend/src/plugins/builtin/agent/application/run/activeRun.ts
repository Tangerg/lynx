import type {
  PlanItem,
  RunError,
  RunUsage,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentView";
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
} from "@/plugins/builtin/agent/adapters/agentStore";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";

interface AgentSessionEntry {
  view: {
    run: { running: boolean };
    openInterrupts: unknown[];
    error: RunError | null;
  };
}

interface AgentRunSettlement {
  sessionId: string;
  needsInput: boolean;
  errorMessage: string | null;
}

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
  const sessionId = useAgentSessionStore.getState().activeSessionId;
  const entry = useAgentStore.getState().sessions[sessionId];
  if (!entry?.view.run.running) return false;
  entry.stop?.();
  return true;
}

export function clearActiveRunError(): void {
  const sessionId = useAgentSessionStore.getState().activeSessionId;
  if (!sessionId) return;
  useAgentStore.getState().clearError(sessionId);
}

function anyAgentRunning(sessions: Record<string, AgentSessionEntry>): boolean {
  for (const id in sessions) {
    if (sessions[id]!.view.run.running) return true;
  }
  return false;
}

export function subscribeAnyAgentRunning(onChange: (running: boolean) => void): () => void {
  let lastRunning = anyAgentRunning(useAgentStore.getState().sessions);
  return useAgentStore.subscribe((state) => {
    const running = anyAgentRunning(state.sessions);
    if (running === lastRunning) return;
    lastRunning = running;
    onChange(running);
  });
}

export function subscribeAgentRunSettlements(
  onSettled: (settlement: AgentRunSettlement) => void,
): () => void {
  const lastRunning = new Map<string, boolean>();
  return useAgentStore.subscribe((state) => {
    const { sessions } = state;
    let count = 0;
    for (const sessionId in sessions) {
      count++;
      const view = sessions[sessionId]!.view;
      const running = view.run.running;
      const was = lastRunning.get(sessionId) ?? false;
      if (was === running) continue;
      lastRunning.set(sessionId, running);
      if (was && !running) {
        onSettled({
          sessionId,
          needsInput: view.openInterrupts.length > 0,
          errorMessage: view.error?.message ?? null,
        });
      }
    }
    if (lastRunning.size > count) {
      for (const sessionId of [...lastRunning.keys()]) {
        if (!(sessionId in sessions)) lastRunning.delete(sessionId);
      }
    }
  });
}
