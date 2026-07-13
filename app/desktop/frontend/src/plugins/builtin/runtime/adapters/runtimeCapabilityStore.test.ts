import { beforeEach, describe, expect, it } from "vitest";
import type { ServerCapabilities } from "@/rpc";
import { useRuntimeStore, useServerFeature } from "./runtimeCapabilityStore";

function makeCaps(overrides: Partial<ServerCapabilities> = {}): ServerCapabilities {
  return {
    protocolVersion: "2026-06-07",
    events: ["segment.started", "segment.finished", "item.started", "item.delta", "item.completed"],
    features: {
      multimodal: false,
      reasoning: true,
      checkpoints: false,
      git: true,
      fileWatch: false,
      lsp: false,
      subagents: false,
      skills: false,
      mcp: true,
      sessionExport: false,
      memory: false,
      relocate: true,
      clientTools: false,
    },
    providers: ["openai", "anthropic"],
    streamingMethods: ["runs.start", "runs.resume", "runs.subscribe"],
    limits: {},
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
    expect(useRuntimeStore.getState().capabilities?.features.reasoning).toBe(true);
  });

  it("replace makes feature flags readable", () => {
    useRuntimeStore.getState().replace(makeCaps());
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.features.reasoning).toBe(true);
    expect(caps.features.multimodal).toBe(false);
  });

  it("events + providers are flat membership lists (§9)", () => {
    useRuntimeStore.getState().replace(makeCaps());
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.events.includes("item.started")).toBe(true);
    expect(caps.events.includes("UNKNOWN")).toBe(false);
    expect(caps.providers.includes("openai")).toBe(true);
    expect(caps.providers.includes("nonsense")).toBe(false);
  });

  // Sanity: import the selector so knip doesn't flag it as unused
  // (the actual hook invocation requires React render context).
  it("exports the feature selector", () => {
    expect(typeof useServerFeature).toBe("function");
  });
});
