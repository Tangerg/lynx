import { useMemo, useRef } from "react";
import { navigator } from "@/lib/navigation";
import type {
  AgentProblem,
  AgentRunMetrics,
  AgentRunOutcome,
  AgentSessionView,
  Message,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import {
  selectCurrentRootRun,
  selectRootNarrativeMessages,
  selectRunTree,
  selectVisibleProblem,
} from "../application/view/runTree";
import type { AgentRootAttention, AgentRunTreeNode } from "../application/view/runTree";
import {
  buildTranscriptRows,
  EMPTY_TRANSCRIPT_ROW_CACHE,
  type TranscriptRow,
  type TranscriptRowCache,
} from "../application/conversation/transcriptRows";
import type {
  SendAgentInputAction,
  StopCurrentRootRunAction,
} from "../application/ports/sessionView";
import { useAgentStore } from "./agentStore";

function useActiveAgentView<T>(select: (view: AgentSessionView) => T): T {
  const sessionId = navigator().use((location) => location.session);
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
  const sessionId = navigator().use((location) => location.session);
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

export function useCurrentRootMetrics(): AgentRunMetrics | null {
  return useActiveAgentView((view) => selectCurrentRootRun(view)?.metrics ?? null);
}

export function useCurrentRootRunId(): string | null {
  return useCurrentRoot()?.id ?? null;
}

export function useAgentToolCalls(): Record<string, ToolCall> {
  return useActiveAgentView((view) => view.toolCalls);
}

export function useRootNarrativeMessages(): Message[] {
  const view = useActiveAgentView((current) => current);
  return useMemo(() => selectRootNarrativeMessages(view), [view]);
}

export function useTranscriptRows(): readonly TranscriptRow[] {
  const view = useActiveAgentView((current) => current);
  // The cache has to outlive the build that produced it — reusing the previous rows is
  // the entire mechanism, and a value closed over by `useMemo` is discarded the moment
  // its deps change. Writing a ref during render is safe here because the build is pure
  // in `(view, cache)`: a render React throws away leaves rows that are still valid for
  // the view they were built from, so the next build either reuses or replaces them.
  const cache = useRef<TranscriptRowCache>(EMPTY_TRANSCRIPT_ROW_CACHE);
  return useMemo(() => {
    const built = buildTranscriptRows(view, cache.current);
    cache.current = built.cache;
    return built.rows;
  }, [view]);
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
  const sessionId = navigator().get().session;
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
