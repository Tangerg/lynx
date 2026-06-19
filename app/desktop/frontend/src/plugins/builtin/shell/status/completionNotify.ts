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
// pattern as agentStore's session-prune subscriber). The plugin entry exists so
// the bridge joins the builtin manifest and primes notification permission at
// load (while the window is focused, so the prompt is allowed).

import type { AgentViewState } from "@/protocol/run/viewState";
import { playCompletionChime } from "@/lib/chime";
import { disposeOnHmr } from "@/lib/hmr";
import { ensureOsNotifyPermission, osNotify } from "@/lib/osNotify";
import { definePlugin } from "@/plugins/sdk";
import { useAgentStore } from "@/state/agentStore";
import { useUiStore } from "@/state/uiStore";

// Per-session last-seen `running`, so we act only on the true→false edge (the
// run settled) rather than on every streaming store mutation.
const lastRunning = new Map<string, boolean>();

function onSettled(sessionId: string, view: AgentViewState): void {
  // Focus gate: only alert when the window is blurred / hidden. document.hasFocus
  // is false when another OS window has focus or the app is minimized.
  if (document.hasFocus()) return;

  let title = "Lyra finished";
  let body = "The agent finished its turn.";
  if (view.openInterrupts.length > 0) {
    title = "Lyra needs your input";
    body = "The agent is waiting for your approval or answer.";
  } else if (view.error) {
    title = "Lyra hit an error";
    body = view.error.message || "The run ended with an error.";
  }
  // tag per session: a session that finishes several runs while you're away
  // replaces its own notification instead of stacking a pile.
  osNotify(title, { body, tag: `run:${sessionId}` });
  // Optional audible companion, same blurred-only gate as the notification.
  if (useUiStore.getState().completionSound) playCompletionChime();
}

const unsubscribe = useAgentStore.subscribe((state) => {
  const { sessions } = state;
  let count = 0;
  for (const id in sessions) {
    count++;
    const running = sessions[id]!.view.run.running;
    const was = lastRunning.get(id) ?? false;
    if (was === running) continue;
    lastRunning.set(id, running);
    if (was && !running) onSettled(id, sessions[id]!.view);
  }
  // Forget dropped sessions so the tracker can't grow unbounded across a long
  // app session that opens/closes many tabs (rare branch — only on a mismatch).
  if (lastRunning.size > count) {
    for (const id of [...lastRunning.keys()]) {
      if (!(id in sessions)) lastRunning.delete(id);
    }
  }
});
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
