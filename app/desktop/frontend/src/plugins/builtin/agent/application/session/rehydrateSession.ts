// Rebuild a mounted session after a server-side history rewrite
// (sessions.rollback / sessions.import). The current projection remains visible
// while every durable fact is read and projected off-store; one guarded commit
// then replaces it. The refresh bumps the stream epoch up front, so a queued
// event from the discarded history cannot land after the replacement.

import { refreshAgentSessionProjection } from "./refreshSessionProjection";
import { agentSessionView } from "../ports/sessionView";

const REWRITE_SYNCHRONIZATION_ATTEMPTS = 2;

export async function rehydrateSessionView(sessionId: string): Promise<void> {
  const synchronize = agentSessionView().getSession(sessionId)?.synchronize;
  for (let attempt = 0; attempt < REWRITE_SYNCHRONIZATION_ATTEMPTS; attempt += 1) {
    const committed = synchronize
      ? await synchronize()
      : Boolean(
          await refreshAgentSessionProjection(sessionId, {
            invalidateQueuedRunEvents: true,
          }),
        );
    if (committed) return;
  }
  throw new Error(`authoritative Session rewrite did not settle for ${sessionId}`);
}
