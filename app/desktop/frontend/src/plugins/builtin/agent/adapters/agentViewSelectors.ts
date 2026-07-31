import { useMemo } from "react";
import type {
  AgentProblem,
  AgentRunOutcome,
  AgentSessionView,
  Message,
  PlanItem,
  RunUsage,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import {
  selectCurrentRootPlan,
  selectCurrentRootRun,
  selectDelegatedRunNarratives,
  selectRootNarrativeMessages,
  selectRunTree,
  selectRunUsage,
  selectVisibleProblem,
} from "../application/view/runTree";
import type {
  AgentRootAttention,
  AgentRunTreeNode,
  DelegatedRunNarrativesByItemId,
} from "../application/view/runTree";
import type {
  SendAgentInputAction,
  StopCurrentRootRunAction,
} from "../application/ports/sessionView";
import { useAgentSessionStore } from "./agentSessionStore";
import { useAgentStore } from "./agentStore";

function useActiveAgentView<T>(select: (view: AgentSessionView) => T): T {
  const sessionId = useAgentSessionStore((state) => state.activeSessionId);
  return useAgentStore((state) =>
    select(state.sessions[sessionId]?.view ?? EMPTY_AGENT_SESSION_VIEW),
  );
}

function useCurrentRoot() {
  return useActiveAgentView(selectCurrentRootRun);
}

export function useAgentAction(kind: "stop"): StopCurrentRootRunAction | null;
export function useAgentAction(kind: "send"): SendAgentInputAction | null;
export function useAgentAction(
  kind: "stop" | "send",
): StopCurrentRootRunAction | SendAgentInputAction | null {
  const sessionId = useAgentSessionStore((state) => state.activeSessionId);
  return useAgentStore((state) => state.sessions[sessionId]?.[kind] ?? null);
}

export function useCurrentRootAttention(): AgentRootAttention {
  const root = useCurrentRoot();
  return useMemo(
    () => (root ? { status: root.status, runId: root.id } : { status: "idle", runId: null }),
    [root],
  );
}

export function useCurrentRootOutcome(): AgentRunOutcome | null {
  return useActiveAgentView((view) => selectCurrentRootRun(view)?.outcome ?? null);
}

export function useCurrentRootRunId(): string | null {
  return useCurrentRoot()?.id ?? null;
}

export function useCurrentRootSegmentId(): string | null {
  return useCurrentRoot()?.activeSegmentId ?? null;
}

export function useCurrentRootUsage(): RunUsage {
  return selectRunUsage(useCurrentRoot());
}

export function useCurrentRootContextTokens(): number | undefined {
  return useCurrentRoot()?.progress?.contextTokens;
}

export function useCurrentRootPlan(): PlanItem[] {
  const view = useActiveAgentView((current) => current);
  return useMemo(() => selectCurrentRootPlan(view), [view]);
}

export function useAgentToolCalls(): Record<string, ToolCall> {
  return useActiveAgentView((view) => view.toolCalls);
}

export function useRootNarrativeMessages(): Message[] {
  const view = useActiveAgentView((current) => current);
  return useMemo(() => selectRootNarrativeMessages(view), [view]);
}

export function useDelegatedRunNarratives(): DelegatedRunNarrativesByItemId {
  const view = useActiveAgentView((current) => current);
  return useMemo(() => selectDelegatedRunNarratives(view), [view]);
}

export function useRunTree(): AgentRunTreeNode[] {
  const view = useActiveAgentView((current) => current);
  return useMemo(() => selectRunTree(view), [view]);
}

export function useAgentSessionTimeline(): TimelineEntry[] {
  return useActiveAgentView((view) => view.timeline);
}

export function useAgentProblem(): AgentProblem | null {
  return useActiveAgentView(selectVisibleProblem);
}

export function useAgentSharedState<T = unknown>(path?: string): T | undefined {
  return useActiveAgentView((view) => selectFromShared<T>(view.shared, path));
}

export function getCurrentSessionView(): AgentSessionView {
  const sessionId = useAgentSessionStore.getState().activeSessionId;
  return useAgentStore.getState().sessions[sessionId]?.view ?? EMPTY_AGENT_SESSION_VIEW;
}

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
