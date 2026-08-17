import { describe, expect, it, vi } from "vitest";
import { DesktopRenderer, type DesktopRendererDependencies } from "./renderer";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

function rendererDependencies(overrides: Partial<DesktopRendererDependencies> = {}) {
  return {
    initializeDesktopHost: vi.fn(async () => undefined),
    prepareWindowChrome: vi.fn(async () => undefined),
    watchWindowChrome: vi.fn(() => vi.fn()),
    mount: vi.fn(() => ({ unmount: vi.fn() })),
    closeRuntime: vi.fn(async () => undefined),
    reportFailure: vi.fn(),
    ...overrides,
  };
}

describe("DesktopRenderer", () => {
  it("cannot mount after final close wins an in-flight bootstrap", async () => {
    const bootstrap = deferred<void>();
    const deps = rendererDependencies({
      initializeDesktopHost: vi.fn(() => bootstrap.promise),
    });
    const renderer = new DesktopRenderer(deps);

    const startup = renderer.start();
    const closing = renderer.dispose();
    bootstrap.resolve();

    await Promise.all([startup, closing]);
    expect(deps.prepareWindowChrome).not.toHaveBeenCalled();
    expect(deps.watchWindowChrome).not.toHaveBeenCalled();
    expect(deps.mount).not.toHaveBeenCalled();
    expect(deps.closeRuntime).toHaveBeenCalledOnce();
  });

  it("cannot install window owners after close wins chrome preparation", async () => {
    const chrome = deferred<void>();
    const deps = rendererDependencies({
      prepareWindowChrome: vi.fn(() => chrome.promise),
    });
    const renderer = new DesktopRenderer(deps);

    const startup = renderer.start();
    await vi.waitFor(() => expect(deps.prepareWindowChrome).toHaveBeenCalledOnce());
    const closing = renderer.dispose();
    chrome.resolve();

    await Promise.all([startup, closing]);
    expect(deps.watchWindowChrome).not.toHaveBeenCalled();
    expect(deps.mount).not.toHaveBeenCalled();
  });

  it("retires the mounted root and watcher synchronously with one close settlement", async () => {
    const unmount = vi.fn();
    const stopWatching = vi.fn();
    const runtimeClose = deferred<void>();
    const deps = rendererDependencies({
      watchWindowChrome: vi.fn(() => stopWatching),
      mount: vi.fn(() => ({ unmount })),
      closeRuntime: vi.fn(() => runtimeClose.promise),
    });
    const renderer = new DesktopRenderer(deps);
    await renderer.start();

    const first = renderer.dispose();
    const second = renderer.dispose();

    expect(second).toBe(first);
    expect(unmount).toHaveBeenCalledOnce();
    expect(stopWatching).toHaveBeenCalledOnce();
    expect(deps.closeRuntime).toHaveBeenCalledOnce();
    runtimeClose.resolve();
    await first;
  });
});
