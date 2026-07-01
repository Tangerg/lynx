// Rebuild a mounted session's view from the server's (just-rewritten)
// history — the shared tail of every history-mutating call (sessions.rollback,
// sessions.import). resetView (not resetSession) keeps the mounted session's
// send/resume bindings alive and bumps the view epoch, so stream batches
// queued before the reset are dropped instead of appending below the rebuilt
// history (useAgentSession); the items.list backfill then replays the
// authoritative history as completed-item events. Sessions that aren't
// mounted need nothing — they hydrate fresh on open.

import { agentRuntime } from "../ports/runtimeGateway";
import { agentViewState } from "../ports/viewState";

export async function rehydrateSessionView(sessionId: string): Promise<void> {
  const store = agentViewState();
  if (!store.getSession(sessionId)) return;
  store.resetView(sessionId);
  // Snapshot the epoch resetView just bumped, to detect mid-flight invalidation.
  const epoch = store.getSession(sessionId)?.viewEpoch;
  const { items } = await agentRuntime().loadSessionHistory(sessionId);
  // Abort the backfill if the await window invalidated it: the session was torn
  // down, a newer resetView superseded this one, or the user started a run (its
  // turn now owns the reset view — appending the rolled-back history below it,
  // arrival-ordered, would corrupt order). The direct items.list path here would
  // otherwise skip the interacted/epoch guards useAgentSession's loader applies.
  const live = store.getSession(sessionId);
  if (!live || live.viewEpoch !== epoch || live.view.messages.length > 0) return;
  if (items.length > 0) store.applyCompletedItems(sessionId, items);
}
