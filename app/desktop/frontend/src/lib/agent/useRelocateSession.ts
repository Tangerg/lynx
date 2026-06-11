import { useCallback } from "react";
import { toast } from "sonner";
import { getContainer } from "@/main/container";
import { asSessionId, errorDetail, isErrorType, RpcError } from "@/rpc";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";

/** Relocate a session (sessions.update cwd — features.relocate gated,
 *  API.md §7.2). Refreshing the sidebar list also re-points the git-state
 *  watch: the workspace-events plugin follows the sessions cache, so the
 *  new cwd propagates without a tab switch. Returns whether it stuck —
 *  the banner keeps its input open on failure. */
export function useRelocateSession(): (id: string, cwd: string) => Promise<boolean> {
  return useCallback(async (id, cwd) => {
    try {
      await getContainer()
        .client()
        .sessions.update({ sessionId: asSessionId(id), cwd });
      await queryClient.invalidateQueries({ queryKey: [SESSIONS_KEY] });
      return true;
    } catch (err) {
      const description = isErrorType(err, "cwd_unavailable")
        ? "That path does not exist on the runtime's disk."
        : err instanceof RpcError
          ? (errorDetail(err.data) ?? err.message)
          : String(err);
      toast.error("Relocate failed", { description });
      return false;
    }
  }, []);
}
