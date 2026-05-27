// In-memory Transport for unit tests + future InProcess (Bubble Tea
// would use a Go equivalent). Two queues: sent (outbound from client)
// and recv (inbound to client). Test harnesses drive both sides by
// pushing messages via `inject()` and reading via `outbox()`.

import type { Transport } from "../transport";
import type { RpcMessage } from "../types";

export interface MemoryTransport extends Transport {
  /** Push a message as if it arrived from the runtime. */
  inject(msg: RpcMessage): void;
  /** Drain all messages the client has sent so far. */
  outbox(): RpcMessage[];
}

export function createMemoryTransport(): MemoryTransport {
  const sent: RpcMessage[] = [];
  let waiters: Array<(msg: RpcMessage | typeof DONE) => void> = [];
  const pending: RpcMessage[] = [];
  let closed = false;

  // Sentinel for end-of-stream (channel close).
  const DONE = Symbol("memory-transport-done");
  type Sentinel = typeof DONE;

  function dispatch(msg: RpcMessage | Sentinel): void {
    if (waiters.length > 0) {
      const next = waiters.shift()!;
      next(msg);
    } else if (msg !== DONE) {
      pending.push(msg);
    }
  }

  async function* recv(): AsyncIterable<RpcMessage> {
    while (!closed || pending.length > 0) {
      const buffered = pending.shift();
      if (buffered !== undefined) {
        yield buffered;
        continue;
      }
      const next = await new Promise<RpcMessage | Sentinel>((resolve) => {
        waiters.push(resolve);
      });
      if (next === DONE) return;
      yield next;
    }
  }

  return {
    async send(msg) {
      if (closed) throw new Error("transport closed");
      sent.push(msg);
    },
    recv,
    async close() {
      closed = true;
      // Wake any blocked recv() with the sentinel.
      const pendingWaiters = waiters;
      waiters = [];
      for (const w of pendingWaiters) w(DONE);
    },
    inject(msg) {
      if (closed) throw new Error("transport closed");
      dispatch(msg);
    },
    outbox() {
      return [...sent];
    },
  };
}
