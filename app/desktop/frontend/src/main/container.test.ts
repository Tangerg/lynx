import { afterEach, describe, expect, it, vi } from "vitest";
import type { DesktopHostClient, LyraClient } from "@/rpc";
import {
  disposeContainer,
  getContainer,
  initializeDesktopHost,
  resetContainer,
  setContainer,
} from "./container";

describe("main/container", () => {
  afterEach(resetContainer);

  it("exposes the Runtime Protocol entry points out of the box", () => {
    const c = getContainer();
    expect(typeof c.client).toBe("function");
    expect(typeof c.sidecar).toBe("function");
    expect(c.desktop).toBeDefined();
  });

  it("setContainer() swaps a single slot, leaving others intact", () => {
    const fake = {} as LyraClient;
    const before = getContainer().desktop;
    setContainer({ client: () => fake });
    expect(getContainer().client()).toBe(fake);
    expect(getContainer().desktop).toBe(before);
  });

  it("resetContainer() restores defaults", async () => {
    const fake = {} as LyraClient;
    setContainer({ client: () => fake });
    await resetContainer();
    expect(getContainer().client()).not.toBe(fake);
  });

  it("client() returns a cached singleton and reset joins its teardown", async () => {
    const first = getContainer().client();
    expect(getContainer().client()).toBe(first);
    const closeFirst = vi.spyOn(first, "close");

    await resetContainer();

    expect(closeFirst).toHaveBeenCalledOnce();
    expect(getContainer().client()).not.toBe(first);
  });

  it("joins the owned client during final application teardown", async () => {
    const client = getContainer().client();
    const close = vi.spyOn(client, "close");

    await disposeContainer();

    expect(close).toHaveBeenCalledOnce();
  });

  it("does not resurrect Runtime owners after final application teardown starts", async () => {
    const owner = getContainer();
    owner.client();
    owner.sidecar();

    const closing = disposeContainer();

    expect(disposeContainer()).toBe(closing);
    expect(() => owner.client()).toThrow("Desktop container is closed");
    expect(() => owner.sidecar()).toThrow("Desktop container is closed");
    await closing;
    expect(() => owner.client()).toThrow("Desktop container is closed");
    expect(() => owner.sidecar()).toThrow("Desktop container is closed");
  });

  it("does not close a client injected by an external owner", async () => {
    const close = vi.fn(async () => {});
    const external = { close } as unknown as LyraClient;
    setContainer({ client: () => external });

    await disposeContainer();

    expect(close).not.toHaveBeenCalled();
  });

  it("closes and rebuilds the shared client after a local token hot swap", async () => {
    const desktop = (localToken: string): DesktopHostClient => ({
      bootstrap: async () => ({
        localRuntime: { endpoint: "http://127.0.0.1:17171", localToken },
        sideloadedPlugins: [],
        sideloadIssues: [],
      }),
      chooseWorkingDirectory: async () => null,
      windowChrome: async () => null,
    });
    setContainer({ desktop: desktop("token-a") });
    await initializeDesktopHost();
    const first = getContainer().client();
    const closeFirst = vi.spyOn(first, "close");

    setContainer({ desktop: desktop("token-b") });
    await initializeDesktopHost();
    const second = getContainer().client();

    expect(second).not.toBe(first);
    expect(closeFirst).toHaveBeenCalledOnce();
    expect(getContainer().client()).toBe(second);
  });

  it("sidecar() returns a cached client for the active endpoint", async () => {
    const first = getContainer().sidecar();
    expect(getContainer().sidecar()).toBe(first);
    await resetContainer();
    expect(getContainer().sidecar()).not.toBe(first);
  });
});
