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

  it("still closes the transport when journal ownership cleanup fails", async () => {
    const transport = createMemoryTransport();
    const closeTransport = vi.spyOn(transport, "close");
    const failure = new Error("journal cleanup failed");
    const client = createLyraClient(transport, {
      mutationJournal: {
        reserve: () => undefined,
        dispose: () => {
          throw failure;
        },
      },
    });

    await expect(client.close()).rejects.toBe(failure);
    expect(closeTransport).toHaveBeenCalledOnce();
  });

  it("preserves both journal and transport cleanup failures", async () => {
    const transport = createMemoryTransport();
    const journalFailure = new Error("journal cleanup failed");
    const transportFailure = new Error("transport cleanup failed");
    const closeMemoryTransport = transport.close.bind(transport);
    vi.spyOn(transport, "close").mockImplementation(async () => {
      await closeMemoryTransport();
      throw transportFailure;
    });
    const client = createLyraClient(transport, {
      mutationJournal: {
        reserve: () => undefined,
        dispose: () => {
          throw journalFailure;
        },
      },
    });

    const failure = await client.close().catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(AggregateError);
    expect((failure as AggregateError).errors).toEqual([journalFailure, transportFailure]);
  });

  it("shares one cleanup settlement across concurrent close callers", async () => {
    const transport = createMemoryTransport();
    const journalFailure = new Error("journal cleanup failed");
    const transportFailure = new Error("transport cleanup failed");
    let rejectTransport!: (error: unknown) => void;
    const closeMemoryTransport = transport.close.bind(transport);
    const closeTransport = vi.spyOn(transport, "close").mockImplementation(async () => {
      await closeMemoryTransport();
      await new Promise<void>((_resolve, reject) => {
        rejectTransport = reject;
      });
    });
    let disposed = false;
    const dispose = vi.fn(() => {
      if (disposed) return;
      disposed = true;
      throw journalFailure;
    });
    const client = createLyraClient(transport, {
      mutationJournal: { reserve: () => undefined, dispose },
    });

    const first = client.close();
    const second = client.close();
    expect(dispose).toHaveBeenCalledOnce();
    expect(closeTransport).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(rejectTransport).toBeTypeOf("function"));
    rejectTransport(transportFailure);

    const [firstResult, secondResult] = await Promise.allSettled([first, second]);
    if (firstResult.status !== "rejected" || secondResult.status !== "rejected") {
      throw new Error("concurrent close unexpectedly resolved");
    }
    expect(secondResult.reason).toBe(firstResult.reason);
    expect((firstResult.reason as AggregateError).errors).toEqual([
      journalFailure,
      transportFailure,
    ]);
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
      streamingMethods: [],
      features: {
        subagents: {
          enabled: true,
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
