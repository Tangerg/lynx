import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { useSessionStore } from "@/state/sessionStore";
import { reportSessionError } from "./reportSessionError";

/**
 * Delete a backend session, close its tab (reselecting a neighbour if it was
 * active), and refetch the sidebar list so the row drops. Counterpart to
 * {@link useCreateSession}.
 */
export function useDeleteSession(): (id: string) => Promise<void> {
  return useCallback(async (id) => {
    try {
      await getContainer().client().sessions.delete(asSessionId(id));
      useSessionStore.getState().closeTab(id);
      void queryClient.invalidateQueries({ queryKey: [SESSIONS_KEY] });
    } catch (err) {
      reportSessionError("delete", err);
    }
  }, []);
}
