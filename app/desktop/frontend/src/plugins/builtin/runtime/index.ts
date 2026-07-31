import { definePlugin } from "@/plugins/sdk";
import { runtimeCapabilities } from "./application/ports/capabilities";
import { discoverRuntime } from "./application/discoverRuntime";
import { installRuntimeEndpointConfiguration } from "./adapters/runtimeEndpointConfiguration";
import { installRuntimeCapabilityPort } from "./adapters/runtimeCapabilityStore";
import { runtimeDiscovery } from "./adapters/runtimeDiscovery";

export default definePlugin({
  name: "lyra.builtin.runtime",
  version: "1.0.0",
  setup({ host }) {
    const disposeEndpoint = installRuntimeEndpointConfiguration(host);
    const disposeCapabilities = installRuntimeCapabilityPort();
    const capabilities = runtimeCapabilities();
    capabilities.clear();
    let active = true;

    void discoverRuntime(runtimeDiscovery())
      .then((discovered) => {
        if (active) capabilities.replace(discovered);
      })
      .catch((error) => {
        if (active) console.warn("[runtime] discovery failed; running degraded:", error);
      });
    return () => {
      active = false;
      capabilities.clear();
      disposeCapabilities();
      disposeEndpoint();
    };
  },
});
