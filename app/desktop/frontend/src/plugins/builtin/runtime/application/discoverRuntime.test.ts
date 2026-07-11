import { describe, expect, it, vi } from "vitest";
import type { DiscoverResponse, RpcClient, ServerCapabilities } from "@/rpc";
import { discoverRuntime, type RuntimeCapabilitySink } from "./discoverRuntime";

const discovery: DiscoverResponse = {
  protocolVersion: "2026-06-07",
  serverInfo: { name: "lyra-runtime", version: "1.0.0", cwd: "/work", home: "/home" },
  capabilities: {
    protocolVersion: "2026-06-07",
    events: [],
    streamingMethods: [],
    features: {
      reasoning: false,
      mcp: false,
      multimodal: false,
      git: false,
      fileWatch: false,
      checkpoints: false,
      lsp: false,
      subagents: false,
      skills: false,
      sessionExport: false,
      memory: false,
      relocate: false,
      clientTools: false,
    },
    providers: [],
    limits: {},
  },
};

function fakeRpc(call: RpcClient["call"]): RpcClient {
  return {
    call,
    notify: vi.fn(),
    subscribe: vi.fn(),
    close: vi.fn(),
  } as unknown as RpcClient;
}

function capabilitySink(): RuntimeCapabilitySink & { replace: ReturnType<typeof vi.fn> } {
  return { replace: vi.fn<(capabilities: ServerCapabilities) => void>() };
}

describe("discoverRuntime", () => {
  it("deduplicates discovery per client without coupling clients or state stores", async () => {
    let resolveFirst: (value: DiscoverResponse) => void = () => undefined;
    const first = new Promise<DiscoverResponse>((resolve) => {
      resolveFirst = resolve;
    });
    const firstCall = vi.fn().mockReturnValue(first);
    const secondCall = vi.fn().mockResolvedValue(discovery);
    const firstRpc = fakeRpc(firstCall);
    const secondRpc = fakeRpc(secondCall);
    const firstSink = capabilitySink();
    const secondSink = capabilitySink();

    const firstDiscovery = discoverRuntime(firstRpc, firstSink);
    expect(discoverRuntime(firstRpc, firstSink)).toBe(firstDiscovery);
    const secondDiscovery = discoverRuntime(secondRpc, secondSink);

    await Promise.resolve();
    expect(firstCall).toHaveBeenCalledOnce();
    expect(secondCall).toHaveBeenCalledOnce();

    resolveFirst(discovery);
    await Promise.all([firstDiscovery, secondDiscovery]);
    expect(firstSink.replace).toHaveBeenCalledWith(discovery.capabilities);
    expect(secondSink.replace).toHaveBeenCalledWith(discovery.capabilities);
  });

  it("clears a failed discovery so the next call can retry", async () => {
    const call = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(discovery);
    const rpc = fakeRpc(call as unknown as RpcClient["call"]);
    const sink = capabilitySink();

    await expect(discoverRuntime(rpc, sink)).rejects.toThrow("offline");
    await expect(discoverRuntime(rpc, sink)).resolves.toBeUndefined();
    expect(call).toHaveBeenCalledTimes(2);
  });
});
