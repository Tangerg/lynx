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
    const performHandshake = vi.fn().mockResolvedValue(undefined);

    startBootstrapLifecycle({
      installPorts,
      initObservability,
      performHandshake,
      reportObservabilityFailure: vi.fn(),
      reportHandshakeFailure: vi.fn(),
    });

    expect(installPorts).toHaveBeenCalledOnce();
    expect(initObservability).toHaveBeenCalledOnce();
    expect(performHandshake).toHaveBeenCalledOnce();
  });

  it("reports handshake and observability failures without throwing", async () => {
    const reportObservabilityFailure = vi.fn();
    const reportHandshakeFailure = vi.fn();
    const observabilityError = new Error("otel failed");
    const handshakeError = new Error("initialize failed");

    startBootstrapLifecycle({
      installPorts: vi.fn(),
      initObservability: vi.fn().mockRejectedValue(observabilityError),
      performHandshake: vi.fn().mockRejectedValue(handshakeError),
      reportObservabilityFailure,
      reportHandshakeFailure,
    });
    await tick();

    expect(reportObservabilityFailure).toHaveBeenCalledWith(observabilityError);
    expect(reportHandshakeFailure).toHaveBeenCalledWith(handshakeError);
  });

  it("runs observability teardown on dispose after init resolves", async () => {
    const teardown = vi.fn();
    const dispose = startBootstrapLifecycle({
      installPorts: vi.fn(),
      initObservability: vi.fn().mockResolvedValue(teardown),
      performHandshake: vi.fn().mockResolvedValue(undefined),
      reportObservabilityFailure: vi.fn(),
      reportHandshakeFailure: vi.fn(),
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
      performHandshake: vi.fn().mockResolvedValue(undefined),
      reportObservabilityFailure: vi.fn(),
      reportHandshakeFailure: vi.fn(),
    });

    dispose();
    init.resolve(teardown);
    await tick();

    expect(teardown).toHaveBeenCalledOnce();
  });
});
