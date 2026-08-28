import { useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { navigator } from "@/lib/navigation";
import type {
  AgentPlan,
  AgentProblem,
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
import type { AgentRunTreeNode } from "../application/view/runTree";
import {
  buildTranscriptRows,
  EMPTY_TRANSCRIPT_ROW_CACHE,
  type TranscriptRow,
  type TranscriptRowCache,
} from "../application/conversation/transcriptRows";
import type {
  AgentProjectionMaterial,
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

type AgentStoreState = ReturnType<typeof useAgentStore.getState>;

class TranscriptRowsProjection {
  private cache: TranscriptRowCache = EMPTY_TRANSCRIPT_ROW_CACHE;
  private rows: readonly TranscriptRow[] = [];

  constructor(private readonly sessionId: string) {}

  select(state: AgentStoreState): readonly TranscriptRow[] {
    const view = state.sessions[this.sessionId]?.view ?? EMPTY_AGENT_SESSION_VIEW;
    const built = buildTranscriptRows(view, this.cache);
    this.cache = built.cache;
    if (
      built.rows.length === this.rows.length &&
      built.rows.every((row, index) => row === this.rows[index])
    ) {
      return this.rows;
    }
    this.rows = built.rows;
    return this.rows;
  }
}

export function useCurrentRootRun() {
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

export function useAgentToolCalls(): Record<string, ToolCall> {
  return useActiveAgentView((view) => view.toolCalls);
}

export function useRootNarrativeMessages(): Message[] {
  const view = useActiveAgentView((current) => current);
  return useMemo(() => selectRootNarrativeMessages(view), [view]);
}

export function useTranscriptRows(): readonly TranscriptRow[] {
  const sessionId = navigator().use((location) => location.session);
  const selectRows = useMemo(() => {
    const projection = new TranscriptRowsProjection(sessionId);
    return (state: AgentStoreState): readonly TranscriptRow[] => projection.select(state);
  }, [sessionId]);
  return useAgentStore(selectRows);
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

export function useAgentPlan(): AgentProjectionMaterial<AgentPlan> {
  const sessionId = navigator().use((location) => location.session);
  return useAgentStore(
    useShallow((state) => {
      const entry = state.sessions[sessionId];
      return {
        generation: entry?.viewEpoch ?? 0,
        value: entry?.view.plan ?? undefined,
      };
    }),
  );
}

export function useAgentSharedMaterial<T = unknown>(path?: string): AgentProjectionMaterial<T> {
  const sessionId = navigator().use((location) => location.session);
  return useAgentStore(
    useShallow((state) => {
      const entry = state.sessions[sessionId];
      return {
        generation: entry?.viewEpoch ?? 0,
        value: selectFromShared<T>(entry?.view.shared ?? EMPTY_AGENT_SESSION_VIEW.shared, path),
      };
    }),
  );
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
