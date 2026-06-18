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
  it("info() returns flat JSON shape", async () => {
    const fetchStub = makeFetch(200, {
      serverInfo: { name: "lyra-core", version: "0.8.1" },
      protocolVersion: "2026-06-03",
      capabilities: { events: [], features: {}, providers: [] },
    });
    const client = createSidecarClient({ baseUrl: "http://x", fetch: fetchStub });
    const info = await client.info();
    expect(info.serverInfo.name).toBe("lyra-core");
    expect(info.protocolVersion).toBe("2026-06-03");
  });

  it("health() accepts 503 with body (unhealthy state)", async () => {
    const fetchStub = makeFetch(503, { ok: false });
    const client = createSidecarClient({ baseUrl: "http://x", fetch: fetchStub });
    const health = await client.health();
    expect(health.ok).toBe(false);
  });

  it("info() throws RpcTransportError on non-2xx (except 503)", async () => {
    const fetchStub = makeFetch(500, "internal error");
    const client = createSidecarClient({ baseUrl: "http://x", fetch: fetchStub });
    await expect(client.info()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("info() throws RpcTransportError on invalid JSON", async () => {
    const fetchStub = makeFetch(200, "<html>not json</html>");
    const client = createSidecarClient({ baseUrl: "http://x", fetch: fetchStub });
    await expect(client.info()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("strips trailing slash from baseUrl", async () => {
    const seen: string[] = [];
    const stub = vi.fn(async (url: string) => {
      seen.push(url);
      return new Response(JSON.stringify({ ok: true }), { status: 200 });
    });
    const client = createSidecarClient({
      baseUrl: "http://x/",
      fetch: stub as unknown as typeof fetch,
    });
    await client.health();
    expect(seen[0]).toBe("http://x/v2/health");
  });
});
