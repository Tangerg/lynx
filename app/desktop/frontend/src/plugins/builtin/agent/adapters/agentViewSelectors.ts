import type {
  AgentViewState,
  Message,
  PlanItem,
  RunError,
  RunUsage,
  TimelineEntry,
  ToolCall,
} from "@/plugins/builtin/agent/public/viewState";
import { INITIAL_VIEW_STATE } from "@/plugins/builtin/agent/public/viewState";
import { useAgentSessionStore } from "./agentSessionStore";
import { type AgentSendAction, type AgentStopAction, useAgentStore } from "./agentStore";

function useActiveAgentView<T>(select: (view: AgentViewState) => T): T {
  const sessionId = useAgentSessionStore((state) => state.activeSessionId);
  // Keep this fallback as the shared module constant. Inline [] / {} fallbacks
  // create a fresh snapshot every render and can loop Zustand subscribers.
  return useAgentStore((state) => select(state.sessions[sessionId]?.view ?? INITIAL_VIEW_STATE));
}

export function useAgentAction(kind: "stop"): AgentStopAction;
export function useAgentAction(kind: "send"): AgentSendAction;
export function useAgentAction(kind: "stop" | "send"): AgentStopAction | AgentSendAction {
  const sessionId = useAgentSessionStore((state) => state.activeSessionId);
  return useAgentStore((state) => state.sessions[sessionId]?.[kind] ?? null);
}

export function useAgentRunning(): boolean {
  return useActiveAgentView((view) => view.run.running);
}

export function useAgentRunId(): string | null {
  return useActiveAgentView((view) => view.run.runId);
}

export function useAgentRunUsage(): RunUsage {
  return useActiveAgentView((view) => view.run.usage);
}

export function useAgentRunContextTokens(): number | undefined {
  return useActiveAgentView((view) => view.run.contextTokens);
}

export function useAgentPlan(): PlanItem[] {
  return useActiveAgentView((view) => view.plan);
}

export function useAgentToolCalls(): Record<string, ToolCall> {
  return useActiveAgentView((view) => view.toolCalls);
}

export function useAgentMessages(): Message[] {
  return useActiveAgentView((view) => view.messages);
}

export function useAgentTimeline(): TimelineEntry[] {
  return useActiveAgentView((view) => view.timeline);
}

export function useAgentError(): RunError | null {
  return useActiveAgentView((view) => view.error);
}

export function useAgentSharedState<T = unknown>(path?: string): T | undefined {
  return useActiveAgentView((view) => selectFromShared<T>(view.shared, path));
}

export function getCurrentSessionView(): AgentViewState {
  const sessionId = useAgentSessionStore.getState().activeSessionId;
  return useAgentStore.getState().sessions[sessionId]?.view ?? INITIAL_VIEW_STATE;
}

// Walk dot-segments into the backend-owned shared document. Returns stable
// references off the store (never a fresh object), so it is safe as a Zustand
// selector result.
function selectFromShared<T>(
  shared: Record<string, unknown>,
  path: string | undefined,
): T | undefined {
  let current: unknown = shared;
  if (!path) return current as T;
  for (const segment of path.split(".")) {
    if (current == null || typeof current !== "object") return undefined;
    current = (current as Record<string, unknown>)[segment];
  }
  return current as T;
}
