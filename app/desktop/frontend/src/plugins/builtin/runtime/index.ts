import { CONFIG, definePlugin } from "@/plugins/sdk";
import { runtimeCapabilities } from "./application/ports/capabilities";
import { installRuntimeEndpointConfiguration } from "./adapters/runtimeEndpointConfiguration";
import { installRuntimeMutationJournalStorage } from "./adapters/runtimeMutationJournalStorage";
import { installRuntimeCapabilityPort } from "./adapters/runtimeCapabilityStore";
import { createRuntimeServiceController } from "./application/runtimeService";
import { runtimeServiceInspector } from "./adapters/runtimeServiceInspector";
import {
  installRuntimeServiceStatusPort,
  useRuntimeServiceStore,
} from "./adapters/runtimeServiceStore";
import { subscribeRuntimeCapabilities } from "./public/capabilities";
import { verifyRuntimeServiceConnection } from "./public/serviceStatus";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

export default definePlugin({
  name: "lyra.builtin.runtime",
  provides: { stream: RUNTIME_STREAM_PORTS },
  requires: { config: CONFIG },
  setup(ctx) {
    const disposeEndpoint = installRuntimeEndpointConfiguration(ctx);
    const disposeMutationJournal = installRuntimeMutationJournalStorage(ctx);
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
    ctx.cleanup(() => {
      serviceController.dispose();
      useRuntimeServiceStore.getState().clear();
      disposeServiceStatus();
      capabilities.clear();
      disposeCapabilities();
      disposeMutationJournal();
      disposeEndpoint();
    });
    return {
      stream: {
        subscribeCapabilities: subscribeRuntimeCapabilities,
        verifyServiceConnection: verifyRuntimeServiceConnection,
      },
    };
  },
});
