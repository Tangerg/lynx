// Built-in plugin: focus-gated run-completion notifications.
//
// When a current root makes an exact Running/Waiting → Waiting/Finished
// transition while the app window is UNFOCUSED, fire one OS notification so a
// user who tabbed away learns whether the turn needs input, completed, failed,
// was canceled, or reached a limit. Never fires while focused (the stream
// itself is the signal) — the universal focus-gate pattern.
//
// Implemented as a module-level store subscription (app-lifetime side effect,
// disposeOnHmr-guarded against dev hot-reload stacking duplicates — the same
// pattern as other app-lifetime bridges). The plugin entry exists so the bridge
// joins the builtin manifest and primes notification permission at load (while
// the window is focused, so the prompt is allowed).

import { playCompletionChime } from "./chime";
import { disposeOnHmr } from "@/lib/hmr";
import { ensureOsNotifyPermission, osNotify } from "./osNotify";
import {
  type RootRunSettlement,
  subscribeRootRunSettlements,
} from "@/plugins/builtin/agent/public/run";
import { definePlugin, READY_HANDLER } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";
import { PRODUCT_NAME } from "@/product";

function onSettled({ sessionId, status, errorMessage }: RootRunSettlement): void {
  // Focus gate: only alert when the window is blurred / hidden. document.hasFocus
  // is false when another OS window has focus or the app is minimized.
  if (document.hasFocus()) return;

  let title = `${PRODUCT_NAME} finished`;
  let body = "The agent finished its turn.";
  switch (status) {
    case "needsInput":
      title = `${PRODUCT_NAME} needs your input`;
      body = "The agent is waiting for your approval or answer.";
      break;
    case "error":
      title = `${PRODUCT_NAME} hit an error`;
      body = errorMessage ?? "The agent run failed.";
      break;
    case "canceled":
      title = `${PRODUCT_NAME} stopped`;
      body = "The agent run was canceled.";
      break;
    case "limit":
      title = `${PRODUCT_NAME} reached a limit`;
      body = "The agent stopped after reaching its configured limit.";
      break;
    case "finished":
      break;
  }
  // tag per session: a session that finishes several runs while you're away
  // replaces its own notification instead of stacking a pile.
  osNotify(title, { body, tag: `run:${sessionId}` });
  // Optional audible companion, same blurred-only gate as the notification.
  if (useUiStore.getState().completionSound) playCompletionChime();
}

export const completionNotify = definePlugin({
  name: "scopeapp.builtin.completion-notify",
  setup(ctx) {
    // Prime notification permission at load (window focused → prompt allowed).
    ensureOsNotifyPermission();
    // Subscribe to run settlements only once the app is READY. The agent
    // view-state port is bound by plugin setup. onReady fires after markAppReady,
    // when every setup has run, so module evaluation cannot race that dependency.
    let unsubscribe: (() => void) | undefined;
    ctx.contribute(READY_HANDLER, () => {
      unsubscribe = subscribeRootRunSettlements(onSettled);
      disposeOnHmr(unsubscribe);
    });
    ctx.cleanup(() => unsubscribe?.());
  },
});
