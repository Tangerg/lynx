import { describe, expect, it } from "vitest";
import { JSONRPC_VERSION } from "../types";
import { createMemoryTransport } from "./memory";

describe("MemoryTransport", () => {
  it("captures sent messages in outbox", async () => {
    const t = createMemoryTransport();
    await t.send({ jsonrpc: JSONRPC_VERSION, id: 1, method: "runtime.ping" });
    expect(t.outbox()).toHaveLength(1);
    expect((t.outbox()[0] as { method: string }).method).toBe("runtime.ping");
  });

  it("injected messages flow through recv()", async () => {
    const t = createMemoryTransport();
    const iter = t.recv()[Symbol.asyncIterator]();

    t.inject({ jsonrpc: JSONRPC_VERSION, method: "notifications/run/event", params: { n: 1 } });

    const next = await iter.next();
    expect(next.done).toBe(false);
    expect((next.value as { params: { n: number } }).params.n).toBe(1);
  });

  it("recv yields buffered messages even when readers arrive late", async () => {
    const t = createMemoryTransport();
    t.inject({ jsonrpc: JSONRPC_VERSION, method: "n1" });
    t.inject({ jsonrpc: JSONRPC_VERSION, method: "n2" });

    const iter = t.recv()[Symbol.asyncIterator]();
    const a = await iter.next();
    const b = await iter.next();
    expect((a.value as { method: string }).method).toBe("n1");
    expect((b.value as { method: string }).method).toBe("n2");
  });

  it("close terminates recv iterator and rejects further send", async () => {
    const t = createMemoryTransport();
    const iter = t.recv()[Symbol.asyncIterator]();
    await t.close();
    const result = await iter.next();
    expect(result.done).toBe(true);
    await expect(t.send({ jsonrpc: JSONRPC_VERSION, method: "x" })).rejects.toThrow();
  });
});
