// Built-in plugin: window-title working indicator.
//
// Prefixes the document title with a "●" while any root run is in progress, so
// a user who tabbed away can tell at a glance — from the OS window list / dock
// — that this window still has work cooking (T1.1 of the UX polish backlog).
// Window-level by design: ANY running root run lights it, not just the active
// tab's, since the title represents the whole window. Sub-agent runs never set
// `view.run.running` (handlers route them to the timeline only), so they don't
// trip the indicator — only the root turn does.
//
// Implemented as a module-level store subscription (app-lifetime side effect,
// HMR-guarded), the same pattern as completionNotify. It writes through the
// registry's single title composer (setWindowWorking → syncDocumentTitle) so
// the dot and the count badge compose instead of clobbering each other.

import { disposeOnHmr } from "@/lib/hmr";
import { subscribeAnyAgentRunning } from "@/plugins/builtin/agent/public/run";
import { definePlugin } from "@/plugins/sdk";
import { usePluginStore } from "@/plugins/sdk/registry";

export const windowTitle = definePlugin({
  name: "lyra.builtin.window-title",
  version: "1.0.0",
  setup({ host }) {
    // Subscribe to the "any run working" signal only once the app is READY.
    // subscribeAnyAgentRunning reads the agent view-state port, which another
    // plugin's setup binds — a module-eval subscription (as this file used to
    // do) ran before that setup and threw "Agent view state port is not
    // configured", crashing the manifest import chain and blanking the window.
    let unsubscribe: (() => void) | undefined;
    host.lifecycle.onReady(() => {
      unsubscribe = subscribeAnyAgentRunning((working) =>
        usePluginStore.getState().setWindowWorking(working),
      );
      disposeOnHmr(unsubscribe);
    });
    return () => unsubscribe?.();
  },
});
