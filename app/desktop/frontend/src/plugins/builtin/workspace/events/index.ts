// Built-in plugin: the app's ONE workspace.subscribe consumer (AUX_API §3).
//
// The plugin entry is now only the composition root. Runtime subscription,
// active-cwd resolution, reconnect/retarget looping, and query invalidation
// live in their owning layers under this bounded context.

import { definePlugin } from "@/plugins/sdk";
import {
  invalidateWorkspaceEvent,
  invalidateWorkspaceEverything,
} from "./adapters/queryInvalidation";
import {
  canSubscribeWorkspaceEvents,
  subscribeRuntimeCapabilities,
  subscribeRuntimeWorkspaceEvents,
} from "./adapters/runtimeWorkspaceEvents";
import {
  resolveActiveSessionWorkspaceCwd,
  subscribeWorkspaceCwdInputs,
} from "./adapters/sessionWorkspaceCwd";
import { createWorkspaceEventLoop } from "./application/workspaceEventLoop";

export default definePlugin({
  name: "lyra.builtin.workspace-events",
  version: "1.0.0",
  setup() {
    const controller = new AbortController();
    let started = false;
    let retargetGeneration = 0;
    const loop = createWorkspaceEventLoop({
      subscribe: ({ cwd, signal }) => subscribeRuntimeWorkspaceEvents(cwd, signal),
      handleEvent: invalidateWorkspaceEvent,
      invalidateAll: invalidateWorkspaceEverything,
      reportError: (error) => console.warn("[workspace-events] subscribe failed:", error),
    });

    const retargetWatch = (): void => {
      const generation = ++retargetGeneration;
      void resolveActiveSessionWorkspaceCwd().then((cwd) => {
        if (generation !== retargetGeneration || controller.signal.aborted) return;
        loop.retarget(cwd);
      });
    };

    const startIfAdvertised = (): void => {
      if (started || controller.signal.aborted || !canSubscribeWorkspaceEvents()) return;
      started = true;
      loop.start(controller.signal);
    };

    startIfAdvertised();
    const unsubRuntime = subscribeRuntimeCapabilities(startIfAdvertised);
    retargetWatch();
    const unsubCwd = subscribeWorkspaceCwdInputs(retargetWatch);

    return () => {
      unsubRuntime();
      unsubCwd();
      controller.abort();
    };
  },
});
