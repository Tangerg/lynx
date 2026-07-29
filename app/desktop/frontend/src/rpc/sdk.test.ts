import { describe, expect, it } from "vitest";
import { createLyraClient } from "./sdk";
import { createMemoryTransport } from "./transports/memory";
import { waitForRequest } from "./transports/memory.testkit";
import { JSONRPC_VERSION, type RpcMessage } from "./types";
import type { ServerCapabilities } from "./wire.generated";

describe("createLyraClient", () => {
  it("attaches request metadata to typed calls", async () => {
    const transport = createMemoryTransport();
    const client = createLyraClient(transport, {
      requestMeta: () => ({
        protocolVersion: "2026-07-19",
        clientInfo: { name: "test", version: "0" },
        clientCapabilities: { events: [], features: {}, interruptTypes: ["approval"] },
      }),
    });

    const promise = client.runtime.discover();
    const req = await waitForRequest(transport, "runtime.discover");

    expect(req.params).toMatchObject({
      _meta: {
        protocolVersion: "2026-07-19",
        clientCapabilities: { interruptTypes: ["approval"] },
      },
    });

    transport.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: {} } as RpcMessage);
    await promise;
    await client.close();
  });

  it("preflights and emits the same request metadata snapshot", async () => {
    const transport = createMemoryTransport();
    let reads = 0;
    const capabilities = {
      runEvents: [],
      runtimeTopics: [],
      stateSnapshots: [],
      streamingMethods: [],
      features: {
        subagents: {
          enabled: true,
          stability: "experimental",
          clientOptIn: true,
          requiredByRunProtocol: true,
        },
      },
      limits: {
        runReplay: { scope: "processRootSegment", maxEvents: 1, maxBytes: 1 },
        runtimeSubscription: { maxTopics: 1, maxWatches: 1 },
      },
    } satisfies ServerCapabilities;
    const client = createLyraClient(transport, {
      capabilities: () => capabilities,
      requestMeta: () => {
        reads += 1;
        return {
          protocolVersion: "2026-07-19",
          clientInfo: { name: "test", version: "0" },
          clientCapabilities: {
            features: { subagents: { enabled: reads === 1 } },
          },
        };
      },
    });

    const promise = client.runs.list({ includeDescendants: true });
    const req = await waitForRequest(transport, "runs.list");
    expect(reads).toBe(1);
    expect(req.params).toMatchObject({
      _meta: {
        clientCapabilities: {
          features: { subagents: { enabled: true } },
        },
      },
    });

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { data: [] },
    } as RpcMessage);
    await promise;
    await client.close();
  });
});
