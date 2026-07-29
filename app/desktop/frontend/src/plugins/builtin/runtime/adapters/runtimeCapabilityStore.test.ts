import { beforeEach, describe, expect, it } from "vitest";
import type { FeatureCapability, ServerCapabilities } from "@/rpc";
import { useRuntimeStore, useServerFeature } from "./runtimeCapabilityStore";

// Every advertised capability carries the feature's own negotiation facts. These
// fixtures are about whether a build offers a feature, so they say "advertised,
// nothing to opt into" once rather than at every key.
function stable(enabled: boolean): FeatureCapability {
  return { enabled, stability: "stable", clientOptIn: false, requiredByRunProtocol: false };
}

function makeCaps(overrides: Partial<ServerCapabilities> = {}): ServerCapabilities {
  return {
    runEvents: [
      "segment.started",
      "segment.finished",
      "item.started",
      "item.delta",
      "item.completed",
    ],
    runtimeTopics: ["files.changed", "skills.changed", "mcp.changed"],
    stateSnapshots: [],
    features: {
      multimodal: stable(false),
      reasoning: stable(true),
      checkpoints: stable(false),
      git: stable(true),
      fileWatch: stable(false),
      lsp: stable(false),
      subagents: stable(false),
      skills: stable(false),
      mcp: stable(true),
      sessionExport: stable(false),
      memory: stable(false),
      relocate: stable(true),
      clientTools: stable(false),
    },
    streamingMethods: ["runs.start", "runs.resume", "runs.subscribe"],
    limits: { runtimeSubscription: { maxTopics: 32, maxWatches: 32 } },
    ...overrides,
  };
}

describe("runtime capability store", () => {
  beforeEach(() => {
    useRuntimeStore.getState().clear();
  });

  it("starts empty (capabilities null before discovery)", () => {
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });

  it("replace stores capabilities", () => {
    useRuntimeStore.getState().replace(makeCaps());
    expect(useRuntimeStore.getState().capabilities?.features.reasoning?.enabled).toBe(true);
  });

  it("replace makes feature flags readable", () => {
    useRuntimeStore.getState().replace(makeCaps());
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.features.reasoning?.enabled).toBe(true);
    expect(caps.features.multimodal?.enabled).toBe(false);
  });

  it("events are a flat membership list (§9)", () => {
    useRuntimeStore.getState().replace(makeCaps());
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.runEvents.includes("item.started")).toBe(true);
    expect(caps.runEvents.includes("UNKNOWN")).toBe(false);
  });

  // Sanity: import the selector so knip doesn't flag it as unused
  // (the actual hook invocation requires React render context).
  it("exports the feature selector", () => {
    expect(typeof useServerFeature).toBe("function");
  });
});
