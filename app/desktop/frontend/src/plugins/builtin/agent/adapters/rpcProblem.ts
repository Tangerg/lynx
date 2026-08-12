import { errorDetail, errorRetryAfterSeconds, errorType, RpcError, RpcTransportError } from "@/rpc";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";

export function agentProblemFromRpcFailure(error: unknown): AgentProblem | null {
  // Transport details belong to the RPC Adapter. Publish only one stable
  // product problem symbol; the banner owns its localized fallback copy.
  if (error instanceof RpcTransportError) return { code: "transport_error" };
  if (!(error instanceof RpcError)) return null;
  const retryAfterSeconds = errorRetryAfterSeconds(error.data);
  return {
    message: errorDetail(error.data),
    code: errorType(error.data),
    ...(retryAfterSeconds !== undefined ? { retryAfterSeconds } : {}),
  };
}
