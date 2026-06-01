// The boot handshake plugin negotiates runtime.initialize and stashes the
// result — but a backend that doesn't implement it yet must NOT break the
// app (degrade silently). Both paths are locked here.

import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { Container } from "@/main/container";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import type { Methods, ServerCapabilities } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";
import bootstrap from "./index";

const fakeCapabilities = { features: {}, providers: [], events: { standard: [], custom: [] } };

function stubContainer(initialize: Methods["runtime"]["initialize"]) {
  setContainer({
    sidecar: { info: vi.fn().mockResolvedValue({}) } as unknown as Container["sidecar"],
    methods: () => ({ runtime: { initialize } }) as unknown as Methods,
  });
}

afterEach(() => {
  resetContainer();
  useRuntimeStore.getState().clear();
  vi.restoreAllMocks();
});

describe("bootstrap handshake", () => {
  it("negotiates runtime.initialize and stores the result", async () => {
    const initialize = vi.fn().mockResolvedValue({
      protocolVersion: "2026-05-28",
      serverInfo: { name: "lyra-runtime", version: "1.2.3" },
      capabilities: fakeCapabilities as unknown as ServerCapabilities,
    });
    stubContainer(initialize);

    await loadPlugin(bootstrap);

    await vi.waitFor(() => {
      expect(useRuntimeStore.getState().protocolVersion).toBe("2026-05-28");
    });
    // The declared capabilities reached the runtime — custom events carry
    // the HITL switches.
    const sent = initialize.mock.calls[0]![0] as { capabilities: { events: { custom: string[] } } };
    expect(sent.capabilities.events.custom).toContain("lyra.approval");
    expect(sent.capabilities.events.custom).toContain("lyra.question");
    expect(useRuntimeStore.getState().serverName).toBe("lyra-runtime");
  });

  it("degrades silently when the runtime hasn't implemented initialize", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    stubContainer(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPlugin(bootstrap);

    await vi.waitFor(() => expect(warn).toHaveBeenCalled());
    // Store stays empty → every capability selector reads false (feature off).
    expect(useRuntimeStore.getState().protocolVersion).toBeNull();
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });
});
