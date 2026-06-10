// Message-level actions shared by the inline header buttons
// (plugins/builtin/chat/message-actions) and the right-click context
// menu (components/chat/message/MessageContextMenu) — the second
// consumer is why these live in lib/ rather than inside either one.
// Every store read happens through getState() at call time (CLAUDE.md
// §5): both callers mount once per message, so they must not subscribe.

import type { Message } from "@/protocol/run/viewState";
import { getCurrentSessionView, useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";
import { flattenText } from "./messageContent";

// Replay the most recent user prompt before the given assistant message.
// The protocol has no "fork-from-here" verb, so the closest regenerate is
// resending that prompt — the backend treats it as a fresh request and
// produces a new response. Silently no-ops when the session has no send
// fn (torn down between render and click).
export function regenerateMessage(msg: Message): void {
  const sid = useSessionStore.getState().activeSessionId;
  const send = useAgentStore.getState().sessions[sid]?.send;
  if (!send) return;
  const { messages } = getCurrentSessionView();
  const idx = messages.findIndex((m) => m.id === msg.id);
  if (idx < 0) return;
  for (let i = idx - 1; i >= 0; i--) {
    const m = messages[i]!;
    if (m.role !== "user") continue;
    const text = flattenText(m.blocks).trim();
    if (text) send(text);
    return;
  }
}

// Load the message text back into the composer so the user can tweak and
// re-send. Doesn't mutate the original message — sending creates a new
// user turn.
export function editMessageInComposer(msg: Message): void {
  const text = flattenText(msg.blocks);
  if (!text) return;
  useComposerStore.getState().setValue(text);
  const ta = document.querySelector<HTMLTextAreaElement>(".composer-input");
  ta?.focus();
  ta?.setSelectionRange(text.length, text.length);
}
