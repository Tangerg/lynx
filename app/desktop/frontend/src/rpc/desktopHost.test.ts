import { describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createDesktopHostClient } from "./desktopHost";

function windowBinding() {
  return {
    Bootstrap: vi.fn(async () => ({
      localRuntime: { endpoint: "http://127.0.0.1:17171" },
      sideloadedPlugins: [],
      sideloadIssues: [],
    })),
    MinimiseWindow: vi.fn(async () => {}),
    ToggleMaximiseWindow: vi.fn(async () => {}),
    CloseWindow: vi.fn(async () => {}),
  };
}

describe("DesktopHostClient", () => {
  it("validates and caches the Wails bootstrap result", async () => {
    const binding = {
      ...windowBinding(),
      Bootstrap: vi.fn(async () => ({
        localRuntime: { endpoint: "http://127.0.0.1:17171", localToken: "token" },
        sideloadedPlugins: [{ id: "acme.tools", source: "export default {};" }],
        sideloadIssues: [],
      })),
    };
    const client = createDesktopHostClient(binding);

    await expect(client.bootstrap()).resolves.toMatchObject({
      localRuntime: { localToken: "token" },
      sideloadedPlugins: [{ id: "acme.tools" }],
    });
    await client.bootstrap();
    expect(binding.Bootstrap).toHaveBeenCalledTimes(1);
  });

  it("returns null outside Wails", async () => {
    await expect(createDesktopHostClient(undefined).bootstrap()).resolves.toBeNull();
  });

  it("rejects a malformed host response", async () => {
    const client = createDesktopHostClient({
      ...windowBinding(),
      Bootstrap: async () => ({ localRuntime: {} }),
    });
    await expect(client.bootstrap()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("forwards each window command to the host", () => {
    const binding = windowBinding();
    const client = createDesktopHostClient(binding);

    client.minimiseWindow();
    client.toggleMaximiseWindow();
    client.closeWindow();

    expect(binding.MinimiseWindow).toHaveBeenCalledTimes(1);
    expect(binding.ToggleMaximiseWindow).toHaveBeenCalledTimes(1);
    expect(binding.CloseWindow).toHaveBeenCalledTimes(1);
  });

  it("stays silent when there is no window to command", () => {
    const client = createDesktopHostClient(undefined);
    expect(() => {
      client.minimiseWindow();
      client.toggleMaximiseWindow();
      client.closeWindow();
    }).not.toThrow();
  });
});
