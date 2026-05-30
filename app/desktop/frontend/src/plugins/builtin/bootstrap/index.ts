// Boot handshake (INTEGRATION.md §2). On load: probe liveness via the
// sidecar, negotiate protocol version + capabilities via runtime.initialize,
// and stash the result in runtimeStore so feature/event gating works.
//
// Fire-and-forget + degrade on failure: a backend that hasn't implemented
// runtime.initialize yet (the current dev mock) MUST NOT block the app.
// Every capability selector reads false pre-handshake, and the UI already
// treats that as "feature off" — so a failed handshake just means no
// capability gating, not a broken app.

import { CLIENT_INFO, PROTOCOL_VERSION } from "@/main/config";
import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import { CUSTOM } from "@/protocol/agui/customEvents";
import type { ClientCapabilities } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";

// AG-UI standard events the reducer renders — declared so the server can
// avoid emitting events we'd drop (API.md §6.1 + §8.2). Custom events are
// derived from the CUSTOM constant so a newly-registered lyra.* handler is
// declared automatically (single source of truth).
const STANDARD_EVENTS = [
  "RUN_STARTED",
  "RUN_FINISHED",
  "RUN_ERROR",
  "STEP_STARTED",
  "STEP_FINISHED",
  "TEXT_MESSAGE_START",
  "TEXT_MESSAGE_CONTENT",
  "TEXT_MESSAGE_END",
  "TEXT_MESSAGE_CHUNK",
  "TOOL_CALL_START",
  "TOOL_CALL_ARGS",
  "TOOL_CALL_END",
  "TOOL_CALL_CHUNK",
  "TOOL_CALL_RESULT",
  "REASONING_MESSAGE_START",
  "REASONING_MESSAGE_CONTENT",
  "REASONING_MESSAGE_END",
  "THINKING_TEXT_MESSAGE_START",
  "THINKING_TEXT_MESSAGE_CONTENT",
  "THINKING_TEXT_MESSAGE_END",
  "STATE_SNAPSHOT",
  "STATE_DELTA",
  "MESSAGES_SNAPSHOT",
  "ACTIVITY_SNAPSHOT",
  "ACTIVITY_DELTA",
  "CUSTOM",
  "RAW",
];

const CLIENT_CAPABILITIES: ClientCapabilities = {
  events: {
    standard: STANDARD_EVENTS,
    // membership here is the HITL switch: declaring lyra.approval /
    // lyra.question tells the server we can render + answer them (§4.3).
    custom: Object.values(CUSTOM),
  },
  features: { multimodal: true, markdown: true },
};

async function handshake(): Promise<void> {
  const { sidecar, methods } = getContainer();
  // Best-effort liveness probe; ignored if the sidecar isn't implemented.
  await sidecar.info().catch(() => undefined);
  const result = await methods().runtime.initialize({
    protocolVersion: PROTOCOL_VERSION,
    clientInfo: CLIENT_INFO,
    capabilities: CLIENT_CAPABILITIES,
  });
  useRuntimeStore.getState().setHandshake(result);
}

export default definePlugin({
  name: "lyra.builtin.bootstrap",
  version: "1.0.0",
  setup() {
    void handshake().catch((err: unknown) => {
      console.warn("[bootstrap] runtime.initialize failed; running degraded:", err);
    });
  },
});
