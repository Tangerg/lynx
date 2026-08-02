import { describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createDesktopHostClient } from "./desktopHost";

describe("DesktopHostClient", () => {
  it("validates and caches the Wails bootstrap result", async () => {
    const Bootstrap = vi.fn(async () => ({
      localRuntime: { endpoint: "http://127.0.0.1:17171", localToken: "token" },
      sideloadedPlugins: [{ id: "acme.tools", source: "export default {};" }],
      sideloadIssues: [],
    }));
    const client = createDesktopHostClient({ Bootstrap });

    await expect(client.bootstrap()).resolves.toMatchObject({
      localRuntime: { localToken: "token" },
      sideloadedPlugins: [{ id: "acme.tools" }],
    });
    await client.bootstrap();
    expect(Bootstrap).toHaveBeenCalledTimes(1);
  });

  it("returns null outside Wails", async () => {
    await expect(createDesktopHostClient(undefined).bootstrap()).resolves.toBeNull();
  });

  it("rejects a malformed host response", async () => {
    const client = createDesktopHostClient({ Bootstrap: async () => ({ localRuntime: {} }) });
    await expect(client.bootstrap()).rejects.toBeInstanceOf(RpcTransportError);
  });
});
