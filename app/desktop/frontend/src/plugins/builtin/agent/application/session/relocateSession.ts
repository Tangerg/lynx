import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { rpcErrorText } from "@/lib/rpcErrors";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner } from "../agentCommandOwner";

/** Relocate a session (sessions.update cwd — features.relocate gated,
 *  API.md §7.2). Refreshing session summaries also re-points the git-state
 *  watch: the workspace-events plugin follows the sessions cache, so the
 *  new cwd propagates without a tab switch. Returns whether it stuck —
 *  the banner keeps its input open on failure. */
export function useRelocateSession(): (
  id: string,
  expectedRevision: number,
  cwd: string,
) => Promise<boolean> {
  return useCallback(async (id, expectedRevision, cwd) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    try {
      await owner.settleSessionSummary(id, expectedRevision, (revision) =>
        runtime.updateSession({ sessionId: id, expectedRevision: revision, cwd }),
      );
      owner.assertCurrent();
      // projects too: the list is derived from session cwds, and this
      // session just moved — its old project may retire, the new one mint.
      await invalidateAgentSessions();
      owner.assertCurrent();
      return true;
    } catch (err) {
      if (!owner.isCurrent()) return false;
      // A conditional-write failure means our revision may be stale; a lost
      // response can also hide a committed relocation. Re-read the Session
      // owner in both cases so the banner receives the current cwd/revision or
      // disappears if another client deleted the Session.
      void invalidateAgentSessions();
      reportSessionError("relocate", err, rpcErrorText(err) ?? String(err));
      return false;
    }
  }, []);
}
