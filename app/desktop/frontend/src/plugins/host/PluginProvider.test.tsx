import { render, screen } from "@testing-library/react";
import type { Host } from "dougong";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  disposeSideloads: vi.fn(),
  loadSideloadedPlugins: vi.fn(),
  publishHostBridge: vi.fn(),
  retractKernel: vi.fn(),
  startKernel: vi.fn(),
  stopKernel: vi.fn(),
}));

vi.mock("../builtin", () => ({ builtinPlugins: [] }));
vi.mock("../sdk", () => ({
  startKernel: mocks.startKernel,
  stopKernel: mocks.stopKernel,
}));
vi.mock("../sdk/kernel", () => ({ retractKernel: mocks.retractKernel }));
vi.mock("./hostBridge", () => ({ publishHostBridge: mocks.publishHostBridge }));
vi.mock("./sideloadDiscovery", () => ({
  loadSideloadedPlugins: mocks.loadSideloadedPlugins,
}));

import { PluginProvider } from "./PluginProvider";

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
  mocks.loadSideloadedPlugins.mockReset();
  mocks.disposeSideloads.mockReset().mockResolvedValue(undefined);
  mocks.loadSideloadedPlugins.mockReturnValue({
    completion: Promise.resolve(0),
    dispose: mocks.disposeSideloads,
  });
  mocks.publishHostBridge.mockReset();
  mocks.retractKernel.mockReset();
  mocks.startKernel.mockReset();
  mocks.stopKernel.mockReset().mockResolvedValue(undefined);
});

afterEach(() => vi.restoreAllMocks());

describe("PluginProvider kernel ownership", () => {
  it("stops the owned kernel when the provider unmounts", async () => {
    const owned = host();
    mocks.startKernel.mockResolvedValue(owned);
    const view = render(
      <PluginProvider>
        <div>workspace</div>
      </PluginProvider>,
    );
    await screen.findByText("workspace");

    view.unmount();

    expect(mocks.retractKernel).toHaveBeenCalledExactlyOnceWith(owned);
    await vi.waitFor(() => expect(mocks.disposeSideloads).toHaveBeenCalledOnce());
    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(owned));
  });

  it("stops a kernel whose startup settles after its provider was retired", async () => {
    const startup = deferred<Host>();
    const owned = host();
    mocks.startKernel.mockReturnValue(startup.promise);
    const view = render(
      <PluginProvider>
        <div>workspace</div>
      </PluginProvider>,
    );

    view.unmount();
    startup.resolve(owned);

    await vi.waitFor(() => expect(mocks.retractKernel).toHaveBeenCalledExactlyOnceWith(owned));
    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(owned));
    expect(mocks.loadSideloadedPlugins).not.toHaveBeenCalled();
  });

  it("still stops the Host when sideload disposal fails", async () => {
    const owned = host();
    mocks.startKernel.mockResolvedValue(owned);
    mocks.disposeSideloads.mockRejectedValue(new Error("platform disposal failed"));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const view = render(
      <PluginProvider>
        <div>workspace</div>
      </PluginProvider>,
    );
    await screen.findByText("workspace");

    view.unmount();

    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(owned));
    await vi.waitFor(() => expect(consoleError).toHaveBeenCalledOnce());
  });
});
