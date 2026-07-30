// Rebuild a mounted session after a server-side history rewrite
// (sessions.rollback / sessions.import). The current projection remains visible
// while every durable fact is read and projected off-store; one guarded commit
// then replaces it. The refresh bumps the stream epoch up front, so a queued
// event from the discarded history cannot land after the replacement.

import { refreshAgentSessionProjection } from "./refreshSessionProjection";

export async function rehydrateSessionView(sessionId: string): Promise<void> {
  await refreshAgentSessionProjection(sessionId, {
    invalidateQueuedRunEvents: true,
  });
}
