import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { loadPlugin, unloadPlugin } from "@/plugins/sdk/definePlugin";
import type { LyraClient, Methods, ServerCapabilities } from "@/rpc";
import { useRuntimeStore } from "./adapters/runtimeCapabilityStore";
import runtimePlugin from "./index";

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
  unloadPlugin(runtimePlugin.name);
  resetContainer();
  useRuntimeStore.getState().clear();
  vi.restoreAllMocks();
});

describe("runtime plugin", () => {
  it("discovers capabilities through the Runtime composition boundary", async () => {
    const discover = vi.fn().mockResolvedValue({
      protocolVersion: "2026-06-07",
      serverInfo: { name: "lyra-runtime", version: "1.2.3", cwd: "/w", home: "/h" },
      capabilities: fakeCapabilities as unknown as ServerCapabilities,
    });
    stubContainer(discover);

    await loadPlugin(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeStore.getState().capabilities).not.toBeNull();
    });
    expect(discover).toHaveBeenCalledOnce();
  });

  it("degrades without publishing stale capabilities when discovery fails", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    useRuntimeStore.getState().replace(fakeCapabilities as unknown as ServerCapabilities);
    stubContainer(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPlugin(runtimePlugin);

    await vi.waitFor(() => expect(warn).toHaveBeenCalled());
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });
});
