import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { Item } from "@/rpc";

export type AgentRestoreType = "history" | "files" | "both";
export type AgentApprovalMode = "plan" | "safe" | "balanced" | "yolo";

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
    title?: string;
    favorite?: boolean;
    cwd?: string;
  }): Promise<void>;
  forkSession(input: { sessionId: string; fromRunId?: string }): Promise<{ id: string }>;
  loadSessionHistory(sessionId: string): Promise<AgentSessionHistory>;
  loadSessionUsage(sessionId: string): Promise<AgentSessionUsage>;
  rollbackSession(input: {
    sessionId: string;
    toRunId?: string;
    restoreType?: AgentRestoreType;
  }): Promise<void>;
  steerRun(runId: string, text: string): Promise<void>;
  isRunNotFound(error: unknown): boolean;
  setApprovalMode(mode: AgentApprovalMode): Promise<void>;
  forgetApprovalRule(id: string): Promise<void>;
}

const port = createSingletonPort<AgentRuntimeGateway>("Agent runtime gateway is not configured");

export const configureAgentRuntimeGateway = port.configure;
export const agentRuntime = port.get;
