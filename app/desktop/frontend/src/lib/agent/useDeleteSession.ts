import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { useSessionStore } from "@/state/sessionStore";

/**
 * Delete a backend session, close its tab (reselecting a neighbour if it was
 * active), and refetch the sidebar list so the row drops. Counterpart to
 * {@link useCreateSession}.
 */
export function useDeleteSession(): (id: string) => Promise<void> {
  const queryClient = useQueryClient();
  return useCallback(
    async (id) => {
      try {
        await getContainer().client().sessions.delete(asSessionId(id));
        useSessionStore.getState().closeTab(id);
        void queryClient.invalidateQueries({ queryKey: ["sessions"] });
      } catch (err) {
        console.error("[session] delete failed:", err);
      }
    },
    [queryClient],
  );
}
