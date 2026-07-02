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

// Known rollback business errors get user copy; unexpected failures keep the
// raw error visible for debugging.
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

// Regeneration rewinds to the prompt's run and sends it again; unreconciled
// prompts fall back to a fresh turn.
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

// Non-destructive composer prefill; sending creates a new user turn.
export function editMessageInComposer(msg: Message): void {
  if (!messageHasDraftContent(msg)) return;
  prefillComposer(msg);
}

// Edit-and-rerun rewinds a reconciled user turn before prefill; unreconciled
// messages fall back to the non-destructive edit path.
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

// Restore stops after rollback. It does not prefill or resend because the user
// is choosing a checkpoint to continue from.
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

// Fork keeps history through this run in a new active session; the original
// session is untouched.
export function forkFromMessage(msg: Message): void {
  const conversation = activeAgentConversation();
  if (!conversation || !msg.runId) return;
  void forkAgentSessionAtRun(conversation.sessionId, msg.runId);
}
