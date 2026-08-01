// Built-in plugin: the app's ONE runtime.subscribe consumer.
//
// The plugin entry is now only the composition root. Runtime subscription,
// active-cwd resolution, reconnect/retarget looping, and query invalidation
// live in their owning layers under this bounded context.

import { definePlugin } from "@/plugins/sdk";
import { subscribeRuntimeCapabilities } from "@/plugins/builtin/runtime/public/capabilities";
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

export default definePlugin({
  name: "lyra.builtin.workspace-events",
  version: "1.0.0",
  requires: ["lyra.builtin.runtime", "lyra.builtin.agent-bootstrap"],
  setup() {
    const loop = createWorkspaceEventLoop({
      subscribe: ({ cwd, signal }) => subscribeRuntimeWorkspaceEvents(cwd, signal),
      handleEvent: invalidateWorkspaceEvent,
      invalidateAll: invalidateWorkspaceEverything,
      reportError: (error) => console.warn("[workspace-events] subscribe failed:", error),
    });

    const disposeProjectIndex = installProjectIndexRefresh();
    const disposeSubscription = startWorkspaceEventSubscription({
      canSubscribe: canSubscribeWorkspaceEvents,
      subscribeCapabilities: subscribeRuntimeCapabilities,
      resolveWorkspaceCwd: resolveActiveSessionWorkspaceCwd,
      subscribeWorkspaceCwdInputs,
      loop,
    });

    return () => {
      disposeSubscription();
      disposeProjectIndex();
    };
  },
});
