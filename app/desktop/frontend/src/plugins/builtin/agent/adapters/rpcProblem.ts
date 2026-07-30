import { errorDetail, errorRetryAfterSeconds, errorType, RpcError } from "@/rpc";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";

export function agentProblemFromRpcError(error: unknown): AgentProblem | null {
  if (!(error instanceof RpcError)) return null;
  const retryAfterSeconds = errorRetryAfterSeconds(error.data);
  return {
    message: errorDetail(error.data),
    code: errorType(error.data),
    ...(retryAfterSeconds !== undefined ? { retryAfterSeconds } : {}),
  };
}
