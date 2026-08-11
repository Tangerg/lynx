import { describe, expect, it, vi } from "vitest";
import { type FeatureCapability, type ServerCapabilities } from "@/rpc";
import { discoverRuntime, type RuntimeDiscovery } from "./discoverRuntime";

// Every advertised capability carries the feature's own negotiation facts. These
// fixtures are about whether a build offers a feature, so they say "advertised,
// nothing to opt into" once rather than at every key.
function stable(enabled: boolean): FeatureCapability {
  return { enabled, stability: "stable", clientOptIn: false, requiredByRunProtocol: false };
}

const capabilities: ServerCapabilities = {
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
  limits: {
    idempotency: { retentionSeconds: 86_400 },
    runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2048, maxBytes: 16_777_216 },
    mcpAuthorizationAttempts: { retentionSeconds: 600 },
    runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
  },
};

function discovery(
  discoverCapabilities: RuntimeDiscovery["discoverCapabilities"],
): RuntimeDiscovery {
  return { discoverCapabilities };
}

describe("discoverRuntime", () => {
  it("deduplicates discovery per gateway without coupling independent gateways", async () => {
    let resolveFirst: (value: ServerCapabilities) => void = () => undefined;
    const first = new Promise<ServerCapabilities>((resolve) => {
      resolveFirst = resolve;
    });
    const firstCall = vi.fn().mockReturnValue(first);
    const secondCall = vi.fn().mockResolvedValue(capabilities);
    const firstSource = discovery(firstCall);
    const secondSource = discovery(secondCall);

    const firstResult = discoverRuntime(firstSource);
    expect(discoverRuntime(firstSource)).toBe(firstResult);
    const secondResult = discoverRuntime(secondSource);

    await Promise.resolve();
    expect(firstCall).toHaveBeenCalledOnce();
    expect(secondCall).toHaveBeenCalledOnce();

    resolveFirst(capabilities);
    await expect(firstResult).resolves.toBe(capabilities);
    await expect(secondResult).resolves.toBe(capabilities);
  });

  it("clears a failed discovery so the next call can retry", async () => {
    const call = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(capabilities);
    const source = discovery(call);

    await expect(discoverRuntime(source)).rejects.toThrow("offline");
    await expect(discoverRuntime(source)).resolves.toBe(capabilities);
    expect(call).toHaveBeenCalledTimes(2);
  });
});
