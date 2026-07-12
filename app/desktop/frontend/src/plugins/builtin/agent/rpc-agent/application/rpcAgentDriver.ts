import type { AgentDriver } from "@/plugins/sdk";

export const DEFAULT_RPC_SESSION_ID = "ses_default";

export type RpcAgentInput = Parameters<AgentDriver["start"]>[0];
export type RpcAgentStartOptions = Parameters<AgentDriver["start"]>[1];
export type RpcAgentRunId = Parameters<AgentDriver["resume"]>[0];
export type RpcAgentInterruptResponses = Parameters<AgentDriver["resume"]>[1];

export interface RpcRunStartParams {
  sessionId: string;
  input: RpcAgentInput;
  provider?: string;
  model?: string;
}

export interface RpcRunResumeParams {
  runId: RpcAgentRunId;
  responses: RpcAgentInterruptResponses;
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
  const { provider, model } = options;
  return {
    sessionId,
    input,
    ...(provider && model ? { provider, model } : {}),
  };
}

export function createRpcAgentDriver(
  sessionId: string,
  gateway: () => RpcRunsGateway,
): AgentDriver {
  return {
    start: (input, options, signal) =>
      gateway().start(rpcRunStartParams(sessionId, input, options), signal),
    resume: (runId, responses, signal) => gateway().resume({ runId, responses }, signal),
  };
}
