// The boot plugin discovers runtime capabilities and stashes the result — but
// a backend that doesn't implement discovery yet must NOT break the app
// (degrade silently). Both paths are locked here.

import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import type { LyraClient, Methods, ServerCapabilities } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";
import bootstrap from "./index";

const fakeCapabilities = { protocolVersion: "2026-06-07", features: {}, providers: [], events: [] };

function stubContainer(discover: Methods["runtime"]["discover"]) {
  setContainer({
    client: () =>
      ({
        rpc: {
          call: (method: string) =>
            method === "runtime.discover"
              ? discover()
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

describe("bootstrap discovery", () => {
  it("discovers runtime capabilities and stores the result", async () => {
    const discover = vi.fn().mockResolvedValue({
      protocolVersion: "2026-06-07",
      serverInfo: { name: "lyra-runtime", version: "1.2.3", cwd: "/w", home: "/h" },
      capabilities: fakeCapabilities as unknown as ServerCapabilities,
    });
    stubContainer(discover);

    await loadPlugin(bootstrap);

    await vi.waitFor(() => {
      expect(useRuntimeStore.getState().capabilities).not.toBeNull();
    });
    expect(discover).toHaveBeenCalledOnce();
  });

  it("degrades silently when the runtime hasn't implemented discovery", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    stubContainer(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPlugin(bootstrap);

    await vi.waitFor(() => expect(warn).toHaveBeenCalled());
    // Store stays empty → every capability selector reads false (feature off).
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });
});
