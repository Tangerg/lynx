import { DESKTOP_CLIENT_INFO } from "@/main/config";
import { PROTOCOL_VERSION } from "@/rpc";
import type { ClientCapabilities, RequestMeta } from "@/rpc";

// There is no list of renderable events: the runtime publishes every
// authoritative frame to whoever it accepted, and a client that could not fold
// them would be refused the run outright rather than served a shortened stream.
export const CLIENT_CAPABILITIES: ClientCapabilities = {
  features: {
    multimodal: { enabled: true },
    subagents: { enabled: true },
  },
  interruptTypes: ["approval", "question"],
};

export function runtimeRequestMeta(): RequestMeta {
  return {
    protocolVersion: PROTOCOL_VERSION,
    clientInfo: DESKTOP_CLIENT_INFO,
    clientCapabilities: CLIENT_CAPABILITIES,
  };
}
