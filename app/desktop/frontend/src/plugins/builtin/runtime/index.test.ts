import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import {
  HTTP_ENDPOINTS,
  PROTOCOL_VERSION,
  type DiscoverResponse,
  type LyraClient,
  type Methods,
  type ReadinessStatus,
  type SidecarClient,
} from "@/rpc";
import { useRuntimeStore } from "./adapters/runtimeCapabilityStore";
import { useRuntimeServiceStore } from "./adapters/runtimeServiceStore";
import runtimePlugin from "./index";
import { removeInstallation } from "@/plugins/sdk/kernel";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";

// Typed, not cast. What this test asserts is that discovery reaches the store at
// all, so the payload could be anything — which is exactly why it was written as
// `as unknown as ServerCapabilities` and then kept advertising `providers` and
// `events` years after the wire dropped them. A fixture the compiler holds cannot
// describe a runtime that does not exist.
const discovery: DiscoverResponse = {
  protocol: { current: PROTOCOL_VERSION, minSupported: PROTOCOL_VERSION },
  serverInfo: {
    name: "lyra-runtime",
    version: "1.2.3",
    defaultWorkspace: { path: "/w" },
    home: "/h",
  },
  capabilities: {
    features: {},
    runEvents: [],
    runtimeTopics: [],
    stateSnapshots: [],
    streamingMethods: [],
    limits: {
      idempotency: { namespace: "idp_test", retentionSeconds: 86_400 },
      runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2048, maxBytes: 16_777_216 },
      mcpAuthorizationAttempts: { retentionSeconds: 600 },
      runtimeSubscription: { maxTopics: 8, maxWatches: 8 },
    },
  },
};

function healthySidecar(): SidecarClient {
  return {
    info: vi.fn().mockResolvedValue({
      protocol: { current: PROTOCOL_VERSION, minSupported: PROTOCOL_VERSION },
      server: { name: "lyra-runtime", version: "1.2.3" },
      transport: "http",
      endpoints: {
        rpc: HTTP_ENDPOINTS.rpc.path,
        info: HTTP_ENDPOINTS.info.path,
        liveness: HTTP_ENDPOINTS.liveness.path,
        readiness: HTTP_ENDPOINTS.readiness.path,
      },
    }),
    liveness: vi.fn().mockResolvedValue({ status: "ok" }),
    readiness: vi.fn().mockResolvedValue({ status: "ok" }),
  };
}

function stubContainer(
  discover: Methods["runtime"]["discover"],
  sidecar: SidecarClient = healthySidecar(),
) {
  setContainer({
    client: () =>
      ({
        runtime: { discover },
      }) as unknown as LyraClient,
    sidecar: () => sidecar,
  });
}

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
  useRuntimeStore.getState().clear();
  useRuntimeServiceStore.getState().clear();
  vi.restoreAllMocks();
});

describe("runtime plugin", () => {
  it("discovers capabilities through the supervised Runtime connection", async () => {
    const discover = vi.fn().mockResolvedValue(discovery);
    stubContainer(discover);

    await loadPluginsForTest(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeStore.getState().capabilities).not.toBeNull();
    });
    expect(discover).toHaveBeenCalledOnce();
  });

  it("inspects all operational endpoints through the Runtime context", async () => {
    const sidecar = healthySidecar();
    stubContainer(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeServiceStore.getState().snapshot.phase).toBe("ready");
    });
    expect(sidecar.info).toHaveBeenCalledOnce();
    expect(sidecar.liveness).toHaveBeenCalledOnce();
    expect(sidecar.readiness).toHaveBeenCalledOnce();
  });

  it("publishes sidecar failure without preserving a stale ready phase", async () => {
    const sidecar = healthySidecar();
    sidecar.readiness = vi.fn().mockRejectedValue(new Error("connection refused"));
    stubContainer(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeServiceStore.getState().snapshot).toMatchObject({
        phase: "unavailable",
        failure: { reason: "failed", detail: "connection refused" },
      });
    });
  });

  it("degrades without publishing stale capabilities when discovery fails", async () => {
    useRuntimeStore.getState().replace(discovery.capabilities);
    stubContainer(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPluginsForTest(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeServiceStore.getState().snapshot).toMatchObject({
        phase: "unavailable",
        failure: { reason: "failed", detail: "method not found" },
      });
    });
    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });

  it("does not publish a discovery result after the plugin is unloaded", async () => {
    let resolveDiscovery: (value: DiscoverResponse) => void = () => undefined;
    const discover = vi.fn(
      () =>
        new Promise<DiscoverResponse>((resolve) => {
          resolveDiscovery = resolve;
        }),
    );
    stubContainer(discover);

    await loadPluginsForTest(runtimePlugin);
    await vi.waitFor(() => expect(discover).toHaveBeenCalledOnce());
    await removeInstallation(runtimePlugin.name);

    resolveDiscovery(discovery);
    await Promise.resolve();
    await Promise.resolve();

    expect(useRuntimeStore.getState().capabilities).toBeNull();
  });

  it("automatically rediscovers and republishes capabilities after a cold-start outage", async () => {
    vi.useFakeTimers();
    try {
      const discover = vi
        .fn()
        .mockRejectedValueOnce(new Error("offline"))
        .mockResolvedValue(discovery);
      stubContainer(discover);

      await loadPluginsForTest(runtimePlugin);
      await vi.advanceTimersByTimeAsync(0);
      expect(useRuntimeStore.getState().capabilities).toBeNull();
      expect(useRuntimeServiceStore.getState().snapshot.phase).toBe("unavailable");

      await vi.advanceTimersByTimeAsync(1_000);
      expect(discover).toHaveBeenCalledTimes(2);
      expect(useRuntimeStore.getState().capabilities).toEqual(discovery.capabilities);
      expect(useRuntimeServiceStore.getState().snapshot.phase).toBe("ready");
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not publish a sidecar result after the plugin is unloaded", async () => {
    let resolveReadiness: (value: ReadinessStatus) => void = () => undefined;
    const sidecar = healthySidecar();
    sidecar.readiness = vi.fn<SidecarClient["readiness"]>(
      () =>
        new Promise<ReadinessStatus>((resolve) => {
          resolveReadiness = resolve;
        }),
    );
    stubContainer(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(runtimePlugin);
    await vi.waitFor(() => expect(sidecar.readiness).toHaveBeenCalledOnce());
    await removeInstallation(runtimePlugin.name);

    resolveReadiness({ status: "ok" });
    await Promise.resolve();
    await Promise.resolve();

    expect(useRuntimeServiceStore.getState().snapshot).toEqual({
      phase: "checking",
      observation: null,
      failure: null,
    });
  });
});
