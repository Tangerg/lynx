// Boot handshake (API.md §3 Lifecycle). On load: probe liveness via the
// sidecar, negotiate protocol version + capabilities via runtime.initialize,
// and stash the result in runtimeStore so feature/event gating works.
//
// Fire-and-forget + degrade on failure: a backend that hasn't implemented
// runtime.initialize yet MUST NOT block the app. Every capability selector
// reads false pre-handshake, and the UI already treats that as "feature
// off" — so a failed handshake just means no capability gating, not a
// broken app.

import { CLIENT_INFO } from "@/main/config";
import { getContainer } from "@/main/container";
import { performHandshake } from "@/main/handshake";
import { definePlugin } from "@/plugins/sdk";
import { getConfig } from "@/plugins/sdk/config";
import { installAgentStatePorts } from "../adapters/agentStatePorts";

async function handshake(): Promise<void> {
  const { sidecar, client } = getContainer();
  // Best-effort liveness probe; ignored if the sidecar isn't implemented.
  await sidecar.info().catch(() => undefined);
  // The negotiation itself lives in main/handshake so the auto-recovery path
  // (rpc reinit decorator) re-runs the exact same thing on a lost session.
  await performHandshake(client().rpc);
}

// Install the OTel triad once, early + always-on (mirror of the backend's
// setupObservability at process start). Heavy SDK is behind the dynamic
// import so it stays off the first-paint path; setup degrades to no-op
// providers if it fails. OTLP export is opt-in via the `otel.endpoint`
// config key (the production swap); unset → local Diagnostics sink only.
async function initObservability(): Promise<() => Promise<void>> {
  const { setupObservability, teardownObservability } = await import("@/lib/observability/setup");
  await setupObservability({
    serviceName: "lyra-frontend",
    serviceVersion: CLIENT_INFO.version,
    otlpEndpoint: getConfig<string>("otel.endpoint") ?? undefined,
  });
  return teardownObservability;
}

export default definePlugin({
  name: "lyra.builtin.bootstrap",
  version: "1.0.0",
  setup() {
    installAgentStatePorts();
    let teardown: (() => Promise<void>) | null = null;
    void initObservability()
      .then((fn) => {
        teardown = fn;
      })
      .catch((err: unknown) => {
        console.warn("[bootstrap] observability init failed; running without telemetry:", err);
      });
    void handshake().catch((err: unknown) => {
      console.warn("[bootstrap] runtime.initialize failed; running degraded:", err);
    });
    return () => {
      void teardown?.();
    };
  },
});
