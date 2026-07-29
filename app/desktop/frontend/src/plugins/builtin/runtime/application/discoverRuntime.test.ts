import { describe, expect, it, vi } from "vitest";
import type { DiscoverResponse, FeatureCapability, RpcClient, ServerCapabilities } from "@/rpc";
import { discoverRuntime, type RuntimeCapabilitySink } from "./discoverRuntime";

// Every advertised capability carries the feature's own negotiation facts. These
// fixtures are about whether a build offers a feature, so they say "advertised,
// nothing to opt into" once rather than at every key.
function stable(enabled: boolean): FeatureCapability {
  return { enabled, stability: "stable", clientOptIn: false, requiredByRunProtocol: false };
}

const discovery: DiscoverResponse = {
  protocol: { current: "2026-07-19", minSupported: "2026-07-19" },
  serverInfo: { name: "lyra-runtime", version: "1.0.0", cwd: "/work", home: "/home" },
  capabilities: {
    runEvents: [],
    runtimeTopics: [],
    stateSnapshots: [],
    streamingMethods: [],
    features: {
      reasoning: stable(false),
      mcp: stable(false),
      multimodal: stable(false),
      git: stable(false),
      fileWatch: stable(false),
      checkpoints: stable(false),
      lsp: stable(false),
      subagents: stable(false),
      skills: stable(false),
      sessionExport: stable(false),
      memory: stable(false),
      relocate: stable(false),
      clientTools: stable(false),
    },
    limits: { runtimeSubscription: { maxTopics: 32, maxWatches: 32 } },
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
