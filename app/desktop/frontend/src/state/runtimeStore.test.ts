import { beforeEach, describe, expect, it } from "vitest";
import type { ServerCapabilities } from "@/rpc";
import {
  useRuntimeStore,
  useServerEmitsEvent,
  useServerFeature,
  useServerHasProvider,
} from "./runtimeStore";

function makeCaps(overrides: Partial<ServerCapabilities> = {}): ServerCapabilities {
  return {
    protocolVersion: "2026-06-03",
    events: ["run.started", "run.finished", "item.started", "item.delta", "item.completed"],
    features: {
      multimodal: false,
      reasoning: true,
      checkpoints: false,
      git: true,
      fileWatch: false,
      subagents: false,
      skills: false,
      mcp: true,
      sessionExport: false,
      memory: false,
      relocate: true,
      clientTools: false,
      attachments: { enabled: false },
    },
    providers: ["openai", "anthropic"],
    streamingMethods: ["runs.start", "runs.resume", "runs.subscribe"],
    limits: {},
    ...overrides,
  };
}

describe("runtimeStore", () => {
  beforeEach(() => {
    useRuntimeStore.getState().clear();
  });

  it("starts empty with all fields null", () => {
    const s = useRuntimeStore.getState();
    expect(s.serverName).toBeNull();
    expect(s.serverVersion).toBeNull();
    expect(s.capabilities).toBeNull();
    expect(s.protocolVersion).toBeNull();
  });

  it("setHandshake flattens InitializeResponse into store fields", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "2026-06-03",
      serverInfo: { name: "lyra-runtime", version: "0.0.0", cwd: "/w", home: "/h" },
      capabilities: makeCaps(),
    });

    const s = useRuntimeStore.getState();
    expect(s.protocolVersion).toBe("2026-06-03");
    expect(s.serverName).toBe("lyra-runtime");
    expect(s.serverVersion).toBe("0.0.0");
    expect(s.capabilities?.features.reasoning).toBe(true);
  });

  it("clear() resets everything", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "x",
      serverInfo: { name: "y", version: "z", cwd: "/w", home: "/h" },
      capabilities: makeCaps(),
    });
    useRuntimeStore.getState().clear();
    const s = useRuntimeStore.getState();
    expect(s.serverName).toBeNull();
    expect(s.capabilities).toBeNull();
    expect(s.protocolVersion).toBeNull();
  });

  it("setHandshake makes feature flags readable", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "x",
      serverInfo: { name: "y", version: "z", cwd: "/w", home: "/h" },
      capabilities: makeCaps(),
    });
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.features.reasoning).toBe(true);
    expect(caps.features.multimodal).toBe(false);
  });

  it("events + providers are flat membership lists (§9)", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "x",
      serverInfo: { name: "y", version: "z", cwd: "/w", home: "/h" },
      capabilities: makeCaps(),
    });
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.events.includes("item.started")).toBe(true);
    expect(caps.events.includes("UNKNOWN")).toBe(false);
    expect(caps.providers.includes("openai")).toBe(true);
    expect(caps.providers.includes("nonsense")).toBe(false);
  });

  // Sanity: import the hooks so knip doesn't flag them as unused
  // (the actual hook invocations require React render context).
  it("all selector hooks are exported", () => {
    expect(typeof useServerFeature).toBe("function");
    expect(typeof useServerEmitsEvent).toBe("function");
    expect(typeof useServerHasProvider).toBe("function");
  });
});
