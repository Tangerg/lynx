import { describe, expect, it } from "vitest";

import { loadDesktopBootstrap, type DesktopBinding } from "./desktopBridge";

describe("loadDesktopBootstrap", () => {
  it("accepts one exact local Runtime connection", async () => {
    const binding: DesktopBinding = {
      call: async (method) => {
        expect(method).toBe("main.DesktopHost.Bootstrap");
        return {
          runtime: {
            endpoint: "http://127.0.0.1:32123",
            localToken: "secret",
            instanceId: "ins_test",
            protocolVersion: "2026-08-21",
            idempotencyNamespace: "idp_test",
            generation: 2,
          },
        };
      },
    };
    await expect(loadDesktopBootstrap(binding)).resolves.toMatchObject({
      runtime: { endpoint: "http://127.0.0.1:32123", generation: 2 },
    });
  });

  it.each([
    { endpoint: "https://example.test", generation: 1 },
    { endpoint: "http://127.0.0.1:32123", generation: 0 },
    { endpoint: "http://127.0.0.1:32123", generation: 1, extension: true },
  ])("rejects an invalid host boundary %#", async (change) => {
    const binding: DesktopBinding = {
      call: async () => {
        const runtime: Record<string, unknown> = {
          endpoint: "http://127.0.0.1:32123",
          localToken: "secret",
          instanceId: "ins_test",
          protocolVersion: "2026-08-21",
          idempotencyNamespace: "idp_test",
          generation: 1,
        };
        Object.assign(runtime, change);
        return { runtime };
      },
    };
    await expect(loadDesktopBootstrap(binding)).rejects.toThrow(TypeError);
  });
});
