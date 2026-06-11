// Message-level actions shared by the inline header buttons
// (plugins/builtin/chat/message-actions) and the right-click context
// menu (components/chat/message/MessageContextMenu) — the second
// consumer is why these live in lib/ rather than inside either one.
// Every store read happens through getState() at call time (CLAUDE.md
// §5): both callers mount once per message, so they must not subscribe.

import type { Message } from "@/protocol/run/viewState";
import { toast } from "sonner";
import { getContainer } from "@/main/container";
import { asRunId, asSessionId, isErrorType } from "@/rpc";
import { getCurrentSessionView, useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";
import { forkSessionAt } from "./useForkSession";
import { flattenText } from "./messageContent";

function prefillComposer(text: string): void {
  useComposerStore.getState().setValue(text);
  const ta = document.querySelector<HTMLTextAreaElement>(".composer-input");
  ta?.focus();
  ta?.setSelectionRange(text.length, text.length);
}

// Truncate the session's history to just BEFORE the given root run
// (sessions.rollback, inclusive-keep semantics — we pass the preceding root
// run's id, or nothing when it was the first), then rebuild the view from
// the truncated server history. resetView (not resetSession) keeps the
// mounted session's send/resume bindings alive — no remount needed.
async function rollbackToBefore(sessionId: string, runId: string): Promise<boolean> {
  const client = getContainer().client();
  const sid = asSessionId(sessionId);
  const { runs } = await client.items.list({ sessionId: sid });
  const roots = runs.filter((r) => !r.parentRunId && !r.spawnedByItemId);
  const idx = roots.findIndex((r) => r.id === runId);
  if (idx < 0) return false;
  const keep = idx > 0 ? roots[idx - 1]!.id : undefined;
  await client.sessions.rollback({ sessionId: sid, ...(keep ? { toRunId: asRunId(keep) } : {}) });
  const store = useAgentStore.getState();
  store.resetView(sessionId);
  const resp = await client.items.list({ sessionId: sid });
  if (resp.data.length > 0) {
    store.applyEvents(
      sessionId,
      resp.data.map((item) => ({ type: "item.completed" as const, item })),
    );
  }
  return true;
}

function reportRollbackError(err: unknown): void {
  if (isErrorType(err, "session_busy")) {
    toast.error("Session is busy — wait for the current run to finish.");
    return;
  }
  console.error("[message] rollback failed:", err);
  toast.error("Couldn't rewind the conversation.");
}

// Regenerate the answer to the most recent user prompt before the given
// assistant message: rewind history to just before that prompt's run
// (sessions.rollback) and resend it — the old answer is gone, not appended
// to. Messages that never reconciled to a server run (no runId) fall back
// to plain resend (a fresh turn below the old one).
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
    if (!text) return;
    if (!m.runId) {
      send(text);
      return;
    }
    void rollbackToBefore(sid, m.runId)
      .then((ok) => {
        // Re-read send at resolve time — resetView kept the binding, but the
        // tab could have been torn down while the rollback was in flight.
        const liveSend = useAgentStore.getState().sessions[sid]?.send;
        if (ok && liveSend) liveSend(text);
        else if (!ok) send(text); // run unknown to the server — plain resend
      })
      .catch(reportRollbackError);
    return;
  }
}

// Load the message text back into the composer so the user can tweak and
// re-send. Doesn't mutate history — sending creates a new user turn.
export function editMessageInComposer(msg: Message): void {
  const text = flattenText(msg.blocks);
  if (!text) return;
  prefillComposer(text);
}

// Edit-and-rerun for a USER message: rewind history to just before this
// message's run, then prefill the composer with its text for tweaking. The
// message (and everything after it) is gone from history — sending writes
// the corrected turn in its place. Falls back to the non-destructive
// composer prefill when the message never reconciled to a run.
export function editAndRerunMessage(msg: Message): void {
  const sid = useSessionStore.getState().activeSessionId;
  const text = flattenText(msg.blocks);
  if (!sid || !text) return;
  if (msg.role !== "user" || !msg.runId) {
    prefillComposer(text);
    return;
  }
  void rollbackToBefore(sid, msg.runId)
    // Run unknown to the server (ok=false) still prefills — the user can at
    // least resend; only a hard failure (busy / transport) aborts with a toast.
    .then(() => prefillComposer(text))
    .catch(reportRollbackError);
}

// Branch the conversation up to AND INCLUDING this message's run
// (sessions.fork{fromRunId}, AUX_API §4.2): the new session keeps history
// through this exchange and opens as the active tab; the original is
// untouched. No-ops for messages that never reconciled to a run.
export function forkFromMessage(msg: Message): void {
  const sid = useSessionStore.getState().activeSessionId;
  if (!sid || !msg.runId) return;
  void forkSessionAt(sid, asRunId(msg.runId));
}
