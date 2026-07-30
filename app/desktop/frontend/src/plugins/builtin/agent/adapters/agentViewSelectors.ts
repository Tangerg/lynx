import { useMemo } from "react";
import type {
  AgentProblem,
  AgentSessionView,
  Message,
  PlanItem,
  RunUsage,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import {
  selectCurrentRootMessages,
  selectCurrentRootPlan,
  selectCurrentRootRun,
  selectRunUsage,
  selectVisibleProblem,
} from "../application/view/runTree";
import { useAgentSessionStore } from "./agentSessionStore";
import { type AgentSendAction, type AgentStopAction, useAgentStore } from "./agentStore";

function useActiveAgentView<T>(select: (view: AgentSessionView) => T): T {
  const sessionId = useAgentSessionStore((state) => state.activeSessionId);
  return useAgentStore((state) =>
    select(state.sessions[sessionId]?.view ?? EMPTY_AGENT_SESSION_VIEW),
  );
}

function useCurrentRoot() {
  return useActiveAgentView(selectCurrentRootRun);
}

export function useAgentAction(kind: "stop"): AgentStopAction;
export function useAgentAction(kind: "send"): AgentSendAction;
export function useAgentAction(kind: "stop" | "send"): AgentStopAction | AgentSendAction {
  const sessionId = useAgentSessionStore((state) => state.activeSessionId);
  return useAgentStore((state) => state.sessions[sessionId]?.[kind] ?? null);
}

export function useCurrentRootRunning(): boolean {
  return useCurrentRoot()?.status === "running";
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

export function useCurrentRootMessages(): Message[] {
  const view = useActiveAgentView((current) => current);
  return useMemo(() => selectCurrentRootMessages(view), [view]);
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
