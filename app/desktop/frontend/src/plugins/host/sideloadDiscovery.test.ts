import type { Host, Platform } from "dougong";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  bootstrap: vi.fn(),
  createSideloadPlatform: vi.fn(),
  registerSideloadedPlugin: vi.fn(),
}));

vi.mock("@/main/container", () => ({
  getContainer: () => ({ desktop: { bootstrap: mocks.bootstrap } }),
}));
vi.mock("../sdk/sideload", () => ({
  createSideloadPlatform: mocks.createSideloadPlatform,
  registerSideloadedPlugin: mocks.registerSideloadedPlugin,
  sideloadManifestSchema: { safeParse: vi.fn() },
}));

import { loadSideloadedPlugins } from "./sideloadDiscovery";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

function host(): Host {
  return {} as Host;
}

beforeEach(() => {
  mocks.bootstrap.mockReset();
  mocks.createSideloadPlatform.mockReset();
  mocks.registerSideloadedPlugin.mockReset();
});

afterEach(() => vi.restoreAllMocks());

describe("sideload discovery generation ownership", () => {
  it("does not create a platform when desktop discovery settles after disposal", async () => {
    const bootstrap = deferred<{ sideloadedPlugins: Array<{ id: string; source: string }> }>();
    mocks.bootstrap.mockReturnValue(bootstrap.promise);
    const discovery = loadSideloadedPlugins(host());

    await discovery.dispose();
    bootstrap.resolve({
      sideloadedPlugins: [{ id: "late", source: "export const manifest = {}" }],
    });

    await expect(discovery.completion).resolves.toBe(0);
    expect(mocks.createSideloadPlatform).not.toHaveBeenCalled();
    expect(mocks.registerSideloadedPlugin).not.toHaveBeenCalled();
  });

  it("binds and disposes the platform owned by the exact Host generation", async () => {
    const ownedHost = host();
    const dispose = vi.fn().mockResolvedValue(undefined);
    const platform = { dispose } as unknown as Platform<string | URL>;
    mocks.createSideloadPlatform.mockReturnValue(platform);
    mocks.bootstrap.mockResolvedValue({
      sideloadedPlugins: [{ id: "slow", source: "export const manifest = {}" }],
    });
    const revokeObjectURL = vi.spyOn(URL, "revokeObjectURL");
    const discovery = loadSideloadedPlugins(ownedHost);

    await vi.waitFor(() =>
      expect(mocks.createSideloadPlatform).toHaveBeenCalledWith(expect.any(Array), ownedHost),
    );
    await discovery.dispose();

    expect(dispose).toHaveBeenCalledOnce();
    await expect(discovery.completion).resolves.toBe(0);
    expect(revokeObjectURL).toHaveBeenCalledOnce();
    expect(mocks.registerSideloadedPlugin).not.toHaveBeenCalled();
  });
});
