import { describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createSidecarClient } from "./sidecar";

function makeFetch(status: number, body: unknown): typeof fetch {
  const stub = vi.fn(
    async () => new Response(typeof body === "string" ? body : JSON.stringify(body), { status }),
  );
  return stub as unknown as typeof fetch;
}

describe("SidecarClient", () => {
  it("info() returns public bootstrap metadata", async () => {
    const fetchStub = makeFetch(200, {
      protocol: { current: "2026-07-19", minSupported: "2026-07-19" },
      server: { name: "lyra-core", version: "0.8.1" },
      transport: "http",
      endpoints: {
        rpc: "/v2/rpc",
        info: "/v2/info",
        liveness: "/v2/health/live",
        readiness: "/v2/health/ready",
      },
    });
    const client = createSidecarClient({ baseUrl: "http://x", fetch: fetchStub });
    const info = await client.info();
    expect(info.server.name).toBe("lyra-core");
    expect(info.protocol.current).toBe("2026-07-19");
  });

  it("readiness() accepts 503 with its diagnostic body", async () => {
    const fetchStub = makeFetch(503, { status: "unhealthy", checks: { storage: "unhealthy" } });
    const client = createSidecarClient({ baseUrl: "http://x", fetch: fetchStub });
    await expect(client.readiness()).resolves.toMatchObject({
      status: "unhealthy",
      checks: { storage: "unhealthy" },
    });
  });

  it("info() throws RpcTransportError on non-2xx", async () => {
    const client = createSidecarClient({ baseUrl: "http://x", fetch: makeFetch(500, "error") });
    await expect(client.info()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("preserves problem details on sidecar failures", async () => {
    const client = createSidecarClient({
      baseUrl: "http://x",
      fetch: makeFetch(500, {
        type: "urn:lyra:transport:internal_error",
        detail: "probe registry unavailable",
        requestId: "req_123",
      }),
    });
    await expect(client.info()).rejects.toMatchObject({
      status: 500,
      requestId: "req_123",
      problemType: "urn:lyra:transport:internal_error",
      message: expect.stringContaining("probe registry unavailable"),
    } satisfies Partial<RpcTransportError>);
  });

  it("uses distinct liveness and readiness endpoints", async () => {
    const seen: string[] = [];
    const stub = vi.fn(async (url: string) => {
      seen.push(url);
      return new Response(JSON.stringify({ status: "ok" }), { status: 200 });
    });
    const client = createSidecarClient({
      baseUrl: "http://x/",
      fetch: stub as unknown as typeof fetch,
    });
    await client.liveness();
    await client.readiness();
    expect(seen).toEqual(["http://x/v2/health/live", "http://x/v2/health/ready"]);
  });
});
