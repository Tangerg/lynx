import { useCallback } from "react";
import type { SidebarSession } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { invalidateSessions, SESSIONS_KEY } from "@/lib/data/queries";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";

/** Pin / unpin a session (sessions.update favorite) and refresh the sidebar.
 *  Optimistic: flips the star in the list right away so the row reorders
 *  without waiting for the RPC + refetch; rolls back on failure. */
export function useToggleFavorite(): (id: string, favorite: boolean) => Promise<void> {
  return useCallback(async (id, favorite) => {
    // Cancel any in-flight sessions refetch before the optimistic write so a
    // background invalidate (workspace resync / reconnect) started earlier
    // can't resolve with the old favorite flag and un-flip the star.
    await queryClient.cancelQueries({ queryKey: [SESSIONS_KEY] });
    const prev = queryClient.getQueryData<SidebarSession[]>([SESSIONS_KEY]);
    queryClient.setQueryData<SidebarSession[]>([SESSIONS_KEY], (old) =>
      old?.map((s) => (s.id === id ? { ...s, favorite } : s)),
    );
    try {
      await agentRuntime().updateSession({ sessionId: id, favorite });
      void invalidateSessions();
    } catch (err) {
      if (prev) queryClient.setQueryData([SESSIONS_KEY], prev);
      reportSessionError("favorite", err);
    }
  }, []);
}
