// The reinit decorator: a call that fails with capability_not_negotiated
// (a lost/restarted backend session) must re-handshake and retry ONCE — the
// auto-recovery that makes a backend restart transparent.

import { describe, expect, it, vi } from "vitest";
import type { RpcClient } from "./client";
import { RpcError } from "./errors";
import { withReinit } from "./sdk";

function capabilityError(): RpcError {
  return new RpcError({
    code: -32002,
    message: "capability_not_negotiated",
    data: { type: "capability_not_negotiated" },
  });
}

/** A stub RpcClient whose `call` runs the supplied script entry per invocation. */
function stubRpc(script: Array<() => Promise<unknown>>): RpcClient {
  let i = 0;
  return {
    call: vi.fn(() => script[Math.min(i++, script.length - 1)]!()),
    notify: vi.fn(),
    subscribe: vi.fn(),
    close: vi.fn(),
  } as unknown as RpcClient;
}

describe("withReinit", () => {
  it("re-handshakes and retries once on capability_not_negotiated", async () => {
    const rpc = stubRpc([
      () => Promise.reject(capabilityError()), // first attempt: lost session
      () => Promise.resolve("ok"), // retry after reinit: succeeds
    ]);
    const reinit = vi.fn().mockResolvedValue(undefined);

    const result = await withReinit(rpc, reinit).call("workspace.mcp.configure", {});

    expect(reinit).toHaveBeenCalledOnce();
    expect(result).toBe("ok");
    expect(rpc.call).toHaveBeenCalledTimes(2);
  });

  it("never reinits runtime.initialize (it IS the recovery — would loop)", async () => {
    const rpc = stubRpc([() => Promise.reject(capabilityError())]);
    const reinit = vi.fn().mockResolvedValue(undefined);

    await expect(withReinit(rpc, reinit).call("runtime.initialize", {})).rejects.toThrow(
      "capability_not_negotiated",
    );
    expect(reinit).not.toHaveBeenCalled();
    expect(rpc.call).toHaveBeenCalledOnce();
  });

  it("passes other errors through without reinit", async () => {
    const other = new RpcError({
      code: -32001,
      message: "session_busy",
      data: { type: "session_busy" },
    });
    const rpc = stubRpc([() => Promise.reject(other)]);
    const reinit = vi.fn().mockResolvedValue(undefined);

    await expect(withReinit(rpc, reinit).call("runs.start", {})).rejects.toThrow("session_busy");
    expect(reinit).not.toHaveBeenCalled();
    expect(rpc.call).toHaveBeenCalledOnce();
  });

  it("does not retry a second time if the call still fails after reinit", async () => {
    const rpc = stubRpc([
      () => Promise.reject(capabilityError()),
      () => Promise.reject(capabilityError()), // still failing after reinit → real error
    ]);
    const reinit = vi.fn().mockResolvedValue(undefined);

    await expect(withReinit(rpc, reinit).call("sessions.list", {})).rejects.toThrow(
      "capability_not_negotiated",
    );
    expect(reinit).toHaveBeenCalledOnce();
    expect(rpc.call).toHaveBeenCalledTimes(2);
  });
});
