import { describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createShellClient } from "./shell";

function makeFetch(status: number, body: unknown): typeof fetch {
  const stub = vi.fn(
    async () => new Response(typeof body === "string" ? body : JSON.stringify(body), { status }),
  );
  return stub as unknown as typeof fetch;
}

describe("ShellClient.sideloadManifest", () => {
  it("returns the validated entry list", async () => {
    const fetchStub = makeFetch(200, [
      { id: "acme.tools", url: "/plugins/acme/index.js" },
      { id: "acme.theme", url: "/plugins/theme/index.js" },
    ]);
    const client = createShellClient({ baseUrl: "http://x", fetch: fetchStub });
    const entries = await client.sideloadManifest();
    expect(entries).toEqual([
      { id: "acme.tools", url: "/plugins/acme/index.js" },
      { id: "acme.theme", url: "/plugins/theme/index.js" },
    ]);
  });

  it("hits ${baseUrl}/plugins and strips a trailing slash", async () => {
    const seen: string[] = [];
    const stub = vi.fn(async (url: string) => {
      seen.push(url);
      return new Response("[]", { status: 200 });
    });
    const client = createShellClient({
      baseUrl: "http://x/",
      fetch: stub as unknown as typeof fetch,
    });
    await client.sideloadManifest();
    expect(seen[0]).toBe("http://x/plugins");
  });

  it("throws RpcTransportError on non-2xx", async () => {
    const client = createShellClient({ baseUrl: "http://x", fetch: makeFetch(500, "boom") });
    await expect(client.sideloadManifest()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("throws RpcTransportError on invalid JSON", async () => {
    const client = createShellClient({ baseUrl: "http://x", fetch: makeFetch(200, "<html>") });
    await expect(client.sideloadManifest()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("rejects a malformed manifest (entries missing id/url) at the trust boundary", async () => {
    const client = createShellClient({
      baseUrl: "http://x",
      fetch: makeFetch(200, [{ id: "ok", url: "/a.js" }, { name: "no-id-or-url" }]),
    });
    await expect(client.sideloadManifest()).rejects.toBeInstanceOf(RpcTransportError);
  });
});
