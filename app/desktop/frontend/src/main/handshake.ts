// Runtime handshake (runtime.initialize) — extracted so BOTH the boot path
// (the bootstrap plugin) and the auto-recovery path (the rpc client's
// reinit-on-capability_not_negotiated decorator, see rpc/sdk.ts) run the exact
// same negotiation. Re-runnable by design: the backend re-negotiates
// idempotently, which is what lets a reconnect — or a restarted backend that
// lost our per-connection session — recover by simply re-handshaking. Deduped:
// a burst of business calls all hitting an un-handshaked backend share one
// in-flight initialize instead of firing N.

import { CLIENT_INFO, PROTOCOL_VERSION } from "@/main/config";
import type { ClientCapabilities, InitializeResponse, RpcClient } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";

// The StreamEvent types the reducer renders — declared so the server can avoid
// emitting events we'd drop (API.md §9). `interruptTypes` is the HITL switch:
// declaring approval / question tells the server we can render + answer them, so
// it won't strand an unresolvable open interrupt (§6.2).
export const CLIENT_CAPABILITIES: ClientCapabilities = {
  events: [
    "run.started",
    "run.progress",
    "run.finished",
    "item.started",
    "item.delta",
    "item.completed",
    "state.snapshot",
    "state.delta",
    // The reducer routes `custom` StreamEvents to host.events.onCustom handlers
    // (third-party content blocks / preview-blocks). Declare it so a spec-strict
    // server (§9: "won't emit event types outside the negotiated set") sends them.
    "custom",
  ],
  features: { multimodal: true },
  interruptTypes: ["approval", "question"],
};

let inFlight: Promise<void> | null = null;

/**
 * Negotiate runtime.initialize over `rpc` and stash the result in runtimeStore
 * so feature/event gating works. Deduped — overlapping callers await the same
 * handshake. Re-runnable: a backend restart drops our session (its dispatcher
 * resets to un-initialized), and re-running this re-establishes it.
 */
export function performHandshake(rpc: RpcClient): Promise<void> {
  if (inFlight) return inFlight;
  inFlight = (async () => {
    try {
      const result = await rpc.call<InitializeResponse>("runtime.initialize", {
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: CLIENT_INFO,
        capabilities: CLIENT_CAPABILITIES,
      });
      useRuntimeStore.getState().setHandshake(result.capabilities);
    } finally {
      inFlight = null;
    }
  })();
  return inFlight;
}
