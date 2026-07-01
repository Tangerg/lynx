// Message-level use cases shared by inline message action buttons and the
// right-click context menu. External callers use the public facade; store reads
// stay inside handlers via getState() so per-message UI does not subscribe.

import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { notifyError, notifyInfo } from "@/lib/notify";
import { buildInput } from "@/plugins/builtin/chat/composer/public/input";
import { composerInputToAgentInput } from "./inputBridge";
import { describeRpcError } from "@/lib/rpcErrors";
import {
  activeAgentConversation,
  forkAgentSessionAtRun,
  rollbackSessionToBeforeRun,
  sendToAgentSession,
  type RestoreType,
} from "@/plugins/builtin/agent/public/session";
import { replaceComposerDraft } from "@/plugins/builtin/chat/composer/public/draft";
import {
  messageDraftContent,
  messageHasDraftContent,
  regenerationPromptBefore,
} from "./messageActionContent";

export type { RestoreType };

function prefillComposer(msg: Message): void {
  replaceComposerDraft(messageDraftContent(msg));
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
  const conversation = activeAgentConversation();
  if (!conversation) return;
  const { sessionId, messages } = conversation;
  const prompt = regenerationPromptBefore(messages, msg.id);
  if (!prompt) return;
  if (!prompt.runId) {
    sendToAgentSession(
      sessionId,
      composerInputToAgentInput(buildInput(prompt.text, prompt.images)),
    );
    return;
  }
  void rollbackSessionToBeforeRun(sessionId, prompt.runId, opts?.restoreFiles ? "both" : "history")
    .then(() => {
      // resetView kept the binding, but the tab could have been torn down —
      // or merely switched away, which nulls send via useAgentSession's
      // cleanup — while the rollback was in flight. No live binding ⇒ we
      // can't resend; surface it instead of dropping the regenerate silently.
      if (
        !sendToAgentSession(
          sessionId,
          composerInputToAgentInput(buildInput(prompt.text, prompt.images)),
        )
      ) {
        notifyInfo("Switched away before regenerate finished — nothing was resent.", {
          source: "session",
        });
      }
    })
    .catch(reportRollbackError);
}

// Load the message text back into the composer so the user can tweak and
// re-send. Doesn't mutate history — sending creates a new user turn.
export function editMessageInComposer(msg: Message): void {
  if (!messageHasDraftContent(msg)) return;
  prefillComposer(msg);
}

// Edit-and-rerun for a USER message: rewind history to just before this
// message's run, then prefill the composer with its text for tweaking. The
// message (and everything after it) is gone from history — sending writes
// the corrected turn in its place. Falls back to the non-destructive
// composer prefill when the message never reconciled to a run.
export function editAndRerunMessage(msg: Message, opts?: RollbackActionOptions): void {
  const conversation = activeAgentConversation();
  if (!conversation || !messageHasDraftContent(msg)) return;
  if (msg.role !== "user" || !msg.runId) {
    prefillComposer(msg);
    return;
  }
  void rollbackSessionToBeforeRun(
    conversation.sessionId,
    msg.runId,
    opts?.restoreFiles ? "both" : "history",
  )
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
  const conversation = activeAgentConversation();
  if (!conversation || msg.role !== "user" || !msg.runId) return;
  void rollbackSessionToBeforeRun(conversation.sessionId, msg.runId, restoreType)
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
  const conversation = activeAgentConversation();
  if (!conversation || !msg.runId) return;
  void forkAgentSessionAtRun(conversation.sessionId, msg.runId);
}
