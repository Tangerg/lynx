// Built-in plugin: the app's ONE runtime.subscribe consumer.
//
// The plugin entry is now only the composition root. Runtime subscription,
// active-cwd resolution, reconnect/retarget looping, and query invalidation
// live in their owning layers under this bounded context.

import { definePlugin } from "@/plugins/sdk";
import { installProjectIndexRefresh } from "./adapters/projectIndexRefresh";
import {
  invalidateWorkspaceEvent,
  invalidateWorkspaceEverything,
  retireWorkspaceReadModels,
} from "./adapters/queryInvalidation";
import {
  canSubscribeWorkspaceEvents,
  subscribeRuntimeWorkspaceEvents,
} from "./adapters/runtimeWorkspaceEvents";
import {
  resolveActiveSessionWorkspaceCwd,
  subscribeWorkspaceCwdInputs,
} from "./adapters/sessionWorkspaceCwd";
import { createWorkspaceEventLoop } from "./application/workspaceEventLoop";
import { startWorkspaceEventSubscription } from "./application/workspaceEventSubscription";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { WORKSPACE_MUTATION_LIFECYCLE_PORTS } from "@/plugins/builtin/workspace/public/ports";

export default definePlugin({
  name: "lyra.builtin.workspace-events",
  requires: {
    runtime: RUNTIME_STREAM_PORTS,
    mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE_PORTS,
  },
  setup(ctx) {
    const loop = createWorkspaceEventLoop({
      subscribe: ({ target, signal }) => subscribeRuntimeWorkspaceEvents(target, signal),
      handleEvent: invalidateWorkspaceEvent,
      invalidateAll: invalidateWorkspaceEverything,
      reportDisconnect: (connectionGeneration) => {
        void ctx.runtime.reportConnectionLoss(connectionGeneration);
      },
    });

    const disposeProjectIndex = installProjectIndexRefresh();
    const disposeSubscription = startWorkspaceEventSubscription({
      canSubscribe: canSubscribeWorkspaceEvents,
      connectionGeneration: ctx.runtime.connectionGeneration,
      subscribeConnection: ctx.runtime.subscribeConnection,
      retireReadModels: () => {
        ctx.mutationLifecycle.replaceRuntimeGeneration();
        retireWorkspaceReadModels();
      },
      resolveWorkspaceCwd: resolveActiveSessionWorkspaceCwd,
      reportResolutionError: (error) =>
        console.warn("[workspace-events] target resolution failed:", error),
      subscribeWorkspaceCwdInputs,
      loop,
    });

    ctx.cleanup(() => {
      disposeSubscription();
      disposeProjectIndex();
    });
  },
});
