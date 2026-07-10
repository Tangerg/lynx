import { describe, expect, it } from "vitest";
import { createLyraClient } from "./sdk";
import { createMemoryTransport } from "./transports/memory";
import { waitForRequest } from "./transports/memory.testkit";
import { JSONRPC_VERSION, type RpcMessage } from "./types";

describe("createLyraClient", () => {
  it("attaches request metadata to typed calls", async () => {
    const transport = createMemoryTransport();
    const client = createLyraClient(transport, {
      requestMeta: () => ({
        protocolVersion: "2026-06-07",
        clientInfo: { name: "test", version: "0" },
        clientCapabilities: { events: [], features: {}, interruptTypes: ["approval"] },
      }),
    });

    const promise = client.runtime.ping();
    const req = await waitForRequest(transport, "runtime.ping");

    expect(req.params).toMatchObject({
      _meta: {
        protocolVersion: "2026-06-07",
        clientCapabilities: { interruptTypes: ["approval"] },
      },
    });

    transport.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: {} } as RpcMessage);
    await promise;
    await client.close();
  });
});
