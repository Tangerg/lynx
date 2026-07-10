import { definePlugin } from "@/plugins/sdk";
import {
  installAgentDefaultSessionPort,
  installAgentRuntimeGateway,
  installAgentStatePorts,
} from "@/plugins/builtin/agent/public/statePorts";
import { startBootstrapLifecycle } from "./application/bootstrapLifecycle";
import { initFrontendObservability } from "./adapters/frontendObservability";
import { performRuntimeDiscovery } from "./adapters/runtimeDiscovery";

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
      discoverRuntime: performRuntimeDiscovery,
      reportObservabilityFailure: (err) => {
        console.warn("[bootstrap] observability init failed; running without telemetry:", err);
      },
      reportDiscoveryFailure: (err) => {
        console.warn("[bootstrap] runtime discovery failed; running degraded:", err);
      },
    });
  },
});
