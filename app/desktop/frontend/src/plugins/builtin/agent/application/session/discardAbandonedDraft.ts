import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentCommandOwner } from "../agentCommandOwner";

/**
 * Delete the draft the user just navigated away from, if they never used it.
 *
 * A draft is provisional: it is a real backend session, kept out of the session
 * list until its first message graduates it. There is no tab strip, so once
 * selection moves on there is no route back to an unused one — it is invisible in
 * the list, invisible in the chat, and still sitting on the runtime. Every visit
 * to the empty-composer screen left one behind, which is where the pile of empty
 * sessions came from.
 *
 * Only an UNUSED draft goes: one with messages has graduated by then, and a
 * non-draft session is someone's conversation. The delete is fire-and-forget
 * cleanup of something the user abandoned, so a failure stays in the console —
 * the session simply reappears in the list on the next refetch (it is no longer
 * draft-marked), where it can be deleted like any other.
 */
export function discardAbandonedDraft(sessionId: string): void {
  const owner = agentCommandOwner();
  const state = agentSessionState();
  const view = agentSessionView();
  const runtime = agentRuntime();
  if (!sessionId || !state.isDraftSession(sessionId)) return;
  if ((view.getSession(sessionId)?.view.messages.length ?? 0) > 0) return;

  void owner
    .settle(runtime.deleteSession(sessionId))
    .then(() => {
      if (owner.isCurrent()) return invalidateAgentSessions();
    })
    .catch((err: unknown) => {
      if (owner.isCurrent()) console.warn("[session] discarding unused draft failed:", err);
    });
}
