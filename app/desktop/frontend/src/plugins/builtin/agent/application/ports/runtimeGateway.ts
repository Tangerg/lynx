import type { Item } from "@/rpc";

export type AgentRestoreType = "history" | "files" | "both";
export type AgentApprovalMode = "plan" | "safe" | "balanced" | "yolo";

export interface AgentRunHistoryRef {
  id: string;
  parentRunId?: string;
  spawnedByItemId?: string;
}

export interface AgentSessionHistory {
  items: Item[];
  runs: AgentRunHistoryRef[];
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

let port: AgentRuntimeGateway | null = null;

export function configureAgentRuntimeGateway(next: AgentRuntimeGateway): void {
  port = next;
}

export function agentRuntime(): AgentRuntimeGateway {
  if (!port) throw new Error("Agent runtime gateway is not configured");
  return port;
}
