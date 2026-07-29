import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { loadPlugin, unloadPlugin } from "@/plugins/sdk/definePlugin";
import { PROTOCOL_VERSION, type DiscoverResponse, type LyraClient, type Methods } from "@/rpc";
import { useRuntimeStore } from "./adapters/runtimeCapabilityStore";
import runtimePlugin from "./index";

// Typed, not cast. What this test asserts is that discovery reaches the store at
// all, so the payload could be anything — which is exactly why it was written as
// `as unknown as ServerCapabilities` and then kept advertising `providers` and
// `events` years after the wire dropped them. A fixture the compiler holds cannot
// describe a runtime that does not exist.
const discovery: DiscoverResponse = {
  protocol: { current: PROTOCOL_VERSION, minSupported: PROTOCOL_VERSION },
  serverInfo: { name: "lyra-runtime", version: "1.2.3", cwd: "/w", home: "/h" },
  capabilities: {
    features: {},
    runEvents: [],
    runtimeTopics: [],
    stateSnapshots: [],
    streamingMethods: [],
    limits: { runtimeSubscription: { maxTopics: 8, maxWatches: 8 } },
  },
};

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
    const discover = vi.fn().mockResolvedValue(discovery);
    stubContainer(discover);

    await loadPlugin(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeStore.getState().capabilities).not.toBeNull();
    });
    expect(discover).toHaveBeenCalledOnce();
  });

  it("degrades without publishing stale capabilities when discovery fails", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    useRuntimeStore.getState().replace(discovery.capabilities);
    stubContainer(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPlugin(runtimePlugin);

    await vi.waitFor(() => expect(warn).toHaveBeenCalled());
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });
});
