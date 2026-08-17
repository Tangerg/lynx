import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FeatureCapability, ServerCapabilities } from "@/rpc";
import type {
  RuntimeConnectionInspection,
  RuntimeConnectionInspector,
} from "../application/runtimeService";
import { runtimeCapabilities } from "../application/ports/capabilities";
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
      clientTools: stable(false),
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
    capabilities,
    service: {
      server: { name: "lyra", version: "1.2.3" },
      protocol: { current: "2026-07-01", minSupported: "2026-07-01" },
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

    successor.dispose();
    expect(useRuntimeConnectionStore.getState()).toMatchObject({
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
