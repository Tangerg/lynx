import { beforeEach, describe, expect, it } from "vitest";
import type { ServerCapabilities } from "@/rpc";
import { useRuntimeStore, useServerFeature } from "./runtimeCapabilityStore";

function makeCaps(overrides: Partial<ServerCapabilities> = {}): ServerCapabilities {
  return {
    events: ["segment.started", "segment.finished", "item.started", "item.delta", "item.completed"],
    features: {
      multimodal: { enabled: false, stability: "stable" },
      reasoning: { enabled: true, stability: "stable" },
      checkpoints: { enabled: false, stability: "stable" },
      git: { enabled: true, stability: "stable" },
      fileWatch: { enabled: false, stability: "stable" },
      lsp: { enabled: false, stability: "stable" },
      subagents: { enabled: false, stability: "stable" },
      skills: { enabled: false, stability: "stable" },
      mcp: { enabled: true, stability: "stable" },
      sessionExport: { enabled: false, stability: "stable" },
      memory: { enabled: false, stability: "stable" },
      relocate: { enabled: true, stability: "stable" },
      clientTools: { enabled: false, stability: "stable" },
    },
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
    expect(caps.events.includes("item.started")).toBe(true);
    expect(caps.events.includes("UNKNOWN")).toBe(false);
  });

  // Sanity: import the selector so knip doesn't flag it as unused
  // (the actual hook invocation requires React render context).
  it("exports the feature selector", () => {
    expect(typeof useServerFeature).toBe("function");
  });
});
