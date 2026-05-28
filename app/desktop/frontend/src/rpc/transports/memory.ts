// In-memory Transport for unit tests + future InProcess (Bubble Tea
// would use a Go equivalent). Backed by a push-pull async channel
// (`channel.ts`) for the inbound side; `inject()` is just a thin alias
// for `channel.push()`. Outbound side stores messages in an array that
// tests inspect via `outbox()`.

import { createPushPullChannel } from "../channel";
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
  const channel = createPushPullChannel<RpcMessage>();

  return {
    async send(msg) {
      if (channel.closed) throw new Error("transport closed");
      sent.push(msg);
    },
    recv: () => channel.iterator(),
    async close() {
      channel.close();
    },
    inject(msg) {
      if (channel.closed) throw new Error("transport closed");
      channel.push(msg);
    },
    outbox() {
      return [...sent];
    },
  };
}
