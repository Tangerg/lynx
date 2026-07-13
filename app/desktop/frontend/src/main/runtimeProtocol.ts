import { CLIENT_INFO, PROTOCOL_VERSION } from "@/main/config";
import type { ClientCapabilities, RequestMeta } from "@/rpc";

export const CLIENT_CAPABILITIES: ClientCapabilities = {
  events: [
    "segment.started",
    "segment.progress",
    "segment.finished",
    "item.started",
    "item.delta",
    "item.completed",
    "state.snapshot",
    "state.delta",
    "custom",
  ],
  features: { multimodal: true },
  interruptTypes: ["approval", "question"],
};

export function runtimeRequestMeta(): RequestMeta {
  return {
    protocolVersion: PROTOCOL_VERSION,
    clientInfo: CLIENT_INFO,
    clientCapabilities: CLIENT_CAPABILITIES,
  };
}
