// In-memory Transport for unit tests. Backed by a push-pull async channel
// (`channel.ts`) for the inbound side; `inject()` is just a thin alias
// for `channel.push()`. Outbound side stores messages in an array that
// tests inspect via `outbox()`.

import { createPushPullChannel } from "../channel";
import type {
  Transport,
  TransportEvent,
  TransportRequest,
  TransportResponseMetadata,
} from "../transport";
import type { WireStreamingMethodName } from "../wire.methods.generated";
import type { RpcMessage } from "../types";

export interface MemoryTransport extends Transport {
  /** Push a message as if it arrived from the runtime. */
  inject(msg: RpcMessage, metadata?: TransportResponseMetadata): void;
  /** End one streaming response as if reported by the transport. */
  endStream(method: WireStreamingMethodName, runIds?: readonly string[]): void;
  /** Drain all messages the client has sent so far. */
  outbox(): TransportRequest[];
}

export function createMemoryTransport(): MemoryTransport {
  const sent: TransportRequest[] = [];
  const channel = createPushPullChannel<TransportEvent>();

  return {
    async send(msg) {
      if (channel.closed) throw new Error("transport closed");
      sent.push(msg);
    },
    recv: () => channel.iterator(),
    async close() {
      channel.close();
    },
    inject(msg, metadata) {
      if (channel.closed) throw new Error("transport closed");
      channel.push({ type: "message", message: msg, metadata });
    },
    endStream(method, runIds = []) {
      if (channel.closed) throw new Error("transport closed");
      channel.push({ type: "streamEnd", method, runIds });
    },
    outbox() {
      return [...sent];
    },
  };
}
