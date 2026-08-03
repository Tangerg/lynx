import { describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createDesktopHostClient } from "./desktopHost";

function hostBinding() {
  return {
    Bootstrap: vi.fn(async () => ({
      localRuntime: { endpoint: "http://127.0.0.1:17171" },
      sideloadedPlugins: [],
      sideloadIssues: [],
    })),
    WindowChrome: vi.fn(async () => ({
      controlsCentreY: 20,
      controlsInlineEnd: 72,
      measured: true,
    })),
  };
}

describe("DesktopHostClient", () => {
  it("validates and caches the Wails bootstrap result", async () => {
    const binding = {
      ...hostBinding(),
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
      ...hostBinding(),
      Bootstrap: async () => ({ localRuntime: {} }),
    });
    await expect(client.bootstrap()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("reads the platform's window-control geometry", async () => {
    await expect(createDesktopHostClient(hostBinding()).windowChrome()).resolves.toEqual({
      controlsCentreY: 20,
      controlsInlineEnd: 72,
    });
  });

  // Every way of having no geometry has to arrive as the same `null`, because the
  // caller's only correct response is to leave the stylesheet's own numbers standing.
  // A zero or a partial object reaching the layout would collapse the gutter that
  // holds the window's controls clear of the header.
  it("answers null for every way of having nothing to measure", async () => {
    await expect(createDesktopHostClient(undefined).windowChrome()).resolves.toBeNull();

    const unmeasured = {
      ...hostBinding(),
      WindowChrome: async () => ({ controlsCentreY: 0, controlsInlineEnd: 0, measured: false }),
    };
    await expect(createDesktopHostClient(unmeasured).windowChrome()).resolves.toBeNull();

    const malformed = { ...hostBinding(), WindowChrome: async () => ({ controlsCentreY: 20 }) };
    await expect(createDesktopHostClient(malformed).windowChrome()).resolves.toBeNull();

    const broken = {
      ...hostBinding(),
      WindowChrome: async () => {
        throw new Error("no window");
      },
    };
    await expect(createDesktopHostClient(broken).windowChrome()).resolves.toBeNull();
  });
});
