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

export default definePlugin({
  name: "lyra.builtin.workspace-events",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const loop = createWorkspaceEventLoop({
      subscribe: ({ target, signal }) => subscribeRuntimeWorkspaceEvents(target, signal),
      handleEvent: invalidateWorkspaceEvent,
      invalidateAll: invalidateWorkspaceEverything,
      reportDisconnect: () => {
        void ctx.runtime.verifyServiceConnection();
      },
    });

    const disposeProjectIndex = installProjectIndexRefresh();
    const disposeSubscription = startWorkspaceEventSubscription({
      canSubscribe: canSubscribeWorkspaceEvents,
      subscribeCapabilities: ctx.runtime.subscribeCapabilities,
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
