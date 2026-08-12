import { definePlugin } from "@/plugins/sdk";
import { runtimeCapabilities } from "./application/ports/capabilities";
import { discoverRuntime } from "./application/discoverRuntime";
import { installRuntimeEndpointConfiguration } from "./adapters/runtimeEndpointConfiguration";
import { installRuntimeCapabilityPort } from "./adapters/runtimeCapabilityStore";
import { runtimeDiscovery } from "./adapters/runtimeDiscovery";
import { createRuntimeServiceController } from "./application/runtimeService";
import { runtimeServiceInspector } from "./adapters/runtimeServiceInspector";
import {
  installRuntimeServiceStatusPort,
  useRuntimeServiceStore,
} from "./adapters/runtimeServiceStore";

export default definePlugin({
  name: "lyra.builtin.runtime",
  version: "1.0.0",
  setup({ host }) {
    const disposeEndpoint = installRuntimeEndpointConfiguration(host);
    const disposeCapabilities = installRuntimeCapabilityPort();
    const serviceStore = useRuntimeServiceStore.getState();
    serviceStore.clear();
    const serviceController = createRuntimeServiceController(runtimeServiceInspector(), {
      checking: () => useRuntimeServiceStore.getState().checking(),
      replace: (observation) => useRuntimeServiceStore.getState().replace(observation),
      unavailable: (failure) => useRuntimeServiceStore.getState().unavailable(failure),
    });
    const disposeServiceStatus = installRuntimeServiceStatusPort(serviceController);
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
    void serviceController.refresh();
    return () => {
      active = false;
      serviceController.dispose();
      useRuntimeServiceStore.getState().clear();
      disposeServiceStatus();
      capabilities.clear();
      disposeCapabilities();
      disposeEndpoint();
    };
  },
});
