import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import {
  HTTP_ENDPOINTS,
  PROTOCOL_VERSION,
  type DiscoverResponse,
  type LyraClient,
  type SidecarClient,
} from "@/rpc";
import { runtimeServiceInspector } from "./runtimeServiceInspector";

const discovery: DiscoverResponse = {
  protocolVersion: PROTOCOL_VERSION,
  serverInfo: {
    instanceId: "runtime_1",
    name: "lyra",
    version: "1.2.3",
    defaultWorkspace: { path: "/workspace" },
    home: "/home",
  },
  capabilities: {
    runEvents: [],
    runtimeTopics: ["files.changed"],
    features: {},
    streamingMethods: ["runtime.subscribe"],
    limits: {
      idempotency: { namespace: "idp_test", retentionSeconds: 86_400 },
      runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2048, maxBytes: 16_777_216 },
      mcpAuthorizationAttempts: { retentionSeconds: 600 },
      runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
    },
  },
};

function runtimeClient(discover = vi.fn().mockResolvedValue(discovery)): LyraClient {
  return { runtime: { discover } } as unknown as LyraClient;
}

function sidecar(overrides: Partial<SidecarClient> = {}): SidecarClient {
  return {
    info: vi.fn().mockResolvedValue({
      protocolVersion: PROTOCOL_VERSION,
      server: { name: "lyra", version: "1.2.3", instanceId: "runtime_1" },
      transport: "http",
      endpoints: {
        rpc: HTTP_ENDPOINTS.rpc.path,
        info: HTTP_ENDPOINTS.info.path,
        liveness: HTTP_ENDPOINTS.liveness.path,
        readiness: HTTP_ENDPOINTS.readiness.path,
      },
    }),
    liveness: vi.fn().mockResolvedValue({ status: "ok", instanceId: "runtime_1" }),
    readiness: vi.fn().mockResolvedValue({
      status: "degraded",
      instanceId: "runtime_1",
      checks: { database: "ok", git: "degraded" },
    }),
    ...overrides,
  };
}

afterEach(resetContainer);

describe("runtime service inspector", () => {
  it("consumes all sidecars and removes their HTTP representation", async () => {
    const client = sidecar();
    const runtime = runtimeClient();
    setContainer({ sidecar: () => client, client: () => runtime });
    const signal = new AbortController().signal;

    await expect(runtimeServiceInspector().inspect(signal)).resolves.toEqual({
      processGeneration: "runtime_1",
      service: {
        server: { name: "lyra", version: "1.2.3" },
        protocolVersion: PROTOCOL_VERSION,
        health: "degraded",
        checks: { database: "ready", git: "degraded" },
      },
      capabilities: discovery.capabilities,
    });
    const infoSignal = vi.mocked(client.info).mock.calls[0]?.[0];
    expect(infoSignal).toBeInstanceOf(AbortSignal);
    expect(infoSignal?.aborted).toBe(false);
    expect(client.liveness).toHaveBeenCalledWith(infoSignal);
    expect(client.readiness).toHaveBeenCalledWith(infoSignal);
    expect(runtime.runtime.discover).toHaveBeenCalledWith(infoSignal);
  });

  it("refuses a server whose advertised binding disagrees with the compiled contract", async () => {
    const client = sidecar({
      info: vi.fn().mockResolvedValue({
        protocolVersion: PROTOCOL_VERSION,
        server: { name: "lyra", version: "1.2.3", instanceId: "runtime_1" },
        transport: "http",
        endpoints: {
          rpc: "/v3/rpc",
          info: HTTP_ENDPOINTS.info.path,
          liveness: HTTP_ENDPOINTS.liveness.path,
          readiness: HTTP_ENDPOINTS.readiness.path,
        },
      }),
    });
    setContainer({ sidecar: () => client, client: () => runtimeClient() });

    await expect(runtimeServiceInspector().inspect(new AbortController().signal)).rejects.toThrow(
      "incompatible rpc endpoint",
    );
  });

  it("refuses a split observation from different HTTP and RPC server identities", async () => {
    const client = sidecar();
    const mismatched = {
      ...discovery,
      serverInfo: { ...discovery.serverInfo, version: "9.9.9" },
    };
    setContainer({
      sidecar: () => client,
      client: () => runtimeClient(vi.fn().mockResolvedValue(mismatched)),
    });

    await expect(runtimeServiceInspector().inspect(new AbortController().signal)).rejects.toThrow(
      "different servers",
    );
  });

  it("refuses one inspection stitched across Runtime process generations", async () => {
    const client = sidecar({
      info: vi.fn().mockResolvedValue({
        protocolVersion: PROTOCOL_VERSION,
        server: { name: "lyra", version: "1.2.3", instanceId: "runtime_retired" },
        transport: "http",
        endpoints: {
          rpc: HTTP_ENDPOINTS.rpc.path,
          info: HTTP_ENDPOINTS.info.path,
          liveness: HTTP_ENDPOINTS.liveness.path,
          readiness: HTTP_ENDPOINTS.readiness.path,
        },
      }),
      liveness: vi.fn().mockResolvedValue({ status: "ok", instanceId: "runtime_retired" }),
      readiness: vi.fn().mockResolvedValue({
        status: "ok",
        instanceId: "runtime_successor",
      }),
    });
    const successor = {
      ...discovery,
      serverInfo: { ...discovery.serverInfo, instanceId: "runtime_successor" },
    } satisfies DiscoverResponse;
    setContainer({
      sidecar: () => client,
      client: () => runtimeClient(vi.fn().mockResolvedValue(successor)),
    });

    await expect(runtimeServiceInspector().inspect(new AbortController().signal)).rejects.toThrow(
      "different Runtime process generations",
    );
  });

  it("cancels sibling sidecars when one member of the inspection fails", async () => {
    let infoSignal: AbortSignal | undefined;
    const client = sidecar({
      info: vi.fn<SidecarClient["info"]>((signal) => {
        infoSignal = signal;
        return new Promise((_resolve, reject) => {
          signal?.addEventListener("abort", () =>
            reject(new DOMException("aborted", "AbortError")),
          );
        });
      }),
      readiness: vi.fn().mockRejectedValue(new Error("readiness failed")),
    });
    setContainer({ sidecar: () => client, client: () => runtimeClient() });

    await expect(runtimeServiceInspector().inspect(new AbortController().signal)).rejects.toThrow(
      "readiness failed",
    );
    expect(infoSignal?.aborted).toBe(true);
  });

  it("cancels sidecars when protocol discovery fails", async () => {
    let liveSignal: AbortSignal | undefined;
    const client = sidecar({
      liveness: vi.fn<SidecarClient["liveness"]>((signal) => {
        liveSignal = signal;
        return new Promise((_resolve, reject) => {
          signal?.addEventListener("abort", () =>
            reject(new DOMException("aborted", "AbortError")),
          );
        });
      }),
    });
    const discover = vi.fn().mockRejectedValue(new Error("protocol mismatch"));
    setContainer({ sidecar: () => client, client: () => runtimeClient(discover) });

    await expect(runtimeServiceInspector().inspect(new AbortController().signal)).rejects.toThrow(
      "protocol mismatch",
    );
    expect(liveSignal?.aborted).toBe(true);
  });
});
