import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { useSessionStore } from "@/state/sessionStore";

/**
 * Fork a session (whole-history copy, sessions.fork) and jump into the new
 * branch. Forking from a run boundary (`fromRunId`, AUX_API §4.2) lands with
 * backend batch B4 — until then no position is passed here.
 *
 * The fork inherits the source's chat history, so unlike a fresh create it
 * is no draft — it shows in the sidebar immediately.
 */
export function useForkSession(): (id: string) => Promise<void> {
  const queryClient = useQueryClient();
  return useCallback(
    async (id) => {
      try {
        const fork = await getContainer()
          .client()
          .sessions.fork({ sessionId: asSessionId(id) });
        useSessionStore.getState().selectTab(fork.id);
        void queryClient.invalidateQueries({ queryKey: ["sessions"] });
      } catch (err) {
        console.error("[session] fork failed:", err);
      }
    },
    [queryClient],
  );
}
