import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { Item, PendingInterruptSet, RunRef } from "@/rpc";
import type { ApprovalMode } from "../../domain/hitl";
import type { AgentInput } from "../../domain/input";
import type { AgentPlanStateSnapshot } from "../../domain/plan";

export type RestoreType = "history" | "files" | "both";

export interface AgentSessionSnapshot {
  items: Item[];
  runs: RunRef[];
  pendingInterruptSets: PendingInterruptSet[];
  state?: AgentPlanStateSnapshot;
}

export interface AgentSessionUsage {
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
}

export interface AgentRuntimeGateway {
  createSession(input: { cwd?: string }): Promise<{ id: string }>;
  /** Resolve once the Session is authoritatively absent. Already absent is success. */
  deleteSession(sessionId: string): Promise<void>;
  updateSession(input: {
    sessionId: string;
    expectedRevision: number;
    title?: string;
    favorite?: boolean;
    cwd?: string;
  }): Promise<{ revision: number }>;
  forkSession(input: { sessionId: string; fromRunId?: string }): Promise<{ id: string }>;
  /**
   * Read every durable fact needed to rebuild the Agent projection. The adapter
   * owns capability-aware query scope; callers always receive one canonical
   * snapshot shape and commit it atomically. Null means the Runtime
   * authoritatively reports that the Session no longer exists; transport and
   * other operational failures still reject.
   */
  loadSessionSnapshot(sessionId: string): Promise<AgentSessionSnapshot | null>;
  loadSessionUsage(sessionId: string): Promise<AgentSessionUsage>;
  rollbackSession(input: {
    sessionId: string;
    toRunId?: string;
    restoreType?: RestoreType;
  }): Promise<{
    droppedRuns: Array<{ runId: string; userInput?: AgentInput }>;
  }>;
  /** Inject a user instruction into the segment the caller believes is executing.
   *  The segment is part of the address: a run that parked and resumed between
   *  typing and sending must refuse rather than deliver the instruction into a
   *  continuation the person never saw. */
  steerRun(runId: string, segmentId: string, input: AgentInput): Promise<void>;
  /** Whether a refusal means "the run this addressed is no longer executing" —
   *  finished, waiting on a person, or already on a different segment. One
   *  question because one answer follows: send the input as a fresh turn. */
  isRunGone(error: unknown): boolean;
  /** Whether a refusal means "the replay window no longer reaches that cursor". The
   *  events are gone for good; the items they produced are not, so the answer is a
   *  cold history read plus a tail attach — not a retry of the same cursor. */
  isReplayLost(error: unknown): boolean;
  setApprovalMode(mode: ApprovalMode): Promise<void>;
  forgetApprovalRule(id: string): Promise<void>;
}

const port = createSingletonPort<AgentRuntimeGateway>("Agent runtime gateway is not configured");

export const configureAgentRuntimeGateway = port.configure;
export const agentRuntime = port.get;
