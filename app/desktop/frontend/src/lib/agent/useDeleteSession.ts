import { useCallback } from "react";
import type { SidebarSession } from "@/lib/data/queries";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { queryClient } from "@/lib/data/queryClient";
import { invalidateSessions, SESSIONS_KEY } from "@/lib/data/queries";
import { useSessionStore } from "@/state/sessionStore";
import { reportSessionError } from "./reportSessionError";

/**
 * Delete a backend session, close its tab (reselecting a neighbour if it was
 * active), and refetch the sidebar list so the row drops. Counterpart to
 * {@link useCreateSession}.
 */
export function useDeleteSession(): (id: string) => Promise<void> {
  return useCallback(async (id) => {
    // Optimistic: drop the row immediately; otherwise it lingers until the
    // delete RPC and the list refetch both complete. Cancel any in-flight
    // refetch first so it can't resolve with the (still-present) row and undo
    // the optimistic removal; snapshot after cancelling for rollback.
    await queryClient.cancelQueries({ queryKey: [SESSIONS_KEY] });
    const prev = queryClient.getQueryData<SidebarSession[]>([SESSIONS_KEY]);
    queryClient.setQueryData<SidebarSession[]>([SESSIONS_KEY], (old) =>
      old?.filter((s) => s.id !== id),
    );
    try {
      await getContainer().client().sessions.delete(asSessionId(id));
      useSessionStore.getState().closeTab(id);
      void invalidateSessions();
    } catch (err) {
      if (prev) queryClient.setQueryData([SESSIONS_KEY], prev);
      reportSessionError("delete", err);
    }
  }, []);
}
