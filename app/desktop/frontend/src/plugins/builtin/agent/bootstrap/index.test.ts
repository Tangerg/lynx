// The boot handshake plugin negotiates runtime.initialize and stashes the
// result — but a backend that doesn't implement it yet must NOT break the
// app (degrade silently). Both paths are locked here.

import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { Container } from "@/main/container";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import type { LyraClient, Methods, ServerCapabilities } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";
import bootstrap from "./index";

const fakeCapabilities = { protocolVersion: "2026-06-03", features: {}, providers: [], events: [] };

// Bootstrap handshakes via the low-level rpc.call (so the auto-recovery path
// shares the exact negotiation), so the fake routes runtime.initialize there.
function stubContainer(initialize: Methods["runtime"]["initialize"]) {
  setContainer({
    sidecar: { info: vi.fn().mockResolvedValue({}) } as unknown as Container["sidecar"],
    client: () =>
      ({
        rpc: {
          call: (method: string, params?: unknown) =>
            method === "runtime.initialize"
              ? initialize(params as never)
              : Promise.reject(new Error(`unexpected method ${method}`)),
        },
      }) as unknown as LyraClient,
  });
}

afterEach(() => {
  resetContainer();
  useRuntimeStore.setState({ capabilities: null });
  vi.restoreAllMocks();
});

describe("bootstrap handshake", () => {
  it("negotiates runtime.initialize and stores the result", async () => {
    const initialize = vi.fn().mockResolvedValue({
      protocolVersion: "2026-06-03",
      serverInfo: { name: "lyra-runtime", version: "1.2.3", cwd: "/w", home: "/h" },
      capabilities: fakeCapabilities as unknown as ServerCapabilities,
    });
    stubContainer(initialize);

    await loadPlugin(bootstrap);

    await vi.waitFor(() => {
      expect(useRuntimeStore.getState().capabilities).not.toBeNull();
    });
    // The declared capabilities reached the runtime — interruptTypes carry
    // the HITL switches (API.md §6.2).
    const sent = initialize.mock.calls[0]![0] as { capabilities: { interruptTypes: string[] } };
    expect(sent.capabilities.interruptTypes).toContain("approval");
    expect(sent.capabilities.interruptTypes).toContain("question");
  });

  it("degrades silently when the runtime hasn't implemented initialize", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    stubContainer(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPlugin(bootstrap);

    await vi.waitFor(() => expect(warn).toHaveBeenCalled());
    // Store stays empty → every capability selector reads false (feature off).
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });
});
