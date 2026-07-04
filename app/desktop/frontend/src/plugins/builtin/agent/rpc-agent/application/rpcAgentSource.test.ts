import type { AgentDriver } from "@/plugins/sdk";
import { describe, expect, it, vi } from "vitest";
import { rpcAgentSource } from "./rpcAgentSource";
import type { RpcRunsGateway } from "./rpcAgentDriver";

const input: Parameters<AgentDriver["start"]>[0] = [{ type: "text", text: "hello" }];

describe("rpcAgentSource", () => {
  it("projects the RPC driver factory into a stable agent source spec", () => {
    const gateway: RpcRunsGateway = {
      start: vi.fn(),
      resume: vi.fn(),
    };

    const source = rpcAgentSource(
      (key) => `t:${key}`,
      () => "ses_1",
      () => gateway,
    );

    expect(source).toMatchObject({
      id: "rpc",
      label: "t:agentSource.rpc",
      priority: 1,
    });
  });

  it("builds drivers for the active session when the source factory runs", async () => {
    const result = {} as Awaited<ReturnType<AgentDriver["start"]>>;
    const gateway: RpcRunsGateway = {
      start: vi.fn().mockResolvedValue(result),
      resume: vi.fn(),
    };
    let activeSessionId = "ses_1";
    const source = rpcAgentSource(
      (key) => key,
      () => activeSessionId,
      () => gateway,
    );
    activeSessionId = "ses_2";

    await expect(source.factory().start(input, {}, undefined)).resolves.toBe(result);

    expect(gateway.start).toHaveBeenCalledWith({ sessionId: "ses_2", input }, undefined);
  });
});
