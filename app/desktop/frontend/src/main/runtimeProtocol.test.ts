import { afterEach, describe, expect, it, vi } from "vitest";
import type { DiscoverResponse, RpcClient } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";
import { discoverRuntime } from "./runtimeProtocol";

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

afterEach(() => {
  useRuntimeStore.setState({ capabilities: null });
  vi.restoreAllMocks();
});

describe("discoverRuntime", () => {
  it("deduplicates per client and allows another client to discover independently", async () => {
    let resolveFirst: (value: DiscoverResponse) => void = () => undefined;
    const first = new Promise<DiscoverResponse>((resolve) => {
      resolveFirst = resolve;
    });
    const firstCall = vi.fn().mockReturnValue(first);
    const secondCall = vi.fn().mockResolvedValue(discovery);
    const firstRpc = fakeRpc(firstCall);
    const secondRpc = fakeRpc(secondCall);

    const firstDiscovery = discoverRuntime(firstRpc);
    expect(discoverRuntime(firstRpc)).toBe(firstDiscovery);
    const secondDiscovery = discoverRuntime(secondRpc);

    await Promise.resolve();
    expect(firstCall).toHaveBeenCalledOnce();
    expect(secondCall).toHaveBeenCalledOnce();

    resolveFirst(discovery);
    await Promise.all([firstDiscovery, secondDiscovery]);
    expect(useRuntimeStore.getState().capabilities).toEqual(discovery.capabilities);
  });

  it("clears a failed discovery so the next call can retry", async () => {
    const call = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(discovery);
    const rpc = fakeRpc(call as unknown as RpcClient["call"]);

    await expect(discoverRuntime(rpc)).rejects.toThrow("offline");
    await expect(discoverRuntime(rpc)).resolves.toBeUndefined();
    expect(call).toHaveBeenCalledTimes(2);
  });
});
