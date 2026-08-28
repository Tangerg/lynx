import { useMemo } from "react";
import type {
  AgentProblem,
  AgentRunMetrics,
  AgentModelSelection,
  AgentRunOutcome,
  AgentRunView,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import type { AgentRootAttention, AgentRunTreeNode } from "../view/runTree";
import type { TranscriptRow } from "../conversation/transcriptRows";
import { isAgentRunFailure } from "../view/runOutcome";

/**
 * The active Session's exact root Run read model.
 *
 * Attention, streaming admission, outcome and metrics are facets of one Run
 * identity, not independently sampled global facts. Keeping them together also
 * gives transcript chrome one place to ask which exact turn owns terminal
 * material while an optimistic successor message is still unassigned.
 */
export class CurrentRootMaterial {
  static readonly idle = new CurrentRootMaterial(null);

  readonly runId: string | null;
  readonly status: AgentRunView["status"] | "idle";
  readonly outcome: AgentRunOutcome | null;
  readonly metrics: AgentRunMetrics | null;
  /** Latest model prompt footprint for this Run. Unlike cumulative usage, this
   * is the number that occupies the model's context window. */
  readonly contextTokens: number | null;
  readonly modelSelection: AgentModelSelection | null;
  readonly attention: AgentRootAttention;

  private constructor(run: AgentRunView | null) {
    this.runId = run?.id ?? null;
    this.status = run?.status ?? "idle";
    this.outcome = run?.outcome ?? null;
    this.metrics = run?.metrics ?? null;
    this.contextTokens = run?.progress?.contextTokens ?? null;
    this.modelSelection = run?.modelSelection ?? null;
    this.attention = Object.freeze(
      run ? { status: run.status, runId: run.id } : { status: "idle", runId: null },
    );
    Object.freeze(this);
  }

  static from(run: AgentRunView | null): CurrentRootMaterial {
    return run ? new CurrentRootMaterial(run) : CurrentRootMaterial.idle;
  }

  get running(): boolean {
    return this.status === "running";
  }

  /** The last narrative row owned by this finished Run is the only row that may
   * host its close material. An unassigned optimistic successor can never steal
   * that ownership merely by becoming the transcript tail. */
  terminalTurnIndex(rows: readonly TranscriptRow[]): number {
    if (
      this.status !== "finished" ||
      this.runId === null ||
      this.outcome === null ||
      isAgentRunFailure(this.outcome)
    ) {
      return -1;
    }
    for (let index = rows.length - 1; index >= 0; index -= 1) {
      const owner = rows[index]!.runOwner;
      if (owner.kind === "owned" && owner.runId === this.runId) return index;
    }
    return -1;
  }
}

export function useCurrentRootMaterial(): CurrentRootMaterial {
  const run = agentSessionView().useCurrentRootRun();
  return useMemo(() => CurrentRootMaterial.from(run), [run]);
}

export function useIsCurrentRootRunning(): boolean {
  return useCurrentRootMaterial().running;
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
