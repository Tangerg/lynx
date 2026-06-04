// Boot handshake (API.md §3 Lifecycle). On load: probe liveness via the
// sidecar, negotiate protocol version + capabilities via runtime.initialize,
// and stash the result in runtimeStore so feature/event gating works.
//
// Fire-and-forget + degrade on failure: a backend that hasn't implemented
// runtime.initialize yet MUST NOT block the app. Every capability selector
// reads false pre-handshake, and the UI already treats that as "feature
// off" — so a failed handshake just means no capability gating, not a
// broken app.

import { CLIENT_INFO, PROTOCOL_VERSION } from "@/main/config";
import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import type { ClientCapabilities } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";

// The StreamEvent types the reducer renders — declared so the server can
// avoid emitting events we'd drop (API.md §9). `interruptKinds` is the HITL
// switch: declaring approval / question tells the server we can render +
// answer them, so it won't strand an unresolvable open interrupt (§6.2).
const CLIENT_CAPABILITIES: ClientCapabilities = {
  events: [
    "run.started",
    "run.finished",
    "item.started",
    "item.delta",
    "item.completed",
    "state.snapshot",
    "state.delta",
    // The reducer routes `custom` StreamEvents to host.events.onCustom
    // handlers (third-party content blocks / preview-blocks). Declare it so a
    // spec-strict server (§9: "won't emit event types outside the negotiated
    // set") actually sends them.
    "custom",
  ],
  features: { multimodal: true },
  interruptKinds: ["approval", "question"],
};

async function handshake(): Promise<void> {
  const { sidecar, client } = getContainer();
  // Best-effort liveness probe; ignored if the sidecar isn't implemented.
  await sidecar.info().catch(() => undefined);
  const result = await client().runtime.initialize({
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
