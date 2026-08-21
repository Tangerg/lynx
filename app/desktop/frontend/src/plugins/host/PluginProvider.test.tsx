import { render, screen } from "@testing-library/react";
import type { Host } from "dougong";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  startKernel: vi.fn(),
  stopKernel: vi.fn(),
}));

vi.mock("../builtin", () => ({ builtinPlugins: [] }));
vi.mock("../sdk", () => ({
  startKernel: mocks.startKernel,
  stopKernel: mocks.stopKernel,
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

    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(owned));
  });
});
