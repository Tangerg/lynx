import { afterEach, describe, expect, it, vi } from "vitest";

import type { RuntimeConnection } from "@lyra/runtime-contract";

import { discoverRuntime } from "./runtimeQueries";

const connection: RuntimeConnection = {
  endpoint: "http://127.0.0.1:32123",
  localToken: "secret",
  instanceId: "ins_test",
  protocolVersion: "2026-08-21",
  idempotencyNamespace: "idp_test",
  generation: 1,
};

function discovery() {
  return {
    protocolVersion: "2026-08-21",
    serverInfo: {
      instanceId: "ins_test",
      name: "lyra-runtime",
      version: "dev",
      defaultWorkspace: { path: "/workspace" },
      home: "/home/test",
    },
    capabilities: {
      runEvents: [],
      runtimeTopics: [],
      streamingMethods: [],
      features: {},
      limits: {
        idempotency: { retentionSeconds: 86_400, namespace: "idp_test" },
        runReplay: {
          scope: "runtimeInstanceRootSegment",
          maxEvents: 10_000,
          maxBytes: 67_108_864,
        },
        mcpAuthorizationAttempts: { retentionSeconds: 600 },
        runtimeSubscription: { maxTopics: 64, maxWatches: 256 },
      },
    },
  };
}

afterEach(() => vi.unstubAllGlobals());

describe("discoverRuntime", () => {
  it("uses the generated client and authenticates one exact discovery request", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: URL | RequestInfo, init?: RequestInit) => {
        expect(String(input)).toBe("http://127.0.0.1:32123/v2/rpc");
        expect(new Headers(init?.headers).get("Authorization")).toBe(
          "Bearer secret",
        );
        const request = JSON.parse(String(init?.body)) as Record<
          string,
          unknown
        >;
        expect(request).toMatchObject({
          jsonrpc: "2.0",
          method: "runtime.discover",
        });
        expect(request.params).toMatchObject({
          _meta: {
            protocolVersion: "2026-08-21",
            clientInfo: { name: "lyra-desktop-app2", version: "0.0.0" },
          },
        });
        return Response.json({
          jsonrpc: "2.0",
          id: request.id,
          result: discovery(),
        });
      }),
    );

    await expect(discoverRuntime(connection)).resolves.toMatchObject({
      serverInfo: { instanceId: "ins_test" },
    });
  });

  it("fails closed when the Runtime identity changes", async () => {
    const changed = discovery();
    changed.serverInfo.instanceId = "ins_other";
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: URL | RequestInfo, init?: RequestInit) => {
        const request = JSON.parse(String(init?.body)) as Record<
          string,
          unknown
        >;
        return Response.json({
          jsonrpc: "2.0",
          id: request.id,
          result: changed,
        });
      }),
    );

    await expect(discoverRuntime(connection)).rejects.toThrow(
      "Runtime identity changed during discovery",
    );
  });
});
