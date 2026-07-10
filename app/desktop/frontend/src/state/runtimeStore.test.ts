import { beforeEach, describe, expect, it } from "vitest";
import type { ServerCapabilities } from "@/rpc";
import { useRuntimeStore, useServerFeature } from "./runtimeStore";

function makeCaps(overrides: Partial<ServerCapabilities> = {}): ServerCapabilities {
  return {
    protocolVersion: "2026-06-07",
    events: ["run.started", "run.finished", "item.started", "item.delta", "item.completed"],
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

describe("runtimeStore", () => {
  beforeEach(() => {
    useRuntimeStore.setState({ capabilities: null });
  });

  it("starts empty (capabilities null before discovery)", () => {
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });

  it("setDiscovery stores capabilities", () => {
    useRuntimeStore.getState().setDiscovery(makeCaps());
    expect(useRuntimeStore.getState().capabilities?.features.reasoning).toBe(true);
  });

  it("setDiscovery makes feature flags readable", () => {
    useRuntimeStore.getState().setDiscovery(makeCaps());
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.features.reasoning).toBe(true);
    expect(caps.features.multimodal).toBe(false);
  });

  it("events + providers are flat membership lists (§9)", () => {
    useRuntimeStore.getState().setDiscovery(makeCaps());
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
