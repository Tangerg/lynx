import { useCallback } from "react";
import { invalidateSessions } from "@/lib/data/queries";
import { rpcErrorText } from "@/lib/rpcErrors";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";

/** Relocate a session (sessions.update cwd — features.relocate gated,
 *  API.md §7.2). Refreshing the sidebar list also re-points the git-state
 *  watch: the workspace-events plugin follows the sessions cache, so the
 *  new cwd propagates without a tab switch. Returns whether it stuck —
 *  the banner keeps its input open on failure. */
export function useRelocateSession(): (id: string, cwd: string) => Promise<boolean> {
  return useCallback(async (id, cwd) => {
    try {
      await agentRuntime().updateSession({ sessionId: id, cwd });
      // projects too: the list is derived from session cwds, and this
      // session just moved — its old project may retire, the new one mint.
      await invalidateSessions({ projects: true });
      return true;
    } catch (err) {
      reportSessionError("relocate", err, rpcErrorText(err) ?? String(err));
      return false;
    }
  }, []);
}
