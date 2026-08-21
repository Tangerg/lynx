import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FeatureCapability, ServerCapabilities } from "@/rpc";
import {
  RUNTIME_SERVICE_RETRY_BASE_MS,
  type RuntimeConnectionInspection,
  type RuntimeConnectionInspector,
} from "../application/runtimeService";
import { runtimeCapabilities } from "../application/ports/capabilities";
import { runtimeServiceStatus } from "../application/ports/serviceStatus";
import {
  resetRuntimeConnectionForTest,
  runtimeSupportsTopic,
  startRuntimeConnection,
  useRuntimeConnectionStore,
  useServerFeature,
} from "./runtimeConnectionProjection";

function stable(enabled: boolean): FeatureCapability {
  return { enabled, stability: "stable", clientOptIn: false, requiredByRunProtocol: false };
}

function makeCaps(overrides: Partial<ServerCapabilities> = {}): ServerCapabilities {
  return {
    runEvents: [
      "segment.started",
      "segment.finished",
      "item.started",
      "item.delta",
      "item.completed",
    ],
    runtimeTopics: ["files.changed", "skills.changed", "mcp.changed"],
    stateSnapshots: [],
    features: {
      multimodal: stable(false),
      reasoning: stable(true),
      checkpoints: stable(false),
      git: stable(true),
      fileWatch: stable(false),
      lsp: stable(false),
      subagents: stable(false),
      skills: stable(false),
      mcp: stable(true),
      sessionExport: stable(false),
      memory: stable(false),
      relocate: stable(true),
    },
    streamingMethods: ["runs.start", "runs.resume", "runs.subscribe"],
    limits: {
      idempotency: { namespace: "idp_test", retentionSeconds: 86_400 },
      runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2048, maxBytes: 16_777_216 },
      mcpAuthorizationAttempts: { retentionSeconds: 600 },
      runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
    },
    ...overrides,
  };
}

function inspection(capabilities = makeCaps()): RuntimeConnectionInspection<ServerCapabilities> {
  return {
    processGeneration: "runtime_1",
    capabilities,
    service: {
      server: { name: "lyra", version: "1.2.3" },
      protocolVersion: "2026-07-01",
      health: "ready",
      checks: {},
    },
  };
}

describe("runtime connection projection", () => {
  beforeEach(() => {
    resetRuntimeConnectionForTest();
  });

  it("starts empty before discovery", () => {
    expect(useRuntimeConnectionStore.getState()).toMatchObject({
      connectionGeneration: null,
      processGeneration: null,
      capabilities: null,
      service: { phase: "checking" },
    });
  });

  it("makes negotiated feature and topic facts readable", () => {
    useRuntimeConnectionStore.setState({ capabilities: makeCaps() });
    const caps = useRuntimeConnectionStore.getState().capabilities!;

    expect(caps.features.reasoning?.enabled).toBe(true);
    expect(caps.features.multimodal?.enabled).toBe(false);
    expect(caps.runEvents.includes("item.started")).toBe(true);
    expect(runtimeSupportsTopic("files.changed")).toBe(true);
    expect(runtimeSupportsTopic("knowledge.changed")).toBe(false);
    expect(typeof useServerFeature).toBe("function");
  });

  it("publishes process generation changes even when capabilities keep the same identity", async () => {
    const capabilities = makeCaps();
    const changed = vi.fn();
    let settleInspection: (value: RuntimeConnectionInspection<ServerCapabilities>) => void = () =>
      undefined;
    const owner = startRuntimeConnection({
      inspect: vi.fn(
        () =>
          new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
            settleInspection = resolve;
          }),
      ),
    });
    const unsubscribe = owner.subscribeConnection(changed);

    useRuntimeConnectionStore.setState({
      connectionGeneration: "connection_retired",
      processGeneration: "runtime_retired",
      capabilities,
    });
    useRuntimeConnectionStore.setState({
      connectionGeneration: "connection_successor",
      processGeneration: "runtime_successor",
      capabilities,
    });

    expect(changed).toHaveBeenCalledTimes(2);
    expect(owner.connectionGeneration()).toBe("connection_successor");
    unsubscribe();
    settleInspection(inspection());
    await vi.waitFor(() =>
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready"),
    );
    owner.dispose();
  });

  it("retires an old inspector and keeps its late result and disposer out of the successor", async () => {
    let settleRetired: (value: RuntimeConnectionInspection<ServerCapabilities>) => void = () =>
      undefined;
    let retiredSignal: AbortSignal | undefined;
    const retiredInspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi.fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>(
        (signal) =>
          new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
            retiredSignal = signal;
            settleRetired = resolve;
          }),
      ),
    };
    const retired = startRuntimeConnection(retiredInspector);
    const retiredChanges = vi.fn();
    const unsubscribeRetired = retired.subscribeConnection(retiredChanges);
    const successorCapabilities = makeCaps({ runtimeTopics: ["mcp.changed"] });
    const successor = startRuntimeConnection({
      inspect: vi.fn().mockResolvedValue(inspection(successorCapabilities)),
    });

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
    });
    expect(retiredSignal?.aborted).toBe(true);

    settleRetired(inspection(makeCaps({ runtimeTopics: ["files.changed"] })));
    await Promise.resolve();
    await Promise.resolve();
    retired.dispose();

    expect(useRuntimeConnectionStore.getState().capabilities).toBe(successorCapabilities);
    expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
    const successorGeneration = successor.connectionGeneration();
    expect(successorGeneration).not.toBeNull();
    expect(retired.connectionGeneration()).toBeNull();
    await retired.reportConnectionLoss(successorGeneration!);
    expect(successor.connectionGeneration()).toBe(successorGeneration);
    expect(retiredChanges).not.toHaveBeenCalled();
    unsubscribeRetired();

    successor.dispose();
    expect(useRuntimeConnectionStore.getState()).toMatchObject({
      connectionGeneration: null,
      processGeneration: null,
      capabilities: null,
      service: { phase: "checking" },
    });
  });

  it("publishes service readiness and capabilities as one observable state", async () => {
    const observed: Array<{ phase: string; hasCapabilities: boolean }> = [];
    const unsubscribe = useRuntimeConnectionStore.subscribe((state) => {
      observed.push({
        phase: state.service.phase,
        hasCapabilities: state.capabilities !== null,
      });
    });
    const owner = startRuntimeConnection({ inspect: vi.fn().mockResolvedValue(inspection()) });

    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
    });

    expect(observed).not.toContainEqual({ phase: "ready", hasCapabilities: false });
    expect(observed.at(-1)).toEqual({ phase: "ready", hasCapabilities: true });
    unsubscribe();
    owner.dispose();
  });

  it("keeps one connection generation across healthy inspections of the same process", async () => {
    const inspector = { inspect: vi.fn().mockResolvedValue(inspection()) };
    const owner = startRuntimeConnection(inspector);
    await vi.waitFor(() => expect(owner.connectionGeneration()).not.toBeNull());
    const admitted = owner.connectionGeneration();

    await runtimeServiceStatus().refresh();

    expect(inspector.inspect).toHaveBeenCalledTimes(2);
    expect(owner.connectionGeneration()).toBe(admitted);
    owner.dispose();
  });

  it("retires the old connection before committing and publishing a new server scope", async () => {
    let settleSuccessor: (value: RuntimeConnectionInspection<ServerCapabilities>) => void = () =>
      undefined;
    const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi
        .fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>()
        .mockResolvedValueOnce(inspection())
        .mockImplementationOnce(
          () =>
            new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
              settleSuccessor = resolve;
            }),
        ),
    };
    const owner = startRuntimeConnection(inspector);
    await vi.waitFor(() => expect(owner.connectionGeneration()).not.toBeNull());
    const predecessor = owner.connectionGeneration();
    const order: string[] = [];
    const unsubscribeConnection = owner.subscribeConnection(() =>
      order.push(`connection:${owner.connectionGeneration() ?? "none"}`),
    );
    const unsubscribeScope = owner.subscribeServerReplacement(() =>
      order.push(`scope:${owner.connectionGeneration() ?? "none"}`),
    );

    const replacement = owner.replaceEndpoint(() =>
      order.push(`commit:${owner.connectionGeneration() ?? "none"}`),
    );

    expect(order).toEqual(["connection:none", "commit:none", "scope:none"]);
    expect(useRuntimeConnectionStore.getState()).toMatchObject({
      connectionGeneration: null,
      processGeneration: null,
      capabilities: null,
      service: { phase: "checking" },
    });
    settleSuccessor(inspection());
    await replacement;
    expect(owner.connectionGeneration()).not.toBe(predecessor);
    unsubscribeScope();
    unsubscribeConnection();
    owner.dispose();
  });

  it("withdraws a lost event-stream generation before its verification settles", async () => {
    let settleRetiredInspection: (
      value: RuntimeConnectionInspection<ServerCapabilities>,
    ) => void = () => undefined;
    let settleRecovery: (value: RuntimeConnectionInspection<ServerCapabilities>) => void = () =>
      undefined;
    const signals: AbortSignal[] = [];
    const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi
        .fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>()
        .mockResolvedValueOnce(inspection())
        .mockImplementationOnce(
          (signal) =>
            new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
              signals.push(signal);
              settleRetiredInspection = resolve;
            }),
        )
        .mockImplementationOnce(
          (signal) =>
            new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
              signals.push(signal);
              settleRecovery = resolve;
            }),
        ),
    };
    const owner = startRuntimeConnection(inspector);
    await vi.waitFor(() => expect(owner.connectionGeneration()).not.toBeNull());
    const predecessorGeneration = owner.connectionGeneration()!;
    const connectionChanges = vi.fn();
    const unsubscribe = owner.subscribeConnection(connectionChanges);
    const retiredInspection = runtimeServiceStatus().refresh();
    await vi.waitFor(() => expect(inspector.inspect).toHaveBeenCalledTimes(2));

    const recovery = owner.reportConnectionLoss(predecessorGeneration);
    await Promise.resolve();

    expect(inspector.inspect).toHaveBeenCalledTimes(3);
    expect(signals[0]?.aborted).toBe(true);
    expect(owner.connectionGeneration()).toBeNull();
    expect(useRuntimeConnectionStore.getState().service.phase).toBe("reconnecting");

    settleRetiredInspection(inspection());
    await retiredInspection;
    await Promise.resolve();
    expect(owner.connectionGeneration()).toBeNull();

    // The Runtime process survived, but the recovered transport/event tail is a
    // distinct connection generation.
    settleRecovery(inspection());
    await recovery;
    const successorGeneration = owner.connectionGeneration();
    expect(successorGeneration).not.toBeNull();
    expect(successorGeneration).not.toBe(predecessorGeneration);
    expect(connectionChanges).toHaveBeenCalledTimes(2);

    await owner.reportConnectionLoss(predecessorGeneration);
    expect(owner.connectionGeneration()).toBe(successorGeneration);
    expect(connectionChanges).toHaveBeenCalledTimes(2);
    unsubscribe();
    owner.dispose();
  });

  it("keeps a failed recovery disconnected until an automatic retry publishes a successor", async () => {
    vi.useFakeTimers();
    try {
      const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
        inspect: vi
          .fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>()
          .mockResolvedValueOnce(inspection())
          .mockRejectedValueOnce(new Error("connection refused"))
          .mockResolvedValueOnce(inspection()),
      };
      const owner = startRuntimeConnection(inspector);
      await vi.advanceTimersByTimeAsync(0);
      const predecessorGeneration = owner.connectionGeneration();
      expect(predecessorGeneration).not.toBeNull();

      await owner.reportConnectionLoss(predecessorGeneration!);
      expect(useRuntimeConnectionStore.getState()).toMatchObject({
        connectionGeneration: null,
        processGeneration: null,
        capabilities: null,
        service: { phase: "unavailable" },
      });

      await vi.advanceTimersByTimeAsync(RUNTIME_SERVICE_RETRY_BASE_MS);
      const successorGeneration = owner.connectionGeneration();
      expect(successorGeneration).not.toBeNull();
      expect(successorGeneration).not.toBe(predecessorGeneration);
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
      owner.dispose();
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps the successor read port installed while capability subscribers reconcile", async () => {
    const retired = startRuntimeConnection({ inspect: vi.fn().mockResolvedValue(inspection()) });
    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
    });
    const reconciled: boolean[] = [];
    const unsubscribe = runtimeCapabilities().subscribe(() => {
      reconciled.push(runtimeCapabilities().supportsRuntimeTopic("mcp.changed"));
    });

    let successor: ReturnType<typeof startRuntimeConnection> | undefined;
    expect(() => {
      successor = startRuntimeConnection({ inspect: vi.fn().mockResolvedValue(inspection()) });
    }).not.toThrow();

    expect(reconciled).toContain(false);
    unsubscribe();
    retired.dispose();
    successor?.dispose();
  });

  it("keeps the retiring read port installed until final capability reconciliation finishes", async () => {
    const owner = startRuntimeConnection({ inspect: vi.fn().mockResolvedValue(inspection()) });
    await vi.waitFor(() => {
      expect(useRuntimeConnectionStore.getState().service.phase).toBe("ready");
    });
    const reconciled: boolean[] = [];
    const unsubscribe = runtimeCapabilities().subscribe(() => {
      reconciled.push(runtimeCapabilities().supportsRuntimeTopic("mcp.changed"));
    });

    expect(() => owner.dispose()).not.toThrow();

    expect(reconciled).toContain(false);
    unsubscribe();
  });
});
