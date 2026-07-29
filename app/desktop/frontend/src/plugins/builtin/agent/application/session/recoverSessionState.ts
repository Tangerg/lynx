// Read the session-scoped state the runtime holds and fold it in.
//
// The run stream carries every state snapshot to whoever is following that run —
// and to nobody else. A window that just opened, one that reloaded mid-run, a
// session driven by the autonomous loop, and a rollback that republished the value
// all leave the fold holding either nothing or a value the runtime has moved past.
// This is the cold half, through the recovery method the state key declares.
//
// It is safe to call at any time and in any order against the live stream: the fold
// keeps whichever value carries the higher revision.

import { agentRuntime } from "../ports/runtimeGateway";
import { agentViewState } from "../ports/viewState";

export async function recoverSessionState(sessionId: string): Promise<void> {
  const snapshot = await agentRuntime().loadSessionState(sessionId);
  if (!snapshot) return;
  // Mounted sessions only: an unmounted one has no view to land in, and it reads
  // this again when it opens.
  if (!agentViewState().getSession(sessionId)) return;
  agentViewState().applyStateSnapshot(sessionId, snapshot);
}
