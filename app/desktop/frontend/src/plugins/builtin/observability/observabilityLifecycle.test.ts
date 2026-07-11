import { describe, expect, it, vi } from "vitest";
import { startObservability, type ObservabilityTeardown } from "./observabilityLifecycle";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("startObservability", () => {
  it("reports initialization failures without breaking plugin startup", async () => {
    const error = new Error("otel failed");
    const reportFailure = vi.fn();

    startObservability(() => Promise.reject(error), reportFailure);
    await tick();

    expect(reportFailure).toHaveBeenCalledWith(error);
  });

  it("runs teardown after initialization", async () => {
    const teardown = vi.fn();
    const dispose = startObservability(() => Promise.resolve(teardown), vi.fn());
    await tick();

    dispose();

    expect(teardown).toHaveBeenCalledOnce();
  });

  it("tears down an initializer that resolves after disposal", async () => {
    const teardown = vi.fn<ObservabilityTeardown>();
    const initialization = deferred<ObservabilityTeardown>();
    const dispose = startObservability(() => initialization.promise, vi.fn());

    dispose();
    initialization.resolve(teardown);
    await tick();

    expect(teardown).toHaveBeenCalledOnce();
  });
});
