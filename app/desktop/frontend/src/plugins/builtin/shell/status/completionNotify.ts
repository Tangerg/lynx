// Built-in plugin: focus-gated run-completion notifications.
//
// When a run settles (running true→false) while the app window is UNFOCUSED,
// fire one OS notification so a user who tabbed away learns their turn is done
// / needs them — the single biggest "agent client" affordance the desktop was
// missing (T1.1 of the UX polish backlog). Never fires while focused (the
// stream itself is the signal) — the universal focus-gate pattern.
//
// Implemented as a module-level store subscription (app-lifetime side effect,
// disposeOnHmr-guarded against dev hot-reload stacking duplicates — the same
// pattern as other app-lifetime bridges). The plugin entry exists so the bridge
// joins the builtin manifest and primes notification permission at load (while
// the window is focused, so the prompt is allowed).

import { playCompletionChime } from "@/lib/chime";
import { disposeOnHmr } from "@/lib/hmr";
import { ensureOsNotifyPermission, osNotify } from "@/lib/osNotify";
import { subscribeAgentRunSettlements } from "@/plugins/builtin/agent/public/run";
import { definePlugin } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

function onSettled({
  sessionId,
  needsInput,
  errorMessage,
}: {
  sessionId: string;
  needsInput: boolean;
  errorMessage: string | null;
}): void {
  // Focus gate: only alert when the window is blurred / hidden. document.hasFocus
  // is false when another OS window has focus or the app is minimized.
  if (document.hasFocus()) return;

  let title = "Lyra finished";
  let body = "The agent finished its turn.";
  if (needsInput) {
    title = "Lyra needs your input";
    body = "The agent is waiting for your approval or answer.";
  } else if (errorMessage) {
    title = "Lyra hit an error";
    body = errorMessage;
  }
  // tag per session: a session that finishes several runs while you're away
  // replaces its own notification instead of stacking a pile.
  osNotify(title, { body, tag: `run:${sessionId}` });
  // Optional audible companion, same blurred-only gate as the notification.
  if (useUiStore.getState().completionSound) playCompletionChime();
}

const unsubscribe = subscribeAgentRunSettlements(onSettled);
disposeOnHmr(unsubscribe);

export const completionNotify = definePlugin({
  name: "lyra.builtin.completion-notify",
  version: "1.0.0",
  setup() {
    // The run-completion → OS-notification bridge is the module-level
    // subscription above; here we only prime permission at load (window
    // focused, so the prompt is allowed).
    ensureOsNotifyPermission();
  },
});
