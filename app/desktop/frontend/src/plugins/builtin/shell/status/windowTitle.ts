// Built-in plugin: window-title working indicator.
//
// Prefixes the document title with a "●" while any root run is in progress, so
// a user who tabbed away can tell at a glance — from the OS window list / dock
// — that this window still has work cooking (T1.1 of the UX polish backlog).
// Window-level by design: any Session whose current root is Running lights it,
// not just the active tab's. Descendant lifecycle stays inside that root-owned
// tree; a child never becomes an unrelated window-level activity fact.
//
// Implemented as a module-level store subscription (app-lifetime side effect,
// HMR-guarded), the same pattern as completionNotify. It writes through the
// registry's single title composer (setWindowWorking → syncDocumentTitle) so
// the dot and the count badge compose instead of clobbering each other.

import { disposeOnHmr } from "@/lib/hmr";
import { subscribeAnySessionRunning } from "@/plugins/builtin/agent/public/run";
import { definePlugin, READY_HANDLER, WINDOW } from "@/plugins/sdk";

export const windowTitle = definePlugin({
  name: "lyra.builtin.window-title",
  requires: { window: WINDOW },
  setup(ctx) {
    // Subscribe to the "any run working" signal only once the app is READY.
    // subscribeAnySessionRunning reads the agent view-state port, which another
    // plugin's setup binds — a module-eval subscription (as this file used to
    // do) ran before that setup and threw "Agent view state port is not
    // configured", crashing the manifest import chain and blanking the window.
    let unsubscribe: (() => void) | undefined;
    ctx.contribute(READY_HANDLER, () => {
      unsubscribe = subscribeAnySessionRunning((working) => ctx.window.setWorking(working));
      disposeOnHmr(unsubscribe);
    });
    ctx.cleanup(() => unsubscribe?.());
  },
});
