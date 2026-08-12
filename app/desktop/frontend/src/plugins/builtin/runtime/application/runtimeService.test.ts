import { describe, expect, it, vi } from "vitest";
import {
  createRuntimeServiceController,
  RUNTIME_SERVICE_INSPECTION_TIMEOUT_MS,
  type RuntimeServiceInspector,
  type RuntimeServiceObservation,
  type RuntimeServiceSink,
} from "./runtimeService";

const observation: RuntimeServiceObservation = {
  server: { name: "lyra", version: "1.2.3" },
  protocol: { current: "2026-07-01", minSupported: "2026-07-01" },
  health: "ready",
  checks: {},
};

function sink() {
  const checking = vi.fn<RuntimeServiceSink["checking"]>();
  const replace = vi.fn<RuntimeServiceSink["replace"]>();
  const unavailable = vi.fn<RuntimeServiceSink["unavailable"]>();
  return { checking, replace, unavailable } satisfies RuntimeServiceSink;
}

describe("runtime service controller", () => {
  it("coalesces concurrent refreshes into one inspection", async () => {
    let settle: (value: RuntimeServiceObservation) => void = () => undefined;
    const inspector: RuntimeServiceInspector = {
      inspect: vi.fn<RuntimeServiceInspector["inspect"]>(
        () =>
          new Promise<RuntimeServiceObservation>((resolve) => {
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

    settle(observation);
    await first;
    expect(target.replace).toHaveBeenCalledWith(observation);
  });

  it("publishes an unavailable state and permits a later retry", async () => {
    const inspector: RuntimeServiceInspector = {
      inspect: vi
        .fn()
        .mockRejectedValueOnce(new Error("connection refused"))
        .mockResolvedValueOnce(observation),
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
    expect(target.replace).toHaveBeenCalledWith(observation);
  });

  it("aborts the request and ignores a settlement after disposal", async () => {
    let settle: (value: RuntimeServiceObservation) => void = () => undefined;
    let receivedSignal: AbortSignal | undefined;
    const inspector: RuntimeServiceInspector = {
      inspect: vi.fn<RuntimeServiceInspector["inspect"]>(
        (signal) =>
          new Promise<RuntimeServiceObservation>((resolve) => {
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
    settle(observation);
    await pending;
    expect(target.replace).not.toHaveBeenCalled();
    expect(target.unavailable).not.toHaveBeenCalled();
  });

  it("aborts a black-holed inspection and publishes a retryable timeout", async () => {
    vi.useFakeTimers();
    try {
      let receivedSignal: AbortSignal | undefined;
      const inspector: RuntimeServiceInspector = {
        inspect: vi.fn<RuntimeServiceInspector["inspect"]>(
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
});
