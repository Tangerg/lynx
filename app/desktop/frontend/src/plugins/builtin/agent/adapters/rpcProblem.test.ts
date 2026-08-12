import { describe, expect, it } from "vitest";
import { RpcError, RpcTransportError } from "@/rpc";
import { agentProblemFromRpcFailure } from "./rpcProblem";

describe("agent RPC problem projection", () => {
  it("projects transport internals to one stable product problem", () => {
    expect(agentProblemFromRpcFailure(new RpcTransportError("fetch failed: socket reset"))).toEqual(
      {
        code: "transport_error",
      },
    );
  });

  it("retains typed protocol detail and retry timing", () => {
    expect(
      agentProblemFromRpcFailure(
        new RpcError({
          code: -32002,
          message: "rate limited",
          data: { type: "rate_limited", detail: "try later", retryAfterSeconds: 3 },
        }),
      ),
    ).toEqual({ code: "rate_limited", message: "try later", retryAfterSeconds: 3 });
  });

  it("does not turn programming errors into user-facing command failures", () => {
    expect(agentProblemFromRpcFailure(new TypeError("broken adapter"))).toBeNull();
  });
});
