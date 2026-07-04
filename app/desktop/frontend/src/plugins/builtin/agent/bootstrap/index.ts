import { definePlugin } from "@/plugins/sdk";
import {
  installAgentDefaultSessionPort,
  installAgentRuntimeGateway,
  installAgentStatePorts,
} from "@/plugins/builtin/agent/public/statePorts";
import { startBootstrapLifecycle } from "./application/bootstrapLifecycle";
import { initFrontendObservability } from "./adapters/frontendObservability";
import { performRuntimeHandshake } from "./adapters/runtimeHandshake";

export default definePlugin({
  name: "lyra.builtin.bootstrap",
  version: "1.0.0",
  setup() {
    return startBootstrapLifecycle({
      installPorts: () => {
        installAgentStatePorts();
        installAgentDefaultSessionPort();
        installAgentRuntimeGateway();
      },
      initObservability: initFrontendObservability,
      performHandshake: performRuntimeHandshake,
      reportObservabilityFailure: (err) => {
        console.warn("[bootstrap] observability init failed; running without telemetry:", err);
      },
      reportHandshakeFailure: (err) => {
        console.warn("[bootstrap] runtime.initialize failed; running degraded:", err);
      },
    });
  },
});
