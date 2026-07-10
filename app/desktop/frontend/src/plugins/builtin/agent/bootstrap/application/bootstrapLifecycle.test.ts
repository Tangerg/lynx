import { describe, expect, it, vi } from "vitest";
import { startBootstrapLifecycle, type BootstrapTeardown } from "./bootstrapLifecycle";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("startBootstrapLifecycle", () => {
  it("installs ports synchronously and starts boot tasks", () => {
    const installPorts = vi.fn();
    const initObservability = vi.fn().mockResolvedValue(vi.fn());
    const discoverRuntime = vi.fn().mockResolvedValue(undefined);

    startBootstrapLifecycle({
      installPorts,
      initObservability,
      discoverRuntime,
      reportObservabilityFailure: vi.fn(),
      reportDiscoveryFailure: vi.fn(),
    });

    expect(installPorts).toHaveBeenCalledOnce();
    expect(initObservability).toHaveBeenCalledOnce();
    expect(discoverRuntime).toHaveBeenCalledOnce();
  });

  it("reports discovery and observability failures without throwing", async () => {
    const reportObservabilityFailure = vi.fn();
    const reportDiscoveryFailure = vi.fn();
    const observabilityError = new Error("otel failed");
    const discoveryError = new Error("discover failed");

    startBootstrapLifecycle({
      installPorts: vi.fn(),
      initObservability: vi.fn().mockRejectedValue(observabilityError),
      discoverRuntime: vi.fn().mockRejectedValue(discoveryError),
      reportObservabilityFailure,
      reportDiscoveryFailure,
    });
    await tick();

    expect(reportObservabilityFailure).toHaveBeenCalledWith(observabilityError);
    expect(reportDiscoveryFailure).toHaveBeenCalledWith(discoveryError);
  });

  it("runs observability teardown on dispose after init resolves", async () => {
    const teardown = vi.fn();
    const dispose = startBootstrapLifecycle({
      installPorts: vi.fn(),
      initObservability: vi.fn().mockResolvedValue(teardown),
      discoverRuntime: vi.fn().mockResolvedValue(undefined),
      reportObservabilityFailure: vi.fn(),
      reportDiscoveryFailure: vi.fn(),
    });
    await tick();

    dispose();

    expect(teardown).toHaveBeenCalledOnce();
  });

  it("runs observability teardown when init resolves after dispose", async () => {
    const teardown = vi.fn<BootstrapTeardown>();
    const init = deferred<BootstrapTeardown>();
    const dispose = startBootstrapLifecycle({
      installPorts: vi.fn(),
      initObservability: vi.fn().mockReturnValue(init.promise),
      discoverRuntime: vi.fn().mockResolvedValue(undefined),
      reportObservabilityFailure: vi.fn(),
      reportDiscoveryFailure: vi.fn(),
    });

    dispose();
    init.resolve(teardown);
    await tick();

    expect(teardown).toHaveBeenCalledOnce();
  });
});
