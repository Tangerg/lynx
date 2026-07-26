import { agentSessionState } from "../application/ports/sessionState";
import { discardAbandonedDraft } from "../application/session/discardAbandonedDraft";

/**
 * Watch selection so an unused draft doesn't outlive the visit that made it.
 *
 * One subscriber rather than a call at each selection site: sessions are selected
 * from the sidebar, the ⌘K palette, deeplinks and session recovery, and a rule
 * about what the user left behind belongs to the transition, not to each caller
 * that happens to cause one. The selection port already reports the previous
 * snapshot for exactly this kind of question.
 */
export function installAbandonedDraftCleanup(): () => void {
  return agentSessionState().subscribeSelection((next, previous) => {
    if (previous.activeSessionId === next.activeSessionId) return;
    discardAbandonedDraft(previous.activeSessionId);
  });
}
