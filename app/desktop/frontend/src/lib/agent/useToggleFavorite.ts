import { useCallback } from "react";
import type { SidebarSession } from "@/lib/data/queries";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { queryClient } from "@/lib/data/queryClient";
import { invalidateSessions, SESSIONS_KEY } from "@/lib/data/queries";
import { reportSessionError } from "./reportSessionError";

/** Pin / unpin a session (sessions.update favorite) and refresh the sidebar.
 *  Optimistic: flips the star in the list right away so the row reorders
 *  without waiting for the RPC + refetch; rolls back on failure. */
export function useToggleFavorite(): (id: string, favorite: boolean) => Promise<void> {
  return useCallback(async (id, favorite) => {
    const prev = queryClient.getQueryData<SidebarSession[]>([SESSIONS_KEY]);
    queryClient.setQueryData<SidebarSession[]>([SESSIONS_KEY], (old) =>
      old?.map((s) => (s.id === id ? { ...s, favorite } : s)),
    );
    try {
      await getContainer()
        .client()
        .sessions.update({ sessionId: asSessionId(id), favorite });
      void invalidateSessions();
    } catch (err) {
      if (prev) queryClient.setQueryData([SESSIONS_KEY], prev);
      reportSessionError("favorite", err);
    }
  }, []);
}
