import { navigator } from "@/lib/navigation";
import { discardAbandonedDraft } from "../application/session/discardAbandonedDraft";

/**
 * Watch where the user goes so an unused draft doesn't outlive the visit that
 * made it.
 *
 * One subscriber rather than a call at each selection site: sessions are selected
 * from the sidebar, the ⌘K palette, deeplinks and session recovery, and a rule
 * about what the user left behind belongs to the transition, not to each caller
 * that happens to cause one. The location reports both sides of a move for
 * exactly this kind of question.
 */
export function installAbandonedDraftCleanup(): () => void {
  return navigator().subscribe((next, previous) => {
    if (previous.session === next.session) return;
    discardAbandonedDraft(previous.session);
  });
}
