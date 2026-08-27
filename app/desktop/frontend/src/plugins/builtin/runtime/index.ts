import { CONFIG, definePlugin } from "@/plugins/sdk";
import { installRuntimeEndpointConfiguration } from "./adapters/runtimeEndpointConfiguration";
import { installRuntimeMutationJournalStorage } from "./adapters/runtimeMutationJournalStorage";
import { runtimeServiceInspector } from "./adapters/runtimeServiceInspector";
import { startRuntimeConnection } from "./adapters/runtimeConnectionProjection";
import {
  RUNTIME_SERVER_SCOPE_PORTS,
  RUNTIME_STREAM_PORTS,
} from "@/plugins/builtin/runtime/public/ports";

export default definePlugin({
  name: "scopeapp.builtin.runtime",
  provides: { serverScope: RUNTIME_SERVER_SCOPE_PORTS, stream: RUNTIME_STREAM_PORTS },
  requires: { config: CONFIG },
  setup(ctx) {
    let connection!: ReturnType<typeof startRuntimeConnection>;
    const disposeEndpoint = installRuntimeEndpointConfiguration(ctx, (commit) => {
      void connection.replaceEndpoint(commit);
    });
    const disposeMutationJournal = installRuntimeMutationJournalStorage(ctx);
    connection = startRuntimeConnection(runtimeServiceInspector());
    ctx.cleanup(() => {
      connection.dispose();
      disposeMutationJournal();
      disposeEndpoint();
    });
    return {
      serverScope: {
        subscribeReplacement: (onReplace: () => void) =>
          connection.subscribeServerReplacement(onReplace),
      },
      stream: {
        connectionGeneration: () => connection.connectionGeneration(),
        subscribeConnection: (onChange: () => void) => connection.subscribeConnection(onChange),
        reportConnectionLoss: (expectedGeneration: string) =>
          connection.reportConnectionLoss(expectedGeneration),
      },
    };
  },
});
