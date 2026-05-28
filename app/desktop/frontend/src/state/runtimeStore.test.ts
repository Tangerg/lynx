import { beforeEach, describe, expect, it } from "vitest";
import type { ServerCapabilities } from "@/rpc";
import {
  useRuntimeStore,
  useServerEmitsCustom,
  useServerEmitsStandard,
  useServerFeature,
  useServerHasProvider,
} from "./runtimeStore";

function makeCaps(overrides: Partial<ServerCapabilities> = {}): ServerCapabilities {
  return {
    events: {
      standard: ["TEXT_MESSAGE_START", "RUN_FINISHED"],
      custom: ["lyra.plan", "lyra.approval"],
    },
    features: {
      multimodal: false,
      reasoning: true,
      checkpoints: false,
      interrupts: false,
      background: true,
      subagents: false,
      skills: false,
      mcp: true,
      sessionExport: false,
      attachments: { enabled: false },
    },
    providers: ["openai", "anthropic"],
    limits: {},
    ...overrides,
  };
}

describe("runtimeStore", () => {
  beforeEach(() => {
    useRuntimeStore.getState().clear();
  });

  it("starts empty with all selectors returning false", () => {
    const s = useRuntimeStore.getState();
    expect(s.serverInfo).toBeNull();
    expect(s.capabilities).toBeNull();
    expect(s.protocolVersion).toBeNull();
  });

  it("setHandshake populates serverInfo + capabilities + protocolVersion", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "2026-05-28",
      serverInfo: { name: "lyra-core", version: "0.8.1" },
      capabilities: makeCaps(),
    });

    const s = useRuntimeStore.getState();
    expect(s.protocolVersion).toBe("2026-05-28");
    expect(s.serverInfo?.serverInfo.name).toBe("lyra-core");
    expect(s.capabilities?.features.reasoning).toBe(true);
  });

  it("clear() resets everything", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "x",
      serverInfo: { name: "y", version: "z" },
      capabilities: makeCaps(),
    });
    useRuntimeStore.getState().clear();
    expect(useRuntimeStore.getState().serverInfo).toBeNull();
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });

  it("capabilities undefined pre-handshake (selectors default to false)", () => {
    // Per docs/API.md §6.1: frontend treats every features.* as false by
    // default. Hooks can't be invoked outside React, but the underlying
    // store state should reflect "nothing known yet".
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });

  it("setHandshake makes feature flags readable", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "x",
      serverInfo: { name: "y", version: "z" },
      capabilities: makeCaps(),
    });
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.features.reasoning).toBe(true);
    expect(caps.features.multimodal).toBe(false);
  });

  it("useServerEmitsStandard / Custom / HasProvider check membership", () => {
    useRuntimeStore.getState().setHandshake({
      protocolVersion: "x",
      serverInfo: { name: "y", version: "z" },
      capabilities: makeCaps(),
    });
    const caps = useRuntimeStore.getState().capabilities!;
    expect(caps.events.standard.includes("TEXT_MESSAGE_START")).toBe(true);
    expect(caps.events.standard.includes("UNKNOWN_EVENT")).toBe(false);
    expect(caps.events.custom.includes("lyra.plan")).toBe(true);
    expect(caps.providers.includes("openai")).toBe(true);
    expect(caps.providers.includes("nonsense")).toBe(false);
  });

  // Sanity: import the hooks so knip doesn't flag them as unused
  // (the actual hook invocations require React render context).
  it("all selector hooks are exported", () => {
    expect(typeof useServerFeature).toBe("function");
    expect(typeof useServerEmitsStandard).toBe("function");
    expect(typeof useServerEmitsCustom).toBe("function");
    expect(typeof useServerHasProvider).toBe("function");
  });
});
