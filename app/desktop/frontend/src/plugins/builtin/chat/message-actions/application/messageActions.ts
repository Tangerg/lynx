// Message-level use cases shared by inline message action buttons and the
// right-click context menu. External callers use the public facade; store reads
// stay inside handlers via getState() so per-message UI does not subscribe.

import type { Message } from "@/protocol/run/viewState";
import { getContainer } from "@/main/container";
import { notifyError, notifyInfo } from "@/lib/notify";
import { asRunId, asSessionId } from "@/rpc";
import { getCurrentSessionView, useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/plugins/builtin/chat/composer/public/store";
import { useSessionStore } from "@/state/sessionStore";
import { flattenText } from "@/plugins/builtin/agent/public/messageContent";
import { buildInput } from "@/plugins/builtin/chat/composer/public/input";
import { describeRpcError } from "@/lib/rpcErrors";
import { forkSessionAt } from "@/lib/agent/useForkSession";
import { rehydrateSessionView } from "@/lib/agent/rehydrateSession";

// Inlined images from a view message's blocks → composer-image shape (mime +
// base64), so resend / regenerate / edit-and-rerun keep the images intact.
function blockImages(msg: Message): { mime: string; data: string }[] {
  return msg.blocks
    .filter((b): b is Extract<Message["blocks"][number], { kind: "image" }> => b.kind === "image")
    .map((b) => ({ mime: b.mime, data: b.data }));
}

// Load a message's content back into the composer for tweak + resend: its text
// into the textarea, its inlined images re-staged. Replaces whatever's there.
function prefillComposer(msg: Message): void {
  const store = useComposerStore.getState();
  const text = flattenText(msg.blocks);
  store.clear(); // wipe current text + staged images first
  store.setValue(text);
  const imgs = blockImages(msg);
  if (imgs.length > 0) store.addImages(imgs);
  const ta = document.querySelector<HTMLTextAreaElement>(".composer-input");
  ta?.focus();
  ta?.setSelectionRange(text.length, text.length);
}

// What a checkpoint restore rewinds (mirrors the wire RestoreType, AUX_API §4.3).
//   - "history": truncate chat to before the turn; working tree untouched.
//   - "files":   restore the working tree to the pre-turn snapshot; chat kept.
//   - "both":    both, atomically.
// "files"/"both" need the pre-turn shadow-git snapshot (toRunId) + features.
// checkpoints; rolling back before the FIRST run has no snapshot to restore from.
export type RestoreType = "history" | "files" | "both";

// Roll the session back to just BEFORE the given root run (sessions.rollback,
// inclusive-keep — we pass the preceding root run's id as the kept boundary, or
// nothing when it was the first), then rebuild the view from the (possibly
// truncated) server history. `restoreType` selects what's rewound; "files"/
// "both" degrade to history-only (loudly) when there's no pre-turn snapshot.
async function rollbackToBefore(
  sessionId: string,
  runId: string,
  restoreType: RestoreType = "history",
): Promise<boolean> {
  const client = getContainer().client();
  const sid = asSessionId(sessionId);
  const { runs } = await client.items.list({ sessionId: sid });
  const roots = runs.filter((r) => !r.parentRunId && !r.spawnedByItemId);
  const idx = roots.findIndex((r) => r.id === runId);
  if (idx < 0) return false;
  const keep = idx > 0 ? roots[idx - 1]!.id : undefined;
  const wantsFiles = restoreType !== "history";
  if (wantsFiles && !keep) {
    notifyInfo("No checkpoint before the first turn — files left as they are.", {
      source: "session",
    });
  }
  await client.sessions.rollback({
    sessionId: sid,
    ...(keep ? { toRunId: asRunId(keep) } : {}),
    ...(wantsFiles && keep ? { restoreType } : {}),
  });
  await rehydrateSessionView(sessionId);
  return true;
}

// Mapped types (session_busy; checkpoint_unavailable — restoreType:"both"
// is atomic, so a missing snapshot is a clean no-op) toast their standard
// copy; anything else is unexpected and also logs the raw error.
function reportRollbackError(err: unknown): void {
  const copy = describeRpcError(err);
  if (!copy) console.error("[message] rollback failed:", err);
  notifyError(copy ?? "Couldn't rewind the conversation.", { source: "session" });
}

export interface RollbackActionOptions {
  /** Also restore the working tree to the pre-turn checkpoint
   *  (restoreType:"both", gated features.checkpoints). */
  restoreFiles?: boolean;
}

// Regenerate the answer to the most recent user prompt before the given
// assistant message: rewind history to just before that prompt's run
// (sessions.rollback) and resend it — the old answer is gone, not appended
// to. Messages that never reconciled to a server run (no runId) fall back
// to plain resend (a fresh turn below the old one).
export function regenerateMessage(msg: Message, opts?: RollbackActionOptions): void {
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
    const imgs = blockImages(m);
    // A user message with only inlined images and no text is still a valid
    // turn to regenerate — the early `return` here would silently skip it.
    if (!text && imgs.length === 0) return;
    if (!m.runId) {
      send(buildInput(text, imgs));
      return;
    }
    void rollbackToBefore(sid, m.runId, opts?.restoreFiles ? "both" : "history")
      .then(() => {
        // resetView kept the binding, but the tab could have been torn down —
        // or merely switched away, which nulls send via useAgentSession's
        // cleanup — while the rollback was in flight. No live binding ⇒ we
        // can't resend; surface it instead of dropping the regenerate silently.
        const liveSend = useAgentStore.getState().sessions[sid]?.send;
        if (!liveSend) {
          notifyInfo("Switched away before regenerate finished — nothing was resent.", {
            source: "session",
          });
          return;
        }
        // ok: history rewound to before the prompt. !ok: run unknown to the
        // server, so this is a plain resend appended below the old turn.
        liveSend(buildInput(text, blockImages(m)));
      })
      .catch(reportRollbackError);
    return;
  }
}

// Load the message text back into the composer so the user can tweak and
// re-send. Doesn't mutate history — sending creates a new user turn.
export function editMessageInComposer(msg: Message): void {
  // Nothing to load if the message has neither text nor images.
  if (!msg.blocks.some((b) => (b.kind === "text" && b.text) || b.kind === "image")) return;
  prefillComposer(msg);
}

// Edit-and-rerun for a USER message: rewind history to just before this
// message's run, then prefill the composer with its text for tweaking. The
// message (and everything after it) is gone from history — sending writes
// the corrected turn in its place. Falls back to the non-destructive
// composer prefill when the message never reconciled to a run.
export function editAndRerunMessage(msg: Message, opts?: RollbackActionOptions): void {
  const sid = useSessionStore.getState().activeSessionId;
  const hasContent = msg.blocks.some((b) => (b.kind === "text" && b.text) || b.kind === "image");
  if (!sid || !hasContent) return;
  if (msg.role !== "user" || !msg.runId) {
    prefillComposer(msg);
    return;
  }
  void rollbackToBefore(sid, msg.runId, opts?.restoreFiles ? "both" : "history")
    // Run unknown to the server (ok=false) still prefills — the user can at
    // least resend; only a hard failure (busy / transport) aborts with a toast.
    .then(() => prefillComposer(msg))
    .catch(reportRollbackError);
}

// Pure restore to a checkpoint: rewind to just BEFORE this user message's turn
// and STOP — no resend, no composer prefill (unlike edit-and-rerun / regenerate,
// which always re-drive the turn). This is cline's "Restore checkpoint": go back
// to a known-good state and decide what to do next yourself. `restoreType`
// chooses what's rewound (conversation / working-tree files / both). No-op for a
// message that never reconciled to a server run.
export function restoreCheckpoint(msg: Message, restoreType: RestoreType): void {
  const sid = useSessionStore.getState().activeSessionId;
  if (!sid || msg.role !== "user" || !msg.runId) return;
  void rollbackToBefore(sid, msg.runId, restoreType)
    .then((ok) => {
      if (ok) notifyInfo(restoreCopy(restoreType), { source: "session" });
    })
    .catch(reportRollbackError);
}

function restoreCopy(restoreType: RestoreType): string {
  switch (restoreType) {
    case "files":
      return "Working tree restored to this checkpoint.";
    case "both":
      return "Conversation and files restored to this checkpoint.";
    default:
      return "Conversation rewound to this checkpoint.";
  }
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
