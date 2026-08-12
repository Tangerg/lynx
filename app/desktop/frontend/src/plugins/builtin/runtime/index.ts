import { definePlugin } from "@/plugins/sdk";
import { runtimeCapabilities } from "./application/ports/capabilities";
import { installRuntimeEndpointConfiguration } from "./adapters/runtimeEndpointConfiguration";
import { installRuntimeCapabilityPort } from "./adapters/runtimeCapabilityStore";
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
      replace: ({ service, capabilities }) => {
        runtimeCapabilities().replace(capabilities);
        useRuntimeServiceStore.getState().replace(service);
      },
      unavailable: (failure) => {
        runtimeCapabilities().clear();
        useRuntimeServiceStore.getState().unavailable(failure);
      },
    });
    const disposeServiceStatus = installRuntimeServiceStatusPort(serviceController);
    const capabilities = runtimeCapabilities();
    capabilities.clear();
    serviceController.start();
    return () => {
      serviceController.dispose();
      useRuntimeServiceStore.getState().clear();
      disposeServiceStatus();
      capabilities.clear();
      disposeCapabilities();
      disposeEndpoint();
    };
  },
});
