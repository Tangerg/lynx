import { CONFIG, definePlugin } from "@/plugins/sdk";
import { installRuntimeEndpointConfiguration } from "./adapters/runtimeEndpointConfiguration";
import { installRuntimeMutationJournalStorage } from "./adapters/runtimeMutationJournalStorage";
import { runtimeServiceInspector } from "./adapters/runtimeServiceInspector";
import { startRuntimeConnection } from "./adapters/runtimeConnectionProjection";
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
    const connection = startRuntimeConnection(runtimeServiceInspector());
    ctx.cleanup(() => {
      connection.dispose();
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
