import { describe, expect, it, vi } from "vitest";
import { createLyraClient } from "./sdk";
import { createMemoryTransport } from "./transports/memory";
import { waitForRequest } from "./transports/memory.testkit";
import { JSONRPC_VERSION, type RpcMessage } from "./types";
import { PROTOCOL_VERSION, type ServerCapabilities } from "./wire.generated";
import discoverResponse from "./samples/method.discover.resp.json";

describe("createLyraClient", () => {
  it("disposes journal ownership before closing the transport", async () => {
    const transport = createMemoryTransport();
    const dispose = vi.fn();
    const client = createLyraClient(transport, {
      mutationJournal: { reserve: () => undefined, dispose },
    });

    await client.close();

    expect(dispose).toHaveBeenCalledOnce();
  });

  it("attaches request metadata to typed calls", async () => {
    const transport = createMemoryTransport();
    const client = createLyraClient(transport, {
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "test", version: "0" },
        clientCapabilities: { events: [], features: {}, interruptTypes: ["approval"] },
      }),
    });

    expect("rpc" in client).toBe(false);

    const promise = client.runtime.discover();
    const req = await waitForRequest(transport, "runtime.discover");

    expect(req.params).toMatchObject({
      _meta: {
        protocolVersion: PROTOCOL_VERSION,
        clientCapabilities: { interruptTypes: ["approval"] },
      },
    });

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: discoverResponse,
    } as RpcMessage);
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
        idempotency: { namespace: "idp_test", retentionSeconds: 86_400 },
        runReplay: { scope: "runtimeInstanceRootSegment", maxEvents: 1, maxBytes: 1 },
        mcpAuthorizationAttempts: { retentionSeconds: 600 },
        runtimeSubscription: { maxTopics: 1, maxWatches: 1 },
      },
    } satisfies ServerCapabilities;
    const client = createLyraClient(transport, {
      capabilities: () => capabilities,
      requestMeta: () => {
        reads += 1;
        return {
          protocolVersion: PROTOCOL_VERSION,
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
