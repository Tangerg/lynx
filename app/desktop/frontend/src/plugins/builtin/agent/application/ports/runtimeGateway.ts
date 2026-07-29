import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { Item } from "@/rpc";
import type { ApprovalMode } from "../../domain/hitl";

export type RestoreType = "history" | "files" | "both";

export interface AgentRunHistoryRef {
  id: string;
  spawnedByItemId?: string;
}

export interface AgentSessionHistory {
  items: Item[];
  runs: AgentRunHistoryRef[];
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
  createSession(input: { cwd?: string }, signal?: AbortSignal): Promise<{ id: string }>;
  deleteSession(sessionId: string): Promise<void>;
  updateSession(input: {
    sessionId: string;
    expectedRevision: number;
    title?: string;
    favorite?: boolean;
    cwd?: string;
  }): Promise<{ revision: number }>;
  forkSession(input: { sessionId: string; fromRunId?: string }): Promise<{ id: string }>;
  loadSessionHistory(sessionId: string): Promise<AgentSessionHistory>;
  /** Does this session hold any transcript item at all? One row is enough to
   *  answer, so this asks for one rather than reading a history. */
  sessionHoldsNothing(sessionId: string): Promise<boolean>;
  loadSessionUsage(sessionId: string): Promise<AgentSessionUsage>;
  rollbackSession(input: {
    sessionId: string;
    toRunId?: string;
    restoreType?: RestoreType;
  }): Promise<void>;
  /** Inject a user instruction into the segment the caller believes is executing.
   *  The segment is part of the address: a run that parked and resumed between
   *  typing and sending must refuse rather than deliver the instruction into a
   *  continuation the person never saw. */
  steerRun(runId: string, segmentId: string, text: string): Promise<void>;
  /** Whether a refusal means "the run this addressed is no longer executing" —
   *  finished, waiting on a person, or already on a different segment. One
   *  question because one answer follows: send the text as a fresh turn. */
  isRunGone(error: unknown): boolean;
  setApprovalMode(mode: ApprovalMode): Promise<void>;
  forgetApprovalRule(id: string): Promise<void>;
}

const port = createSingletonPort<AgentRuntimeGateway>("Agent runtime gateway is not configured");

export const configureAgentRuntimeGateway = port.configure;
export const agentRuntime = port.get;
