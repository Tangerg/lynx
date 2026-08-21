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
import {
  resetRuntimeConnectionForTest,
  useRuntimeConnectionStore,
} from "./adapters/runtimeConnectionProjection";
import runtimePlugin from "./index";
import { removeInstallation } from "@/plugins/sdk/kernel";
import { startKernel, stopKernel } from "@/plugins/sdk/bootstrap";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";

// Typed, not cast. What this test asserts is that discovery reaches the store at
// all, so the payload could be anything — which is exactly why it was written as
// `as unknown as ServerCapabilities` and then kept advertising `providers` and
// `events` years after the wire dropped them. A fixture the compiler holds cannot
// describe a runtime that does not exist.
const discovery: DiscoverResponse = {
  protocolVersion: PROTOCOL_VERSION,
  serverInfo: {
    instanceId: "runtime_1",
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
      protocolVersion: PROTOCOL_VERSION,
      server: { name: "lyra-runtime", version: "1.2.3", instanceId: "runtime_1" },
      transport: "http",
      endpoints: {
        rpc: HTTP_ENDPOINTS.rpc.path,
        info: HTTP_ENDPOINTS.info.path,
        liveness: HTTP_ENDPOINTS.liveness.path,
        readiness: HTTP_ENDPOINTS.readiness.path,
      },
    }),
    liveness: vi.fn().mockResolvedValue({ status: "ok", instanceId: "runtime_1" }),
    readiness: vi.fn().mockResolvedValue({ status: "ok", instanceId: "runtime_1" }),
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
  resetRuntimeConnectionForTest();
  vi.restoreAllMocks();
});

describe("runtime plugin", () => {
  it("discovers capabilities through the supervised Runtime connection", async () => {
    const discover = vi.fn().mockResolvedValue(discovery);
    stubContainer(discover);

    await loadPluginsForTest(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().capabilities).not.toBeNull();
    });
    expect(discover).toHaveBeenCalledOnce();
  });

  it("inspects all operational endpoints through the Runtime context", async () => {
    const sidecar = healthySidecar();
    stubContainer(vi.fn().mockResolvedValue(discovery), sidecar);

    await loadPluginsForTest(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
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
      expect(useRuntimeConnectionStore.getState().service).toMatchObject({
        phase: "unavailable",
        failure: { reason: "failed", detail: "connection refused" },
      });
    });
  });

  it("degrades without publishing stale capabilities when discovery fails", async () => {
    useRuntimeConnectionStore.setState({ capabilities: discovery.capabilities });
    stubContainer(vi.fn().mockRejectedValue(new Error("method not found")));

    await loadPluginsForTest(runtimePlugin);

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service).toMatchObject({
        phase: "unavailable",
        failure: { reason: "failed", detail: "method not found" },
      });
    });
    expect(useRuntimeConnectionStore.getState().capabilities).toBeNull();
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

    expect(useRuntimeConnectionStore.getState().capabilities).toBeNull();
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
      expect(useRuntimeConnectionStore.getState().capabilities).toBeNull();
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("unavailable");

      await vi.advanceTimersByTimeAsync(1_000);
      expect(discover).toHaveBeenCalledTimes(2);
      expect(useRuntimeConnectionStore.getState().capabilities).toEqual(discovery.capabilities);
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
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

    resolveReadiness({ status: "ok", instanceId: "runtime_1" });
    await Promise.resolve();
    await Promise.resolve();

    expect(useRuntimeConnectionStore.getState().service).toEqual({
      phase: "checking",
      observation: null,
      failure: null,
    });
  });

  it("does not let a retired Runtime installation clear its successor projection", async () => {
    stubContainer(vi.fn().mockResolvedValue(discovery));
    const retired = await startKernel([runtimePlugin]);
    let successor: Awaited<ReturnType<typeof startKernel>> | undefined;
    try {
      await vi.waitFor(() => {
        expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
      });

      stubContainer(vi.fn().mockResolvedValue(discovery));
      successor = await startKernel([runtimePlugin]);
      await vi.waitFor(() => {
        expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
        expect(useRuntimeConnectionStore.getState().capabilities).toEqual(discovery.capabilities);
      });

      await stopKernel(retired);

      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
      expect(useRuntimeConnectionStore.getState().capabilities).toEqual(discovery.capabilities);
    } finally {
      if (successor) await stopKernel(successor);
      else await stopKernel(retired);
    }
  });
});
