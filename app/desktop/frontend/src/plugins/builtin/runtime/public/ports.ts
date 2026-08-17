// The Runtime context's setup-time contract — see `agent/public/ports` for why
// only setup-time readers need one.

import { service } from "dougong";

export interface RuntimeStreamPorts {
  connectionGeneration: () => string | null;
  subscribeConnection: (onChange: () => void) => () => void;
  reportConnectionLoss: (expectedGeneration: string) => Promise<void>;
}

export const RUNTIME_STREAM_PORTS = service<RuntimeStreamPorts>("lyra.runtime.streamPorts");
