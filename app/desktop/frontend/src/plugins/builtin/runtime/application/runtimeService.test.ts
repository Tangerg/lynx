import { describe, expect, it, vi } from "vitest";
import type { ServerCapabilities } from "@/rpc";
import {
  createRuntimeServiceController,
  RUNTIME_SERVICE_HEALTHY_POLL_MS,
  RUNTIME_SERVICE_INSPECTION_TIMEOUT_MS,
  RUNTIME_SERVICE_RETRY_BASE_MS,
  RUNTIME_SERVICE_RETRY_CAP_MS,
  type RuntimeConnectionInspection,
  type RuntimeConnectionInspector,
  type RuntimeServiceObservation,
  type RuntimeServiceSink,
} from "./runtimeService";

const observation: RuntimeServiceObservation = {
  server: { name: "lyra", version: "1.2.3" },
  protocol: { current: "2026-07-01", minSupported: "2026-07-01" },
  health: "ready",
  checks: {},
};

const inspection: RuntimeConnectionInspection<ServerCapabilities> = {
  processGeneration: "runtime_1",
  service: observation,
  capabilities: {
    runEvents: [],
    runtimeTopics: [],
    stateSnapshots: [],
    features: {},
    streamingMethods: [],
    limits: {
      idempotency: { namespace: "idp_test", retentionSeconds: 86_400 },
      runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 2048, maxBytes: 16_777_216 },
      mcpAuthorizationAttempts: { retentionSeconds: 600 },
      runtimeSubscription: { maxTopics: 32, maxWatches: 32 },
    },
  },
};

function sink() {
  const checking = vi.fn<RuntimeServiceSink<ServerCapabilities>["checking"]>();
  const replace = vi.fn<RuntimeServiceSink<ServerCapabilities>["replace"]>();
  const unavailable = vi.fn<RuntimeServiceSink<ServerCapabilities>["unavailable"]>();
  return { checking, replace, unavailable } satisfies RuntimeServiceSink<ServerCapabilities>;
}

describe("runtime service controller", () => {
  it("coalesces concurrent refreshes into one inspection", async () => {
    let settle: (value: RuntimeConnectionInspection<ServerCapabilities>) => void = () => undefined;
    const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi.fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>(
        () =>
          new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
            settle = resolve;
          }),
      ),
    };
    const target = sink();
    const controller = createRuntimeServiceController(inspector, target);

    const first = controller.refresh();
    const second = controller.refresh();
    expect(second).toBe(first);
    expect(target.checking).toHaveBeenCalledOnce();
    expect(inspector.inspect).toHaveBeenCalledOnce();

    settle(inspection);
    await first;
    expect(target.replace).toHaveBeenCalledWith(inspection);
  });

  it("publishes an unavailable state and permits a later retry", async () => {
    const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi
        .fn()
        .mockRejectedValueOnce(new Error("connection refused"))
        .mockResolvedValueOnce(inspection),
    };
    const target = sink();
    const controller = createRuntimeServiceController(inspector, target);

    await controller.refresh();
    expect(target.unavailable).toHaveBeenCalledWith({
      reason: "failed",
      detail: "connection refused",
    });
    await controller.refresh();
    expect(inspector.inspect).toHaveBeenCalledTimes(2);
    expect(target.replace).toHaveBeenCalledWith(inspection);
  });

  it("silently recovers a consumer-reported connection loss", async () => {
    const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi.fn().mockRejectedValue(new Error("connection refused")),
    };
    const target = sink();
    const controller = createRuntimeServiceController(inspector, target);

    await controller.recover();

    expect(target.checking).not.toHaveBeenCalled();
    expect(target.unavailable).toHaveBeenCalledWith({
      reason: "failed",
      detail: "connection refused",
    });
  });

  it("supersedes an older inspection before starting recovery", async () => {
    let settle: (value: RuntimeConnectionInspection<ServerCapabilities>) => void = () => undefined;
    const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi
        .fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>()
        .mockImplementationOnce(
          () =>
            new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
              settle = resolve;
            }),
        )
        .mockRejectedValueOnce(new Error("connection refused")),
    };
    const target = sink();
    const controller = createRuntimeServiceController(inspector, target);

    const older = controller.refresh();
    const recovery = controller.recover();
    expect(inspector.inspect).toHaveBeenCalledTimes(2);
    await older;
    settle(inspection);
    await recovery;
    await Promise.resolve();

    expect(inspector.inspect).toHaveBeenCalledTimes(2);
    expect(target.replace).not.toHaveBeenCalled();
    expect(target.unavailable).toHaveBeenCalledWith({
      reason: "failed",
      detail: "connection refused",
    });
  });

  it("aborts the request and ignores a settlement after disposal", async () => {
    let settle: (value: RuntimeConnectionInspection<ServerCapabilities>) => void = () => undefined;
    let receivedSignal: AbortSignal | undefined;
    const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
      inspect: vi.fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>(
        (signal) =>
          new Promise<RuntimeConnectionInspection<ServerCapabilities>>((resolve) => {
            receivedSignal = signal;
            settle = resolve;
          }),
      ),
    };
    const target = sink();
    const controller = createRuntimeServiceController(inspector, target);

    const pending = controller.refresh();
    controller.dispose();
    expect(receivedSignal?.aborted).toBe(true);
    settle(inspection);
    await pending;
    expect(target.replace).not.toHaveBeenCalled();
    expect(target.unavailable).not.toHaveBeenCalled();
  });

  it("aborts a black-holed inspection and publishes a retryable timeout", async () => {
    vi.useFakeTimers();
    try {
      let receivedSignal: AbortSignal | undefined;
      const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
        inspect: vi.fn<RuntimeConnectionInspector<ServerCapabilities>["inspect"]>(
          (signal) =>
            new Promise((_resolve, reject) => {
              receivedSignal = signal;
              signal.addEventListener("abort", () =>
                reject(new DOMException("aborted", "AbortError")),
              );
            }),
        ),
      };
      const target = sink();
      const controller = createRuntimeServiceController(inspector, target);

      const pending = controller.refresh();
      await vi.advanceTimersByTimeAsync(RUNTIME_SERVICE_INSPECTION_TIMEOUT_MS);
      await pending;

      expect(receivedSignal?.aborted).toBe(true);
      expect(target.unavailable).toHaveBeenCalledWith({ reason: "timeout" });
    } finally {
      vi.useRealTimers();
    }
  });

  it("automatically retries a failed cold start and then polls a healthy Runtime", async () => {
    vi.useFakeTimers();
    try {
      const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
        inspect: vi.fn().mockRejectedValueOnce(new Error("offline")).mockResolvedValue(inspection),
      };
      const target = sink();
      const controller = createRuntimeServiceController(inspector, target);

      controller.start();
      await vi.advanceTimersByTimeAsync(0);
      expect(inspector.inspect).toHaveBeenCalledOnce();
      expect(target.unavailable).toHaveBeenCalledOnce();

      await vi.advanceTimersByTimeAsync(RUNTIME_SERVICE_RETRY_BASE_MS - 1);
      expect(inspector.inspect).toHaveBeenCalledOnce();
      await vi.advanceTimersByTimeAsync(1);
      expect(inspector.inspect).toHaveBeenCalledTimes(2);
      expect(target.replace).toHaveBeenCalledWith(inspection);

      await vi.advanceTimersByTimeAsync(RUNTIME_SERVICE_HEALTHY_POLL_MS);
      expect(inspector.inspect).toHaveBeenCalledTimes(3);
      controller.dispose();
    } finally {
      vi.useRealTimers();
    }
  });

  it("stops scheduled retries when disposed", async () => {
    vi.useFakeTimers();
    try {
      const inspector: RuntimeConnectionInspector<ServerCapabilities> = {
        inspect: vi.fn().mockRejectedValue(new Error("offline")),
      };
      const controller = createRuntimeServiceController(inspector, sink());

      controller.start();
      await vi.advanceTimersByTimeAsync(0);
      controller.dispose();
      await vi.advanceTimersByTimeAsync(RUNTIME_SERVICE_RETRY_CAP_MS * 2);

      expect(inspector.inspect).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });
});
