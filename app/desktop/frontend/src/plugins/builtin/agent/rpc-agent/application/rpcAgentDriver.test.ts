import type { AgentDriver } from "@/plugins/sdk";
import { asItemId, asRunId } from "@/rpc";
import { describe, expect, it, vi } from "vitest";
import {
  DEFAULT_RPC_SESSION_ID,
  activeRpcSessionId,
  createRpcAgentDriver,
  rpcRunStartParams,
  type RpcRunsGateway,
} from "./rpcAgentDriver";

const input: Parameters<AgentDriver["start"]>[0] = [{ type: "text", text: "hello" }];

describe("activeRpcSessionId", () => {
  it("falls back to the runtime default session id when no session is active", () => {
    expect(activeRpcSessionId(null)).toBe(DEFAULT_RPC_SESSION_ID);
    expect(activeRpcSessionId("")).toBe(DEFAULT_RPC_SESSION_ID);
    expect(activeRpcSessionId("ses_123")).toBe("ses_123");
  });
});

describe("rpcRunStartParams", () => {
  it("sends provider and model only as a complete pair", () => {
    expect(rpcRunStartParams("ses_1", input, { provider: "openai", model: "gpt" })).toEqual({
      sessionId: "ses_1",
      input,
      provider: "openai",
      model: "gpt",
    });
    expect(rpcRunStartParams("ses_1", input, { provider: "openai" })).toEqual({
      sessionId: "ses_1",
      input,
    });
    expect(rpcRunStartParams("ses_1", input, { model: "gpt" })).toEqual({
      sessionId: "ses_1",
      input,
    });
  });
});

describe("createRpcAgentDriver", () => {
  it("delegates start and resume through the gateway", async () => {
    const startResult = {} as Awaited<ReturnType<AgentDriver["start"]>>;
    const resumeResult = {} as Awaited<ReturnType<AgentDriver["resume"]>>;
    const gateway: RpcRunsGateway = {
      start: vi.fn().mockResolvedValue(startResult),
      resume: vi.fn().mockResolvedValue(resumeResult),
    };
    const signal = new AbortController().signal;
    const driver = createRpcAgentDriver("ses_1", () => gateway);

    await expect(driver.start(input, { provider: "openai" }, signal)).resolves.toBe(startResult);
    const parentRunId = asRunId("run_1");
    const responses: Parameters<AgentDriver["resume"]>[1] = [
      { itemId: asItemId("item_1"), response: { type: "approval", decision: "approve" } },
    ];
    await expect(driver.resume(parentRunId, responses, signal)).resolves.toBe(resumeResult);

    expect(gateway.start).toHaveBeenCalledWith({ sessionId: "ses_1", input }, signal);
    expect(gateway.resume).toHaveBeenCalledWith({ parentRunId, responses }, signal);
  });
});
