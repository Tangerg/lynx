// Rebuild a mounted session's view from the server's (just-rewritten)
// history — the shared tail of every history-mutating call (sessions.rollback,
// sessions.import). resetView (not resetSession) keeps the mounted session's
// send/resume bindings alive and bumps the view epoch, so stream batches
// queued before the reset are dropped instead of appending below the rebuilt
// history (useAgentSession); the items.list backfill then replays the
// authoritative history as completed-item events. Sessions that aren't
// mounted need nothing — they hydrate fresh on open.

import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";

export async function rehydrateSessionView(sessionId: string): Promise<void> {
  const store = useAgentStore.getState();
  if (!store.sessions[sessionId]) return;
  store.resetView(sessionId);
  const { data } = await getContainer()
    .client()
    .items.list({ sessionId: asSessionId(sessionId) });
  if (data.length > 0) {
    store.applyEvents(
      sessionId,
      data.map((item) => ({ type: "item.completed" as const, item })),
    );
  }
}
