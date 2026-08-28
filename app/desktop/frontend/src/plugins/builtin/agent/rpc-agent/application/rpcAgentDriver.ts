import type { AgentDriver } from "@/plugins/sdk";

export const DEFAULT_RPC_SESSION_ID = "ses_default";

export type RpcAgentInput = Parameters<AgentDriver["start"]>[0];
export type RpcAgentStartOptions = Parameters<AgentDriver["start"]>[1];
export type RpcAgentRunId = Parameters<AgentDriver["resume"]>[0];
export type RpcAgentResumeOptions = Parameters<AgentDriver["resume"]>[1];

export interface RpcRunStartParams {
  sessionId: string;
  input: RpcAgentInput;
  provider?: string;
  model?: string;
  reasoningEffort?: string;
}

export interface RpcRunResumeParams {
  runId: RpcAgentRunId;
  responses: RpcAgentResumeOptions["responses"];
  input?: RpcAgentResumeOptions["input"];
}

export interface RpcRunsGateway {
  start: (params: RpcRunStartParams, signal?: AbortSignal) => ReturnType<AgentDriver["start"]>;
  resume: (params: RpcRunResumeParams, signal?: AbortSignal) => ReturnType<AgentDriver["resume"]>;
}

export function activeRpcSessionId(sessionId: string | null | undefined): string {
  return sessionId || DEFAULT_RPC_SESSION_ID;
}

export function rpcRunStartParams(
  sessionId: string,
  input: RpcAgentInput,
  options: RpcAgentStartOptions,
): RpcRunStartParams {
  const { provider, model, reasoningEffort } = options;
  return {
    sessionId,
    input,
    ...(provider && model
      ? { provider, model, ...(reasoningEffort ? { reasoningEffort } : {}) }
      : {}),
  };
}

export function createRpcAgentDriver(
  sessionId: string,
  gateway: () => RpcRunsGateway,
): AgentDriver {
  return {
    start: (input, options, signal) =>
      gateway().start(rpcRunStartParams(sessionId, input, options), signal),
    resume: (runId, options, signal) => gateway().resume({ runId, ...options }, signal),
  };
}
